package api

import (
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/config"
)

const (
	defaultLoginWindow     = 15 * time.Minute
	defaultLoginMaxPerIP   = 30
	defaultLoginMaxPerUser = 8
	defaultLoginMaxKeys    = 10000
)

type loginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time

	window  time.Duration
	maxIP   int
	maxUser int
	maxKeys int

	now func() time.Time
}

func newLoginLimiter(window time.Duration, maxIP, maxUser int) *loginLimiter {
	return &loginLimiter{
		failures: make(map[string][]time.Time),
		window:   window,
		maxIP:    maxIP,
		maxUser:  maxUser,
		maxKeys:  config.Inteiro("LOGIN_RATE_MAX_KEYS", defaultLoginMaxKeys),
		now:      time.Now,
	}
}

func ipKey(ip string) string     { return "ip:" + ip }
func userKey(name string) string { return "user:" + normalizeUsername(name) }

func normalizeUsername(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (l *loginLimiter) allowed(ip, username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.countLocked(ipKey(ip)) < l.maxIP && l.countLocked(userKey(username)) < l.maxUser
}

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

	l.podarLocked()
}

func (l *loginLimiter) podarLocked() {
	if l.maxKeys <= 0 || len(l.failures) <= l.maxKeys {
		return
	}

	cutoff := l.now().Add(-l.window)
	for chave, marcas := range l.failures {
		if len(marcas) == 0 || !marcas[len(marcas)-1].After(cutoff) {
			delete(l.failures, chave)
		}
	}
	if len(l.failures) <= l.maxKeys {
		return
	}

	type candidata struct {
		chave  string
		falhas int
		ultima time.Time
	}
	restantes := make([]candidata, 0, len(l.failures))
	for chave, marcas := range l.failures {
		restantes = append(restantes, candidata{chave, len(marcas), marcas[len(marcas)-1]})
	}
	sort.Slice(restantes, func(i, j int) bool {
		if restantes[i].falhas != restantes[j].falhas {
			return restantes[i].falhas < restantes[j].falhas
		}
		return restantes[i].ultima.Before(restantes[j].ultima)
	})

	excedente := len(l.failures) - l.maxKeys
	for _, c := range restantes[:excedente] {
		delete(l.failures, c.chave)
	}
	log.Printf("[Auth] limitador de login podado: %d chaves descartadas, teto de %d", excedente, l.maxKeys)
}

func (l *loginLimiter) succeed(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, userKey(username))
}

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

var proxiesConfiaveisPadrao = []string{
	"127.0.0.0/8", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7",
}

func proxiesConfiaveis() []*net.IPNet {
	brutos := proxiesConfiaveisPadrao
	if raw := strings.TrimSpace(os.Getenv("TRUSTED_PROXY_CIDRS")); raw != "" {
		brutos = strings.Split(raw, ",")
	}

	redes := make([]*net.IPNet, 0, len(brutos))
	for _, bruto := range brutos {
		bruto = strings.TrimSpace(bruto)
		if bruto == "" {
			continue
		}
		if !strings.Contains(bruto, "/") {
			if ip := net.ParseIP(bruto); ip != nil && ip.To4() != nil {
				bruto += "/32"
			} else {
				bruto += "/128"
			}
		}
		_, rede, err := net.ParseCIDR(bruto)
		if err != nil {
			log.Printf("[API] TRUSTED_PROXY_CIDRS: %q não é faixa válida e foi ignorada", bruto)
			continue
		}
		redes = append(redes, rede)
	}
	return redes
}

func ehProxyConfiavel(ip net.IP, redes []*net.IPNet) bool {
	for _, rede := range redes {
		if rede.Contains(ip) {
			return true
		}
	}
	return false
}

func clientIP(r *http.Request, trustProxy bool) string {
	remoto := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		remoto = host
	}
	if !trustProxy {
		return remoto
	}

	redes := proxiesConfiaveis()
	origem := net.ParseIP(remoto)
	if origem == nil || !ehProxyConfiavel(origem, redes) {
		return remoto
	}

	cliente := ""
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		saltos := strings.Split(xff, ",")
		for i := len(saltos) - 1; i >= 0; i-- {
			ip := net.ParseIP(strings.TrimSpace(saltos[i]))
			if ip == nil {
				break
			}
			if cliente == "" {
				cliente = ip.String()
			}
			if !ehProxyConfiavel(ip, redes) {
				return ip.String()
			}
		}
	} else if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ip != nil {
		cliente = ip.String()
	}
	if cliente != "" {
		return cliente
	}
	return remoto
}

