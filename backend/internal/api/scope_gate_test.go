package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

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
		garantirUnidade(t, a.SiteID)
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

func garantirUnidade(t *testing.T, siteID *uint) {
	t.Helper()

	if siteID == nil {
		return
	}
	codigo := fmt.Sprintf("qa-unidade-%d", *siteID)
	err := database.DB.Exec(
		`INSERT INTO sites (id, name, code, created_at)
		 VALUES (?, ?, ?, now()) ON CONFLICT (id) DO NOTHING`,
		*siteID, codigo, codigo).Error
	if err != nil {
		t.Fatalf("criar a unidade %d do teste: %v", *siteID, err)
	}
	database.DB.Exec(`SELECT setval('sites_id_seq', GREATEST((SELECT last_value FROM sites_id_seq), ?::bigint))`, *siteID)
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

	scope, status := resolveScope(sess, httptest.NewRequest(http.MethodGet, "/api/x", nil))
	if status != 0 || !scope.filter || len(scope.ids) != 0 {
		t.Fatalf("scope de sessão vazia = %+v, status = %d", scope, status)
	}
	if scope.matches(nil) || scope.matches(&unidade) {
		t.Error("scope de sessão vazia aceitou registro")
	}
}

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

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	cfg.requireGlobalRole(auth.RoleAdmin)(okHandler)(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("API_TOKEN barrado em rota de admin: status = %d", rec.Code)
	}
}

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
