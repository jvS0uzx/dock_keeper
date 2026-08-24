package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/joaov/vd_stats/internal/auth"
	"github.com/joaov/vd_stats/internal/database"
)

// sessaoDeTeste abre uma sessão real e a derruba no fim do teste.
//
// Precisa de banco: a sessão deixou de viver num mapa em memória e a
// autorização é relida a cada Lookup, então uma sessão só se sustenta se o
// usuário e as concessões existirem de verdade. Fabricar sessão para usuário
// inexistente passou a devolver 401, que é o comportamento correto.
func sessaoDeTeste(t *testing.T, nome string, accesses []auth.Access) auth.Session {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de sessão")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}

	// O nome vai cru: dois testes conferem o username que chegou no handler, e
	// um prefixo faria a asserção falhar por motivo que não é o do teste.
	usuario := nome
	limparUsuarioDeGate(t, usuario)
	t.Cleanup(func() { limparUsuarioDeGate(t, usuario) })

	user := database.User{
		Username:     usuario,
		PasswordHash: "sem-login-neste-teste",
		Role:         auth.MaxRole(accesses),
		Active:       true,
	}
	if err := database.DB.Create(&user).Error; err != nil {
		t.Fatalf("criar o usuário de %s: %v", nome, err)
	}
	for _, a := range accesses {
		grant := database.UserSiteAccess{UserID: user.ID, SiteID: a.SiteID, Role: a.Role}
		if err := database.DB.Create(&grant).Error; err != nil {
			t.Fatalf("criar concessão de %s: %v", nome, err)
		}
	}

	s, err := auth.CreateSession(user.ID, user.Username, accesses)
	if err != nil {
		t.Fatalf("CreateSession(%s): %v", nome, err)
	}
	t.Cleanup(func() { auth.Logout(s.Token) })
	return s
}

func limparUsuarioDeGate(t *testing.T, username string) {
	t.Helper()

	var ids []uint
	database.DB.Model(&database.User{}).Where("username = ?", username).Pluck("id", &ids)
	if len(ids) > 0 {
		database.DB.Where("user_id IN ?", ids).Delete(&database.UserSession{})
		database.DB.Where("user_id IN ?", ids).Delete(&database.UserSiteAccess{})
	}
	database.DB.Unscoped().Where("username = ?", username).Delete(&database.User{})
}

