package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
)

const testToken = "token-de-teste"

func testConfig() Config {
	return Config{
		Addr:           ":0",
		Token:          testToken,
		AllowedOrigins: []string{"https://painel.exemplo.com"},
		tickets:        newTicketStore(),
	}
}

func okHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func TestRequireAuthRejectsMissingToken(t *testing.T) {
	h := testConfig().requireAuth(okHandler)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/metrics/live", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sem token: status = %d, esperado %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuthAcceptsHeaders(t *testing.T) {
	cases := map[string]func(*http.Request){
		"bearer":      func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+testToken) },
		"x-api-token": func(r *http.Request) { r.Header.Set("X-API-Token", testToken) },
	}

	for name, setHeader := range cases {
		t.Run(name, func(t *testing.T) {
			h := testConfig().requireAuth(okHandler)
			req := httptest.NewRequest(http.MethodGet, "/api/metrics/live", nil)
			setHeader(req)

			rec := httptest.NewRecorder()
			h(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
			}
		})
	}
}

func TestRequireAuthRejectsWrongToken(t *testing.T) {
	h := testConfig().requireAuth(okHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/metrics/live", nil)
	req.Header.Set("Authorization", "Bearer errado")

	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("token errado: status = %d, esperado %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestTokenNaQueryNuncaAutentica(t *testing.T) {
	cfg := testConfig()

	for name, h := range map[string]http.HandlerFunc{
		"rota normal": cfg.requireAuth(okHandler),
		"rota de SSE": cfg.requireTicket(okHandler),
	} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/api/x?token="+testToken, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s aceitou o API_TOKEN na query: status = %d", name, rec.Code)
		}
	}
}

func TestTicketAutorizaStreamUmaVezSo(t *testing.T) {
	cfg := testConfig()
	h := cfg.requireTicket(okHandler)

	ticket, err := cfg.tickets.issue(machineSession)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/s?ticket="+ticket, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("primeiro uso do ticket: status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/s?ticket="+ticket, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("ticket reutilizado foi aceito: status = %d", rec.Code)
	}
}

func TestTicketExpirado(t *testing.T) {
	cfg := testConfig()
	ticket, _ := cfg.tickets.issue(machineSession)

	cfg.tickets.mu.Lock()
	cfg.tickets.issued[ticket] = ticketEntry{expires: time.Now().Add(-time.Second)}
	cfg.tickets.mu.Unlock()

	rec := httptest.NewRecorder()
	cfg.requireTicket(okHandler)(rec, httptest.NewRequest(http.MethodGet, "/api/s?ticket="+ticket, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("ticket expirado foi aceito: status = %d", rec.Code)
	}
}

func TestTicketInventadoNaoAutoriza(t *testing.T) {
	rec := httptest.NewRecorder()
	testConfig().requireTicket(okHandler)(rec, httptest.NewRequest(http.MethodGet, "/api/s?ticket=chutado", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("ticket inventado foi aceito: status = %d", rec.Code)
	}
}

func TestCORSReflectsOnlyAllowedOrigin(t *testing.T) {
	h := testConfig().withCORS(okHandler)

	for origin, want := range map[string]string{
		"https://painel.exemplo.com": "https://painel.exemplo.com",
		"https://invasor.exemplo":    "",
	} {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("Origin", origin)

		rec := httptest.NewRecorder()
		h(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != want {
			t.Errorf("origem %s: Allow-Origin = %q, esperado %q", origin, got, want)
		}
		if rec.Header().Get("Vary") != "Origin" {
			t.Errorf("origem %s: falta Vary: Origin", origin)
		}
	}
}

func TestCORSPreflightShortCircuits(t *testing.T) {
	called := false
	h := testConfig().withCORS(func(http.ResponseWriter, *http.Request) { called = true })

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodOptions, "/api/servers", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight: status = %d, esperado %d", rec.Code, http.StatusNoContent)
	}
	if called {
		t.Fatal("preflight chegou no handler")
	}
}

func TestAllowMethods(t *testing.T) {
	h := allowMethods(http.MethodGet, http.MethodPost)(okHandler)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodDelete, "/api/servers", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if got := rec.Header().Get("Allow"); got != "GET, POST" {
		t.Fatalf("Allow = %q, esperado %q", got, "GET, POST")
	}
}

func TestRoutesRequireToken(t *testing.T) {
	handler := Routes(testConfig())

	protected := []string{
		"/api/servers",
		"/api/metrics/live",
		"/api/metrics/history",
		"/api/security/radar",
		"/api/ssl/domains",
		"/api/alerts/rules",
		"/api/logs/search",
		"/api/containers/logs/stream",
		"/api/security/authlog/stream",
		"/api/stream-ticket",
	}
	for _, path := range protected {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s sem token: status = %d, esperado %d", path, rec.Code, http.StatusUnauthorized)
		}
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("/healthz: status = %d, esperado %d", rec.Code, http.StatusOK)
	}
}

func TestLoadConfigRequiresToken(t *testing.T) {
	t.Setenv("API_TOKEN", "")
	if _, err := LoadConfig(":8080"); err == nil {
		t.Fatal("LoadConfig aceitou subir sem API_TOKEN")
	}

	t.Setenv("API_TOKEN", testToken)
	t.Setenv("ALLOWED_ORIGINS", "https://a.exemplo , https://b.exemplo")
	cfg, err := LoadConfig(":8080")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.AllowedOrigins) != 2 || cfg.AllowedOrigins[1] != "https://b.exemplo" {
		t.Fatalf("AllowedOrigins = %v", cfg.AllowedOrigins)
	}
}

