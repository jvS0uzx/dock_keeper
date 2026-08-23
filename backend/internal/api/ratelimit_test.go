package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// relogioFalso devolve um now controlado pelo teste, para a janela deslizante
// ser exercitada sem esperar de verdade.
func relogioFalso(inicio time.Time) (func() time.Time, func(time.Duration)) {
	agora := inicio
	return func() time.Time { return agora }, func(d time.Duration) { agora = agora.Add(d) }
}

func TestLimiteDeLoginBloqueiaPorIP(t *testing.T) {
	l := newLoginLimiter(15*time.Minute, 3, 100)

	for i := 0; i < 3; i++ {
		if !l.allowed("192.0.2.10", "nome"+string(rune('a'+i))) {
			t.Fatalf("tentativa %d deveria ser permitida", i+1)
		}
		l.fail("192.0.2.10", "nome"+string(rune('a'+i)))
	}

	if l.allowed("192.0.2.10", "outro") {
		t.Error("quarta tentativa do mesmo IP deveria ser recusada")
	}
	if !l.allowed("192.0.2.11", "outro") {
		t.Error("outro IP não pode herdar o bloqueio")
	}
}

func TestLimiteDeLoginBloqueiaPorUsuario(t *testing.T) {
	l := newLoginLimiter(15*time.Minute, 100, 3)

	// IPs diferentes a cada tentativa: sem o eixo por nome, a força bruta
	// distribuída passaria inteira.
	for i := 0; i < 3; i++ {
		ip := "198.51.100." + string(rune('1'+i))
		if !l.allowed(ip, "admin") {
			t.Fatalf("tentativa %d deveria ser permitida", i+1)
		}
		l.fail(ip, "admin")
	}

	if l.allowed("198.51.100.9", "admin") {
		t.Error("quarta tentativa contra o mesmo nome deveria ser recusada")
	}
	if !l.allowed("198.51.100.9", "outro-nome") {
		t.Error("outro nome não pode herdar o bloqueio")
	}
}

// Alternar maiúsculas não pode render uma cota nova: auth.Login normaliza o
// nome antes de procurar o usuário, e o limite precisa normalizar igual.
func TestLimiteDeLoginNormalizaONome(t *testing.T) {
	l := newLoginLimiter(15*time.Minute, 100, 2)

	l.fail("203.0.113.5", "Admin")
	l.fail("203.0.113.5", "  ADMIN  ")

	if l.allowed("203.0.113.5", "admin") {
		t.Error("variação de caixa do mesmo nome deveria cair no mesmo balde")
	}
}

func TestLimiteDeLoginLiberaDepoisDaJanela(t *testing.T) {
	l := newLoginLimiter(15*time.Minute, 2, 2)
	now, avancar := relogioFalso(time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC))
	l.now = now

	l.fail("192.0.2.20", "alguem")
	l.fail("192.0.2.20", "alguem")
	if l.allowed("192.0.2.20", "alguem") {
		t.Fatal("deveria estar bloqueado dentro da janela")
	}

	avancar(16 * time.Minute)
	if !l.allowed("192.0.2.20", "alguem") {
		t.Error("passada a janela, a tentativa deveria voltar a ser permitida")
	}
	if len(l.failures) != 0 {
		t.Errorf("falhas vencidas deveriam ter sido descartadas, sobraram %d chaves", len(l.failures))
	}
}

// Acertar a senha libera a própria conta, mas não devolve cota ao endereço:
// senão quem tem uma credencial válida renovaria o orçamento de tentativas
// contra as outras contas apenas intercalando o próprio login.
func TestAcertoLiberaContaMasNaoOEndereco(t *testing.T) {
	l := newLoginLimiter(15*time.Minute, 3, 2)

	l.fail("192.0.2.30", "alvo")
	l.fail("192.0.2.30", "alvo")
	l.fail("192.0.2.30", "proprio")

	l.succeed("alvo")

	if !l.allowed("192.0.2.31", "alvo") {
		t.Error("a conta que autenticou deveria sair do bloqueio")
	}
	if l.allowed("192.0.2.30", "terceiro") {
		t.Error("o endereço deveria continuar no teto de 3 falhas")
	}
}

// O 429 tem de sair antes de auth.Login, que é onde mora o bcrypt. O teste roda
// sem banco: se a ordem estiver invertida, o handler alcança database.DB nulo.
func TestLoginResponde429AntesDeConferirASenha(t *testing.T) {
	cfg := testConfig()
	cfg.logins = newLoginLimiter(15*time.Minute, 2, 2)
	cfg.logins.fail("192.0.2.40", "admin")
	cfg.logins.fail("192.0.2.40", "admin")

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"username":"admin","password":"chute"}`))
	req.RemoteAddr = "192.0.2.40:51234"
	rec := httptest.NewRecorder()

	cfg.loginHandler(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusTooManyRequests)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("resposta 429 deveria dizer quanto tempo esperar")
	}
}

// Nome inexistente e nome cadastrado precisam ser indistinguíveis pelo limite:
// ele conta falhas, que os dois casos produzem igual.
func TestLimiteDeLoginNaoRevelaSeOUsuarioExiste(t *testing.T) {
	l := newLoginLimiter(15*time.Minute, 100, 2)

	for _, nome := range []string{"existe", "nao-existe"} {
		l.fail("192.0.2.50", nome)
		l.fail("192.0.2.50", nome)
		if l.allowed("192.0.2.50", nome) {
			t.Errorf("%q: deveria estar bloqueado após 2 falhas", nome)
		}
	}
}

func TestClientIPIgnoraCabecalhoDeProxyPorPadrao(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = "10.0.0.7:44444"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "1.2.3.4")

	if got := clientIP(req, false); got != "10.0.0.7" {
		t.Errorf("sem proxy declarado: clientIP = %q, esperado o RemoteAddr", got)
	}
}

func TestClientIPUsaAUltimaEntradaDoForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = "10.0.0.7:44444"
	// A primeira entrada veio no pedido do cliente e é forjável; a última foi
	// acrescentada pelo proxy da borda.
	req.Header.Set("X-Forwarded-For", "9.9.9.9, 203.0.113.77")

	if got := clientIP(req, true); got != "203.0.113.77" {
		t.Errorf("clientIP = %q, esperado a entrada acrescentada pelo proxy", got)
	}
}