// pedeTicket exercita a rota real: autentica por cabeçalho e lê o ticket da
// resposta, como o painel faz antes de abrir o EventSource.
func pedeTicket(t *testing.T, cfg Config, s auth.Session) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/stream-ticket", nil)
	req.Header.Set("Authorization", "Bearer "+s.Token)

	rec := httptest.NewRecorder()
	cfg.requireAuth(cfg.streamTicketHandler)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stream-ticket: status = %d", rec.Code)
	}

	var body struct {
		Ticket string `json:"ticket"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode do ticket: %v", err)
	}
	if body.Ticket == "" {
		t.Fatal("resposta sem ticket")
	}
	return body.Ticket
}

// Regressão do furo C2: o ticket de SSE não carregava sessão, o handler caía no
// fallback de admin global e um visualizador de uma filial lia o auth.log e o
// docker logs de qualquer VPS do parque.
func TestTicketCarregaASessaoDeQuemPediu(t *testing.T) {
	cfg := testConfig()
	filial := uint(3)
	outraFilial := uint(9)

	viewer := sessaoDeTeste(t, "olheiro-da-filial", []auth.Access{
		{SiteID: &filial, Role: auth.RoleViewer},
	})
	ticket := pedeTicket(t, cfg, viewer)

	var vista auth.Session
	rec := httptest.NewRecorder()
	cfg.requireTicket(func(w http.ResponseWriter, r *http.Request) {
		vista = sessionFrom(r)
		okHandler(w, r)
	})(rec, httptest.NewRequest(http.MethodGet, "/api/s?ticket="+ticket, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("ticket próprio recusado: status = %d", rec.Code)
	}
	if vista.Username != "olheiro-da-filial" {
		t.Errorf("stream correu como %q, esperado o visualizador que pediu o ticket", vista.Username)
	}
	if auth.MaxRole(vista.Accesses) != auth.RoleViewer {
		t.Errorf("papel no stream = %q, esperado %q", auth.MaxRole(vista.Accesses), auth.RoleViewer)
	}

	// É esta negativa que o lookupServer usa para responder 404 ao servidor de
	// outra unidade — e ela só existe porque a sessão certa chegou ao handler.
	if !auth.CanSeeSite(vista.Accesses, &filial) {
		t.Error("visualizador perdeu acesso à própria unidade")
	}
	if auth.CanSeeSite(vista.Accesses, &outraFilial) {
		t.Error("visualizador alcançou servidor de outra unidade pelo stream")
	}
	if auth.CanSeeSite(vista.Accesses, nil) {
		t.Error("visualizador de filial alcançou VPS de infraestrutura pelo stream")
	}
}

// O ticket é de uso único e amarrado a uma sessão: consumido, some do store.
func TestTicketDeUmaSessaoNaoServeDuasVezes(t *testing.T) {
	cfg := testConfig()
	viewer := sessaoDeTeste(t, "olheiro", []auth.Access{{SiteID: nil, Role: auth.RoleViewer}})

	ticket := pedeTicket(t, cfg, viewer)
	if _, ok := cfg.tickets.consume(ticket); !ok {
		t.Fatal("primeiro consumo falhou")
	}
	if _, ok := cfg.tickets.consume(ticket); ok {
		t.Error("ticket foi aceito duas vezes")
	}
}

// Handler alcançado sem passar pelo gate não pode rodar como admin global.
func TestSessionFromFalhaFechado(t *testing.T) {
	sess := sessionFrom(httptest.NewRequest(http.MethodGet, "/api/x", nil))

	if len(sess.Accesses) != 0 {
		t.Fatalf("contexto vazio devolveu concessões: %+v", sess.Accesses)
	}
	if auth.GlobalRole(sess.Accesses) != "" {
		t.Errorf("contexto vazio virou papel global %q", auth.GlobalRole(sess.Accesses))
	}
	unidade := uint(3)
	if auth.CanSeeSite(sess.Accesses, &unidade) || auth.CanSeeSite(sess.Accesses, nil) {
		t.Error("contexto vazio enxergou alguma unidade")
	}

	// O recorte resultante é o que vira "1 = 0" no WHERE: filtra, e sem
	// nenhuma unidade na lista.
	scope, status := resolveScope(sess, httptest.NewRequest(http.MethodGet, "/api/x", nil))
	if status != 0 || !scope.filter || len(scope.ids) != 0 {
		t.Fatalf("scope de sessão vazia = %+v, status = %d", scope, status)
	}
	if scope.matches(nil) || scope.matches(&unidade) {
		t.Error("scope de sessão vazia aceitou registro")
	}
}

// Regressão do furo C3: o gate olhava o maior papel em qualquer escopo, então
// o admin de uma filial alcançava /api/users e /api/servers.
func TestAdminDeUnidadeNaoEhAdminGlobal(t *testing.T) {
	cfg := testConfig()
	filial := uint(3)

	casos := []struct {
		nome     string
		accesses []auth.Access
		querSt   int
	}{
		{"admin de filial", []auth.Access{{SiteID: &filial, Role: auth.RoleAdmin}}, http.StatusForbidden},
		{"operador global", []auth.Access{{SiteID: nil, Role: auth.RoleOperator}}, http.StatusForbidden},
		{"visualizador global", []auth.Access{{SiteID: nil, Role: auth.RoleViewer}}, http.StatusForbidden},
		{"admin global", []auth.Access{{SiteID: nil, Role: auth.RoleAdmin}}, http.StatusOK},
		{"admin global e de filial", []auth.Access{
			{SiteID: nil, Role: auth.RoleAdmin},
			{SiteID: &filial, Role: auth.RoleAdmin},
		}, http.StatusOK},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			s := sessaoDeTeste(t, c.nome, c.accesses)
			req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
			req.Header.Set("Authorization", "Bearer "+s.Token)

			rec := httptest.NewRecorder()
			cfg.requireGlobalRole(auth.RoleAdmin)(okHandler)(rec, req)
			if rec.Code != c.querSt {
				t.Errorf("status = %d, esperado %d", rec.Code, c.querSt)
			}
		})
	}

	// O API_TOKEN continua sendo credencial de máquina com admin global.
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	cfg.requireGlobalRole(auth.RoleAdmin)(okHandler)(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("API_TOKEN barrado em rota de admin: status = %d", rec.Code)
	}
}

// O mesmo furo, agora pelo mux montado — é a fiação que importa aqui.
//
// O admin de filial é barrado antes do handler, então nada toca o banco. Para
// o admin global a sonda usa PUT numa rota que não aceita PUT: 405 prova que
// passou da autenticação, sem executar o handler.
func TestRotasDeAdminNoMuxExigemConcessaoGlobal(t *testing.T) {
	handler := Routes(testConfig())
	filial := uint(3)

	deFilial := sessaoDeTeste(t, "admin-da-filial", []auth.Access{{SiteID: &filial, Role: auth.RoleAdmin}})
	global := sessaoDeTeste(t, "admin-global", []auth.Access{{SiteID: nil, Role: auth.RoleAdmin}})

	for _, path := range []string{"/api/users", "/api/servers"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+deFilial.Token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s com admin de filial: status = %d, esperado 403", path, rec.Code)
		}

		req = httptest.NewRequest(http.MethodPut, path, nil)
		req.Header.Set("Authorization", "Bearer "+global.Token)
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s com admin global: status = %d, esperado 405 (passou da autenticação)", path, rec.Code)
		}
	}
}