const (
	defaultIngestWindow = time.Minute
	defaultIngestMax    = 120
	defaultEnrollMax    = 10

	defaultRecusaAnonimaMax = 30
)

var ingestLimiter = newLoginLimiter(defaultIngestWindow, defaultIngestMax, defaultIngestMax)

func janelaDeIngestao() time.Duration {
	return config.Duracao("INGEST_RATE_WINDOW", defaultIngestWindow)
}

func tetoDeIngestao() int {
	return config.Inteiro("INGEST_RATE_MAX", defaultIngestMax)
}

func tetoDeEnroll() int {
	return config.Inteiro("INGEST_RATE_MAX_ENROLL", defaultEnrollMax)
}

func (l *loginLimiter) usar(chave string, teto int, janela time.Duration) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	agora := l.now()
	corte := agora.Add(-janela)
	marcas := l.failures[chave][:0]
	for _, m := range l.failures[chave] {
		if m.After(corte) {
			marcas = append(marcas, m)
		}
	}
	l.failures[chave] = marcas

	if len(marcas) >= teto {
		espera := marcas[0].Add(janela).Sub(agora)
		if espera < time.Second {
			espera = time.Second
		}
		return false, espera
	}

	l.failures[chave] = append(marcas, agora)
	l.podarLocked()
	return true, 0
}

func chaveDeIngestao(r *http.Request, cred deviceAuth) string {
	if cred.DeviceID != "" {
		return "ingest:" + cred.DeviceID
	}
	return "ingest-ip:" + clientIP(r, config.Booleano("TRUST_PROXY_HEADERS", false))
}

func tetoDeRecusaAnonima() int {
	return config.Inteiro("INGEST_RATE_MAX_UNAUTH", defaultRecusaAnonimaMax)
}

func chaveDeRecusaAnonima(r *http.Request) string {
	return "recusa-ip:" + clientIP(r, config.Booleano("TRUST_PROXY_HEADERS", false))
}

func recusasAnonimasNoTeto(w http.ResponseWriter, r *http.Request) bool {
	ingestLimiter.mu.Lock()
	recusas := ingestLimiter.contarNaJanelaLocked(chaveDeRecusaAnonima(r), janelaDeIngestao())
	ingestLimiter.mu.Unlock()
	if recusas < tetoDeRecusaAnonima() {
		return false
	}
	auditarSoAPrimeiraRecusa(r, chaveDeRecusaAnonima(r))

	w.Header().Set("Retry-After", strconv.Itoa(int(janelaDeIngestao().Seconds())))
	writeError(w, http.StatusTooManyRequests, "muitas tentativas recusadas vindas deste endereço; tente de novo depois da janela")
	return true
}

func contarRecusaAnonima(r *http.Request) {
	ingestLimiter.usar(chaveDeRecusaAnonima(r), tetoDeRecusaAnonima(), janelaDeIngestao())
}

func (l *loginLimiter) contarNaJanelaLocked(chave string, janela time.Duration) int {
	corte := l.now().Add(-janela)
	n := 0
	for _, m := range l.failures[chave] {
		if m.After(corte) {
			n++
		}
	}
	return n
}

func auditarSoAPrimeiraRecusa(r *http.Request, chave string) {
	if inedita, _ := ingestLimiter.usar("recusa-auditada:"+chave, 1, janelaDeIngestao()); !inedita {
		auditHandledByHandler(r)
	}
}

func limitarTaxa(w http.ResponseWriter, r *http.Request, chave string, teto int) bool {
	permitido, espera := ingestLimiter.usar(chave, teto, janelaDeIngestao())
	if permitido {
		return true
	}
	auditarSoAPrimeiraRecusa(r, chave)

	w.Header().Set("Retry-After", strconv.Itoa(int(espera.Seconds()+0.999)))
	writeError(w, http.StatusTooManyRequests, "limite de envios atingido; tente de novo depois da janela")
	return false
}
