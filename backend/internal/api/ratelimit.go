package api

import (
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Padrões do limite de login, aplicados quando o ambiente não diz outra coisa.
//
// Os dois eixos existem por motivos diferentes e nenhum substitui o outro: o
// teto por nome contém a força bruta concentrada numa conta, vinda de muitos
// endereços; o teto por endereço contém a varredura que troca de nome a cada
// tentativa e, principalmente, impede que a rota vire negação de serviço — cada
// tentativa custa um bcrypt, e sem teto o atacante compra CPU do servidor de
// graça.
//
// O teto por endereço é folgado de propósito: um escritório inteiro sai por um
// único IP público, e trancar a rota para todo mundo por causa de um vizinho
// distraído é pior que o ataque que o número previne.
const (
	defaultLoginWindow     = 15 * time.Minute
	defaultLoginMaxPerIP   = 30
	defaultLoginMaxPerUser = 8
)

// loginLimiter conta tentativas falhas de login numa janela deslizante.
//
// Mora em memória, como o ticketStore e as sessões: reiniciar o painel zera a
// contagem. Aceitável porque o alvo é encarecer a tentativa às cegas, não
// manter um histórico — e evita mais uma tabela quente.
type loginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time

	window  time.Duration
	maxIP   int
	maxUser int

	// now é injetável para o teste de janela não depender do relógio de parede.
	now func() time.Time
}

func newLoginLimiter(window time.Duration, maxIP, maxUser int) *loginLimiter {
	return &loginLimiter{
		failures: make(map[string][]time.Time),
		window:   window,
		maxIP:    maxIP,
		maxUser:  maxUser,
		now:      time.Now,
	}
}

// As chaves dos dois eixos dividem o mesmo mapa; o prefixo evita que um usuário
// chamado "10.0.0.1" consuma a cota do endereço de mesmo nome.
func ipKey(ip string) string     { return "ip:" + ip }
func userKey(name string) string { return "user:" + normalizeUsername(name) }

// normalizeUsername repete o tratamento que auth.Login dá ao nome. Sem isso
// bastaria alternar maiúsculas a cada tentativa para nunca cair no mesmo balde.
func normalizeUsername(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// allowed diz se a tentativa pode custar um bcrypt. Precisa ser chamada antes
// da conferência de senha, não depois: é ali que está o custo que o limite
// existe para conter.
//
// Não distingue usuário existente de inexistente, porque conta apenas falhas
// registradas — que os dois casos produzem igualmente. Um nome nunca cadastrado
// e um nome cadastrado com senha errada chegam ao mesmo 429 no mesmo momento.
func (l *loginLimiter) allowed(ip, username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.countLocked(ipKey(ip)) < l.maxIP && l.countLocked(userKey(username)) < l.maxUser
}

// fail registra a tentativa malsucedida nos dois eixos.
//
// O aviso sai no instante em que o teto é atingido, e só nele: registrar cada
// recusa seguinte daria ao atacante um jeito de encher o disco de log de graça.
// Como allowed recusa a partir do teto, a contagem nunca o ultrapassa.
func (l *loginLimiter) fail(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.failures[ipKey(ip)] = append(l.failures[ipKey(ip)], now)
	l.failures[userKey(username)] = append(l.failures[userKey(username)], now)

	if len(l.failures[ipKey(ip)]) == l.maxIP {
		log.Printf("[Auth] %s atingiu o teto de %d tentativas de login na janela", ip, l.maxIP)
	}
	if len(l.failures[userKey(username)]) == l.maxUser {
		log.Printf("[Auth] a conta %q atingiu o teto de %d tentativas de login na janela",
			normalizeUsername(username), l.maxUser)
	}
}

// succeed libera a conta que acabou de autenticar, e só ela.
//
// A contagem do endereço fica de pé: zerá-la no acerto daria a quem tem uma
// credencial válida qualquer um jeito de renovar a cota de tentativas contra as
// outras contas, bastando intercalar o próprio login.
func (l *loginLimiter) succeed(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, userKey(username))
}

// countLocked devolve quantas falhas da chave ainda estão dentro da janela e,
// de passagem, descarta as vencidas. É o que dispensa uma rotina de limpeza:
// chave que envelheceu some na primeira consulta seguinte.
func (l *loginLimiter) countLocked(key string) int {
	cutoff := l.now().Add(-l.window)
	kept := l.failures[key][:0]
	for _, at := range l.failures[key] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, key)
		return 0
	}
	l.failures[key] = kept
	return len(kept)
}

// clientIP resolve o endereço de origem da requisição.
//
// Cabeçalho de proxy só é lido quando o operador declara que existe proxy à
// frente: sem essa declaração o cabeçalho vem do próprio cliente, e trocá-lo a
// cada tentativa contornaria o limite inteiro. Declarado o proxy, vale a última
// entrada de X-Forwarded-For — é a que o proxy da borda acrescentou, a única da
// lista que o cliente não escolheu.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); v != "" {
			return v
		}
		if parts := strings.Split(r.Header.Get("X-Forwarded-For"), ","); len(parts) > 0 {
			if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
				return last
			}
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func envDuration(key string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(key)))
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func envInt(key string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || n <= 0 {
		return def
	}
	return n
}