func TestRequireRoleRespeitaHierarquia(t *testing.T) {
	cfg := testConfig()

	for _, role := range []string{auth.RoleViewer, auth.RoleOperator, auth.RoleAdmin} {
		req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
		req.Header.Set("Authorization", "Bearer "+testToken)

		rec := httptest.NewRecorder()
		cfg.requireRole(role)(okHandler)(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("API_TOKEN barrado em %s: status = %d", role, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	cfg.requireRole(auth.RoleViewer)(okHandler)(rec, httptest.NewRequest(http.MethodGet, "/api/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sem credencial: status = %d, esperado 401", rec.Code)
	}
}

func TestLoginEhPublicoEResultoFechado(t *testing.T) {
	handler := Routes(testConfig())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/login", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("/api/auth/login: status = %d, esperado 405 (rota pública)", rec.Code)
	}

	for _, path := range []string{"/api/users", "/api/auth/me", "/api/sites"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s sem credencial: status = %d, esperado 401", path, rec.Code)
		}
	}
}

func TestSiteScope(t *testing.T) {
	cases := map[string]struct {
		raw        string
		ok         bool
		filter     bool
		includeNil bool
		ids        int
	}{
		"vazio":     {"", true, false, false, 0},
		"all":       {"all", true, false, false, 0},
		"none":      {"none", true, true, true, 0},
		"id valido": {"7", true, true, false, 1},
		"zero":      {"0", false, false, false, 0},
		"texto":     {"abc", false, false, false, 0},
	}

	for name, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/x?site_id="+c.raw, nil)
		scope, ok := parseSiteScope(req)

		if ok != c.ok {
			t.Errorf("%s: ok = %v, esperado %v", name, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if scope.filter != c.filter || scope.includeNil != c.includeNil || len(scope.ids) != c.ids {
			t.Errorf("%s: scope = %+v", name, scope)
		}
	}
}

func TestSiteScopeMatches(t *testing.T) {
	umaUnidade := uint(3)
	outraUnidade := uint(9)

	semFiltro, _ := parseSiteScope(httptest.NewRequest(http.MethodGet, "/x", nil))
	if !semFiltro.matches(nil) || !semFiltro.matches(&umaUnidade) {
		t.Error("sem filtro deveria aceitar tudo")
	}

	naoClassificado, _ := parseSiteScope(httptest.NewRequest(http.MethodGet, "/x?site_id=none", nil))
	if !naoClassificado.matches(nil) || naoClassificado.matches(&umaUnidade) {
		t.Error("none deveria aceitar só o não classificado")
	}

	daUnidade, _ := parseSiteScope(httptest.NewRequest(http.MethodGet, "/x?site_id=3", nil))
	if !daUnidade.matches(&umaUnidade) || daUnidade.matches(&outraUnidade) || daUnidade.matches(nil) {
		t.Error("filtro por unidade aceitou registro de fora")
	}
}

func sitePtr(id uint) *uint { return &id }

func TestResolveScopeIntersectaComAsConcessoes(t *testing.T) {
	global := auth.Session{Accesses: []auth.Access{{SiteID: nil, Role: auth.RoleViewer}}}
	restrito := auth.Session{Accesses: []auth.Access{
		{SiteID: sitePtr(3), Role: auth.RoleOperator},
		{SiteID: sitePtr(5), Role: auth.RoleViewer},
	}}

	req := func(q string) *http.Request {
		return httptest.NewRequest(http.MethodGet, "/api/x"+q, nil)
	}

	if scope, status := resolveScope(global, req("")); status != 0 || scope.filter {
		t.Errorf("global sem filtro: scope=%+v status=%d", scope, status)
	}
	if scope, status := resolveScope(global, req("?site_id=none")); status != 0 || !scope.includeNil {
		t.Errorf("global none: scope=%+v status=%d", scope, status)
	}

	scope, status := resolveScope(restrito, req(""))
	if status != 0 || !scope.filter || len(scope.ids) != 2 {
		t.Fatalf("restrito sem filtro: scope=%+v status=%d", scope, status)
	}
	if scope.matches(nil) {
		t.Error("restrito enxergou o escopo sem unidade")
	}

	if _, status := resolveScope(restrito, req("?site_id=3")); status != 0 {
		t.Errorf("unidade própria: status=%d", status)
	}
	if _, status := resolveScope(restrito, req("?site_id=9")); status != http.StatusForbidden {
		t.Errorf("unidade alheia: status=%d, esperado 403", status)
	}
	if _, status := resolveScope(restrito, req("?site_id=none")); status != http.StatusForbidden {
		t.Errorf("none restrito: status=%d, esperado 403", status)
	}
	if _, status := resolveScope(restrito, req("?site_id=abc")); status != http.StatusBadRequest {
		t.Errorf("malformado: status=%d, esperado 400", status)
	}
}

func TestRequireRoleComSessaoDePessoa(t *testing.T) {
	viewer := sessaoDeTeste(t, "olheiro", []auth.Access{{SiteID: nil, Role: auth.RoleViewer}})

	cfg := testConfig()

	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.Header.Set("Authorization", "Bearer "+viewer.Token)
	rec := httptest.NewRecorder()
	cfg.requireRole(auth.RoleViewer)(func(w http.ResponseWriter, r *http.Request) {
		if got := sessionFrom(r).Username; got != "olheiro" {
			t.Errorf("sessão no contexto = %q", got)
		}
		okHandler(w, r)
	})(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer em rota viewer: status=%d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/x", nil)
	req.Header.Set("Authorization", "Bearer "+viewer.Token)
	rec = httptest.NewRecorder()
	cfg.requireRoleByMethod(auth.RoleViewer, auth.RoleOperator)(okHandler)(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer em escrita: status=%d, esperado 403", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.Header.Set("Authorization", "Bearer "+viewer.Token)
	rec = httptest.NewRecorder()
	cfg.requireRoleByMethod(auth.RoleViewer, auth.RoleOperator)(okHandler)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer em leitura: status=%d", rec.Code)
	}
}
