package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	srvC3A = "00000000-0000-0000-0000-0000000c3a01"
	srvC3B = "00000000-0000-0000-0000-0000000c3b01"
)

type cenarioC3 struct {
	siteA, siteB uint
	donoA        auth.Session
	outroA       auth.Session
	viewerA      auth.Session
	viewerB      auth.Session
	adminGlobal  auth.Session
}

func sessaoDaUnidade(userID uint, nome string, site uint, papel string) auth.Session {
	s := site
	return auth.Session{UserID: userID, Username: nome, Role: papel, Accesses: []auth.Access{{SiteID: &s, Role: papel}}}
}

func garantirUsuario(t *testing.T, id uint, nome, papel string) {
	t.Helper()

	err := database.DB.Exec(
		`INSERT INTO users (id, username, password_hash, role, active, created_at)
		 VALUES (?, ?, 'sem-login-neste-teste', ?, true, now()) ON CONFLICT (id) DO NOTHING`,
		id, nome, papel).Error
	if err != nil {
		t.Fatalf("criar o usuário %d do cenário: %v", id, err)
	}
	database.DB.Exec(`SELECT setval('users_id_seq', GREATEST((SELECT last_value FROM users_id_seq), ?::bigint))`, id)
}

func setupC3(t *testing.T) cenarioC3 {
	t.Helper()
	setupAuditAPI(t)

	limpar := func() {
		database.DB.Exec("DELETE FROM dashboards WHERE owner_user_id BETWEEN 9100 AND 9199")
		database.DB.Exec("DELETE FROM annotations WHERE author_user_id BETWEEN 9100 AND 9199")
		database.DB.Exec("DELETE FROM user_site_accesses WHERE user_id BETWEEN 9100 AND 9199")
		database.DB.Exec("DELETE FROM users WHERE id BETWEEN 9100 AND 9199")
		database.DB.Unscoped().Where("id IN ?", []string{srvC3A, srvC3B}).Delete(&database.Server{})
		database.DB.Where("code IN ?", []string{"qa-c3-a", "qa-c3-b"}).Delete(&database.Site{})
	}
	limpar()
	t.Cleanup(limpar)

	a := database.Site{Name: "qa-c3-a", Code: "qa-c3-a"}
	b := database.Site{Name: "qa-c3-b", Code: "qa-c3-b"}
	if err := database.DB.Create(&a).Error; err != nil {
		t.Fatalf("unidade A: %v", err)
	}
	if err := database.DB.Create(&b).Error; err != nil {
		t.Fatalf("unidade B: %v", err)
	}
	for _, s := range []database.Server{
		{ID: srvC3A, Name: "host-c3-a", HostIP: "203.0.113.31", Kind: "ssh", SiteID: &a.ID},
		{ID: srvC3B, Name: "host-c3-b", HostIP: "203.0.113.32", Kind: "ssh", SiteID: &b.ID},
	} {
		if err := database.DB.Create(&s).Error; err != nil {
			t.Fatalf("servidor %s: %v", s.Name, err)
		}
	}

	for _, u := range []struct {
		id    uint
		nome  string
		papel string
	}{
		{9101, "dono-a", auth.RoleOperator}, {9102, "outro-a", auth.RoleOperator},
		{9103, "admin-c3", auth.RoleAdmin}, {9104, "viewer-a", auth.RoleViewer},
		{9105, "viewer-b", auth.RoleViewer},
	} {
		garantirUsuario(t, u.id, u.nome, u.papel)
	}

	return cenarioC3{
		siteA:       a.ID,
		siteB:       b.ID,
		donoA:       sessaoDaUnidade(9101, "dono-a", a.ID, auth.RoleOperator),
		outroA:      sessaoDaUnidade(9102, "outro-a", a.ID, auth.RoleOperator),
		viewerA:     sessaoDaUnidade(9104, "viewer-a", a.ID, auth.RoleViewer),
		viewerB:     sessaoDaUnidade(9105, "viewer-b", b.ID, auth.RoleViewer),
		adminGlobal: auth.Session{UserID: 9103, Username: "admin-c3", Role: auth.RoleAdmin, Accesses: []auth.Access{{Role: auth.RoleAdmin}}},
	}
}

func chamar(t *testing.T, h http.HandlerFunc, sess auth.Session, metodo, rota, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	req := withSession(httptest.NewRequest(metodo, rota, strings.NewReader(corpo)), sess)
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

type painelResp struct {
	ID       uint   `json:"id"`
	Position int    `json:"position"`
	Title    string `json:"title"`
	ServerID string `json:"server_id"`
	Metric   string `json:"metric"`
	Range    string `json:"range"`
	Width    int    `json:"width"`
}

type dashboardResp struct {
	ID        uint         `json:"id"`
	Name      string       `json:"name"`
	Panels    []painelResp `json:"panels"`
	UpdatedAt time.Time    `json:"updated_at"`
}

func painel(srv, metrica, faixa string, largura int) string {
	return fmt.Sprintf(`{"title":"P %s","server_id":"%s","metric":"%s","range":"%s","width":%d}`, metrica, srv, metrica, faixa, largura)
}

func criarDashboard(t *testing.T, sess auth.Session, nome string, paineis ...string) *httptest.ResponseRecorder {
	t.Helper()
	corpo := `{"name":"` + nome + `","panels":[` + strings.Join(paineis, ",") + `]}`
	return chamar(t, dashboardsHandler, sess, http.MethodPost, "/api/dashboards", corpo)
}

func listarDashboards(t *testing.T, sess auth.Session) []dashboardResp {
	t.Helper()
	rec := chamar(t, dashboardsHandler, sess, http.MethodGet, "/api/dashboards", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/dashboards: status %d (%s)", rec.Code, rec.Body.String())
	}
	var out []dashboardResp
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("GET /api/dashboards: %v", err)
	}
	return out
}

func decodeDashboard(t *testing.T, rec *httptest.ResponseRecorder) dashboardResp {
	t.Helper()
	var d dashboardResp
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("resposta do dashboard: %v (%s)", err, rec.Body.String())
	}
	return d
}

func TestDashboardPertenceSoAoDono(t *testing.T) {
	c := setupC3(t)

	rec := criarDashboard(t, c.donoA, "Principal", painel(srvC3A, "cpu", "24h", 2))
	if rec.Code != http.StatusCreated {
		t.Fatalf("criar: status %d (%s)", rec.Code, rec.Body.String())
	}
	d := decodeDashboard(t, rec)
	if d.ID == 0 || d.Name != "Principal" || len(d.Panels) != 1 {
		t.Fatalf("resposta = %+v", d)
	}
	p := d.Panels[0]
	if p.ID == 0 || p.Position != 0 || p.ServerID != srvC3A || p.Metric != "cpu" || p.Range != "24h" || p.Width != 2 || p.Title != "P cpu" {
		t.Errorf("painel = %+v", p)
	}

	if n := len(listarDashboards(t, c.donoA)); n != 1 {
		t.Errorf("o dono vê %d dashboards, esperado 1", n)
	}
	if n := len(listarDashboards(t, c.outroA)); n != 0 {
		t.Errorf("outro usuário da mesma unidade vê %d dashboards do dono", n)
	}
}

func TestDashboardComServidorForaDoAlcanceResponde404(t *testing.T) {
	c := setupC3(t)

	rec := criarDashboard(t, c.donoA, "Invasor", painel(srvC3B, "cpu", "1h", 1))
	if rec.Code != http.StatusNotFound {
		t.Errorf("painel de servidor da unidade B: status %d, esperado 404 (%s)", rec.Code, rec.Body.String())
	}
	if n := len(listarDashboards(t, c.donoA)); n != 0 {
		t.Errorf("a recusa gravou %d dashboard(s)", n)
	}
}

func TestDashboardNomeRepetidoDoMesmoDonoResponde409(t *testing.T) {
	c := setupC3(t)

	if rec := criarDashboard(t, c.donoA, "Rede", painel(srvC3A, "net_rx", "7d", 1)); rec.Code != http.StatusCreated {
		t.Fatalf("primeiro: status %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := criarDashboard(t, c.donoA, "Rede"); rec.Code != http.StatusConflict {
		t.Errorf("mesmo nome, mesmo dono: status %d, esperado 409", rec.Code)
	}
	if rec := criarDashboard(t, c.outroA, "Rede"); rec.Code != http.StatusCreated {
		t.Errorf("mesmo nome, outro dono: status %d, esperado 201 (%s)", rec.Code, rec.Body.String())
	}
}

func TestDashboardLimitesDePaineisEDeDashboards(t *testing.T) {
	c := setupC3(t)

	treze := make([]string, 13)
	for i := range treze {
		treze[i] = painel(srvC3A, "cpu", "1h", 1)
	}
	if rec := criarDashboard(t, c.donoA, "Treze", treze...); rec.Code != http.StatusBadRequest {
		t.Errorf("13 painéis: status %d, esperado 400", rec.Code)
	}
	if rec := criarDashboard(t, c.donoA, "Doze", treze[:12]...); rec.Code != http.StatusCreated {
		t.Errorf("12 painéis: status %d, esperado 201 (%s)", rec.Code, rec.Body.String())
	}

	for i := 2; i <= 20; i++ {
		if rec := criarDashboard(t, c.donoA, "D"+strconv.Itoa(i)); rec.Code != http.StatusCreated {
			t.Fatalf("dashboard %d: status %d (%s)", i, rec.Code, rec.Body.String())
		}
	}
	if rec := criarDashboard(t, c.donoA, "D21"); rec.Code != http.StatusBadRequest {
		t.Errorf("21º dashboard do mesmo usuário: status %d, esperado 400", rec.Code)
	}
}

func TestDashboardValidacoes(t *testing.T) {
	c := setupC3(t)

	casos := map[string]string{
		"nome vazio":           `{"name":"  ","panels":[]}`,
		"nome longo":           `{"name":"` + strings.Repeat("x", 81) + `","panels":[]}`,
		"título longo":         `{"name":"ok","panels":[{"title":"` + strings.Repeat("t", 81) + `","server_id":"` + srvC3A + `","metric":"cpu","range":"1h","width":1}]}`,
		"métrica inválida":     `{"name":"ok","panels":[` + painel(srvC3A, "latency", "1h", 1) + `]}`,
		"faixa inválida":       `{"name":"ok","panels":[` + painel(srvC3A, "cpu", "2y", 1) + `]}`,
		"largura inválida":     `{"name":"ok","panels":[` + painel(srvC3A, "cpu", "1h", 3) + `]}`,
		"sem servidor":         `{"name":"ok","panels":[{"title":"x","metric":"cpu","range":"1h","width":1}]}`,
		"corpo que não é json": `nao-e-json`,
	}
	for nome, corpo := range casos {
		rec := chamar(t, dashboardsHandler, c.donoA, http.MethodPost, "/api/dashboards", corpo)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, esperado 400 (%s)", nome, rec.Code, rec.Body.String())
		}
	}
}

func TestDashboardDeOutroUsuarioResponde404(t *testing.T) {
	c := setupC3(t)
	d := decodeDashboard(t, criarDashboard(t, c.donoA, "Meu", painel(srvC3A, "cpu", "1h", 1)))
	id := strconv.FormatUint(uint64(d.ID), 10)

	if rec := chamar(t, dashboardsHandler, c.outroA, http.MethodPut, "/api/dashboards?id="+id, `{"name":"Tomado","panels":[]}`); rec.Code != http.StatusNotFound {
		t.Errorf("PUT de outro usuário: status %d, esperado 404", rec.Code)
	}
	if rec := chamar(t, dashboardsHandler, c.outroA, http.MethodDelete, "/api/dashboards?id="+id, ""); rec.Code != http.StatusNotFound {
		t.Errorf("DELETE de outro usuário: status %d, esperado 404", rec.Code)
	}
	if got := listarDashboards(t, c.donoA); len(got) != 1 || got[0].Name != "Meu" || len(got[0].Panels) != 1 {
		t.Errorf("o dashboard do dono mudou: %+v", got)
	}
}

func TestDashboardPutSubstituiNomeEPaineis(t *testing.T) {
	c := setupC3(t)
	d := decodeDashboard(t, criarDashboard(t, c.donoA, "Antigo", painel(srvC3A, "cpu", "1h", 1)))
	id := strconv.FormatUint(uint64(d.ID), 10)
	antigo := d.Panels[0].ID

	corpo := `{"name":"Novo","panels":[` + painel(srvC3A, "mem", "6h", 1) + `,` + painel(srvC3A, "temperature", "30d", 2) + `]}`
	rec := chamar(t, dashboardsHandler, c.donoA, http.MethodPut, "/api/dashboards?id="+id, corpo)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT: status %d (%s)", rec.Code, rec.Body.String())
	}

	got := listarDashboards(t, c.donoA)
	if len(got) != 1 || got[0].Name != "Novo" || len(got[0].Panels) != 2 {
		t.Fatalf("depois do PUT: %+v", got)
	}
	if got[0].Panels[0].Metric != "mem" || got[0].Panels[1].Metric != "temperature" || got[0].Panels[1].Position != 1 {
		t.Errorf("painéis fora de ordem: %+v", got[0].Panels)
	}
	var restos int64
	database.DB.Model(&database.DashboardPanel{}).Where("id = ?", antigo).Count(&restos)
	if restos != 0 {
		t.Errorf("o painel antigo continuou no banco")
	}
}

func TestDashboardApagadoLevaOsPaineisEmCascata(t *testing.T) {
	c := setupC3(t)
	d := decodeDashboard(t, criarDashboard(t, c.donoA, "Cascata", painel(srvC3A, "cpu", "1h", 1), painel(srvC3A, "load", "1h", 1)))

	rec := chamar(t, dashboardsHandler, c.donoA, http.MethodDelete, "/api/dashboards?id="+strconv.FormatUint(uint64(d.ID), 10), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE: status %d (%s)", rec.Code, rec.Body.String())
	}
	var n int64
	database.DB.Model(&database.DashboardPanel{}).Where("dashboard_id = ?", d.ID).Count(&n)
	if n != 0 {
		t.Errorf("%d painel(is) sobreviveram ao dashboard", n)
	}

	d2 := decodeDashboard(t, criarDashboard(t, c.donoA, "Cascata SQL", painel(srvC3A, "cpu", "1h", 1)))
	if err := database.DB.Exec("DELETE FROM dashboards WHERE id = ?", d2.ID).Error; err != nil {
		t.Fatalf("apagar direto no banco: %v", err)
	}
	database.DB.Model(&database.DashboardPanel{}).Where("dashboard_id = ?", d2.ID).Count(&n)
	if n != 0 {
		t.Errorf("a chave estrangeira não apagou em cascata: %d painel(is) órfão(s)", n)
	}
}

func TestPainelDeServidorQueSaiuDoAlcanceEOmitido(t *testing.T) {
	c := setupC3(t)
	criarDashboard(t, c.donoA, "Misto", painel(srvC3A, "cpu", "1h", 1))

	database.DB.Model(&database.Server{}).Where("id = ?", srvC3A).Update("site_id", c.siteB)

	got := listarDashboards(t, c.donoA)
	if len(got) != 1 || len(got[0].Panels) != 0 {
		t.Errorf("servidor mudou para a unidade B e o painel continuou visível: %+v", got)
	}
}

func comTokenDeMaquina(t *testing.T, metodo, rota, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(metodo, rota, strings.NewReader(corpo))
	req.Header.Set("X-API-Token", testToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	return rec
}

func TestDashboardsEEscritaDeAnotacaoExigemSessaoDeUsuario(t *testing.T) {
	setupC3(t)

	casos := []struct{ metodo, rota, corpo string }{
		{http.MethodGet, "/api/dashboards", ""},
		{http.MethodPost, "/api/annotations", `{"server_id":null,"text":"da máquina"}`},
		{http.MethodDelete, "/api/annotations?id=1", ""},
	}
	for _, c := range casos {
		rec := comTokenDeMaquina(t, c.metodo, c.rota, c.corpo)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s com token de máquina: status %d (%s), esperado 403", c.metodo, c.rota, rec.Code, rec.Body.String())
			continue
		}
		corpo := rec.Body.String()
		if !strings.Contains(corpo, "sessão de usuário") && !strings.Contains(corpo, "somente leitura") {
			t.Errorf("%s %s: recusa sem explicação (%s)", c.metodo, c.rota, corpo)
		}
	}
}

func TestMaquinaLeAnotacoesComoConcessaoGlobal(t *testing.T) {
	c := setupC3(t)
	if rec := anotar(t, c.donoA, `{"server_id":"`+srvC3A+`","text":"do servidor"}`); rec.Code != http.StatusCreated {
		t.Fatalf("anotação de servidor: status %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := anotar(t, c.adminGlobal, `{"server_id":null,"text":"global"}`); rec.Code != http.StatusCreated {
		t.Fatalf("anotação global: status %d (%s)", rec.Code, rec.Body.String())
	}

	for _, rota := range []string{"/api/annotations", "/api/annotations?server_id=" + srvC3A} {
		rec := comTokenDeMaquina(t, http.MethodGet, rota, "")
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s com token de máquina: status %d (%s), esperado 200", rota, rec.Code, rec.Body.String())
			continue
		}
		var out []anotacaoResp
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("GET %s: %v", rota, err)
		}
		if vistas := textos(out); !vistas["do servidor"] || !vistas["global"] {
			t.Errorf("GET %s com token de máquina viu %v, esperado a do servidor e a global", rota, vistas)
		}
	}
}

func TestEscritasDePaineisEAnotacoesSaoAuditadas(t *testing.T) {
	setupC3(t)
	sess := sessaoReal(t, "admin-auditoria-c3", auth.RoleAdmin)
	t.Cleanup(func() {
		database.DB.Exec("DELETE FROM dashboards WHERE owner_user_id = ?", sess.UserID)
		database.DB.Exec("DELETE FROM annotations WHERE author_user_id = ?", sess.UserID)
	})

	if rec := postComSessao(t, "/api/dashboards", `{"name":"Auditado","panels":[]}`, sess); rec.Code != http.StatusCreated {
		t.Fatalf("POST dashboards: status %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := postComSessao(t, "/api/annotations", `{"server_id":null,"text":"janela de manutenção"}`, sess); rec.Code != http.StatusCreated {
		t.Fatalf("POST annotations: status %d (%s)", rec.Code, rec.Body.String())
	}
	if n := len(linhasDe(t, "dashboards.create")); n != 1 {
		t.Errorf("linhas dashboards.create = %d, esperada 1", n)
	}
	if n := len(linhasDe(t, "annotations.create")); n != 1 {
		t.Errorf("linhas annotations.create = %d, esperada 1", n)
	}
}

type anotacaoResp struct {
	ID        uint      `json:"id"`
	ServerID  *string   `json:"server_id"`
	At        time.Time `json:"at"`
	Text      string    `json:"text"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

func anotar(t *testing.T, sess auth.Session, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	return chamar(t, annotationsHandler, sess, http.MethodPost, "/api/annotations", corpo)
}

func listarAnotacoes(t *testing.T, sess auth.Session, query string) []anotacaoResp {
	t.Helper()
	rec := chamar(t, annotationsHandler, sess, http.MethodGet, "/api/annotations"+query, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/annotations%s: status %d (%s)", query, rec.Code, rec.Body.String())
	}
	var out []anotacaoResp
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("GET /api/annotations: %v", err)
	}
	return out
}

func textos(as []anotacaoResp) map[string]bool {
	m := map[string]bool{}
	for _, a := range as {
		m[a.Text] = true
	}
	return m
}

func TestAnotacaoRecortadaPorUnidadeEGlobalParaTodos(t *testing.T) {
	c := setupC3(t)

	rec := anotar(t, c.donoA, `{"server_id":"`+srvC3A+`","text":"deploy da unidade A"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("anotação do operador da unidade: status %d (%s)", rec.Code, rec.Body.String())
	}
	var criada anotacaoResp
	json.Unmarshal(rec.Body.Bytes(), &criada)
	if criada.Author != "dono-a" || criada.ServerID == nil || *criada.ServerID != srvC3A || criada.At.IsZero() {
		t.Errorf("resposta = %+v", criada)
	}
	if rec := anotar(t, c.adminGlobal, `{"server_id":null,"text":"manutenção geral"}`); rec.Code != http.StatusCreated {
		t.Fatalf("anotação global do admin: status %d (%s)", rec.Code, rec.Body.String())
	}

	vistasA := textos(listarAnotacoes(t, c.viewerA, "?server_id="+srvC3A))
	if !vistasA["deploy da unidade A"] || !vistasA["manutenção geral"] {
		t.Errorf("viewer da unidade A viu %v", vistasA)
	}
	vistasB := textos(listarAnotacoes(t, c.viewerB, ""))
	if vistasB["deploy da unidade A"] || !vistasB["manutenção geral"] {
		t.Errorf("viewer da unidade B viu %v: a da unidade A não pode aparecer, a global sim", vistasB)
	}
	if rec := chamar(t, annotationsHandler, c.viewerB, http.MethodGet, "/api/annotations?server_id="+srvC3A, ""); rec.Code != http.StatusNotFound {
		t.Errorf("viewer da unidade B pedindo servidor da A: status %d, esperado 404", rec.Code)
	}
}

func TestAnotacaoExigePapelDeOperador(t *testing.T) {
	c := setupC3(t)

	if rec := anotar(t, c.donoA, `{"server_id":null,"text":"global sem permissão"}`); rec.Code != http.StatusForbidden {
		t.Errorf("anotação global por operador de unidade: status %d, esperado 403", rec.Code)
	}
	if rec := anotar(t, c.viewerA, `{"server_id":"`+srvC3A+`","text":"viewer escrevendo"}`); rec.Code != http.StatusForbidden {
		t.Errorf("viewer anotando: status %d, esperado 403", rec.Code)
	}
	if rec := anotar(t, c.donoA, `{"server_id":"`+srvC3B+`","text":"fora do alcance"}`); rec.Code != http.StatusNotFound {
		t.Errorf("anotação em servidor de outra unidade: status %d, esperado 404", rec.Code)
	}
}

func TestAnotacaoValidacoes(t *testing.T) {
	c := setupC3(t)

	casos := map[string]string{
		"texto vazio":   `{"server_id":"` + srvC3A + `","text":"   "}`,
		"texto longo":   `{"server_id":"` + srvC3A + `","text":"` + strings.Repeat("a", 281) + `"}`,
		"data inválida": `{"server_id":"` + srvC3A + `","text":"ok","at":"ontem"}`,
		"não é json":    `{`,
	}
	for nome, corpo := range casos {
		if rec := anotar(t, c.donoA, corpo); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, esperado 400 (%s)", nome, rec.Code, rec.Body.String())
		}
	}
	if rec := chamar(t, annotationsHandler, c.donoA, http.MethodGet, "/api/annotations?from=ontem", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("GET com from inválido: status %d, esperado 400", rec.Code)
	}
}

func TestAnotacaoPeriodoPadraoEUltimas24Horas(t *testing.T) {
	c := setupC3(t)
	antiga := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	if rec := anotar(t, c.donoA, `{"server_id":"`+srvC3A+`","text":"antiga","at":"`+antiga+`"}`); rec.Code != http.StatusCreated {
		t.Fatalf("anotação antiga: status %d (%s)", rec.Code, rec.Body.String())
	}
	anotar(t, c.donoA, `{"server_id":"`+srvC3A+`","text":"recente"}`)

	padrao := textos(listarAnotacoes(t, c.donoA, "?server_id="+srvC3A))
	if padrao["antiga"] || !padrao["recente"] {
		t.Errorf("período padrão devolveu %v, esperado só a recente", padrao)
	}
	de := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339)
	amplo := textos(listarAnotacoes(t, c.donoA, "?server_id="+srvC3A+"&from="+de))
	if !amplo["antiga"] || !amplo["recente"] {
		t.Errorf("from de 72 h atrás devolveu %v", amplo)
	}
}

func TestAnotacaoSoOAutorOuAdminGlobalApaga(t *testing.T) {
	c := setupC3(t)
	var a anotacaoResp
	json.Unmarshal(anotar(t, c.donoA, `{"server_id":"`+srvC3A+`","text":"minha"}`).Body.Bytes(), &a)
	id := strconv.FormatUint(uint64(a.ID), 10)

	if rec := chamar(t, annotationsHandler, c.viewerB, http.MethodDelete, "/api/annotations?id="+id, ""); rec.Code != http.StatusNotFound {
		t.Errorf("usuário que não enxerga a anotação: status %d, esperado 404", rec.Code)
	}
	if rec := chamar(t, annotationsHandler, c.outroA, http.MethodDelete, "/api/annotations?id="+id, ""); rec.Code != http.StatusForbidden {
		t.Errorf("outro operador da unidade: status %d, esperado 403", rec.Code)
	}
	if rec := chamar(t, annotationsHandler, c.donoA, http.MethodDelete, "/api/annotations?id="+id, ""); rec.Code != http.StatusOK {
		t.Errorf("autor apagando: status %d, esperado 200", rec.Code)
	}

	json.Unmarshal(anotar(t, c.donoA, `{"server_id":"`+srvC3A+`","text":"do admin apagar"}`).Body.Bytes(), &a)
	if rec := chamar(t, annotationsHandler, c.adminGlobal, http.MethodDelete, "/api/annotations?id="+strconv.FormatUint(uint64(a.ID), 10), ""); rec.Code != http.StatusOK {
		t.Errorf("admin global apagando a de outro: status %d, esperado 200", rec.Code)
	}
}

func TestUsuarioApagadoLevaOsDashboards(t *testing.T) {
	setupC3(t)
	admin := sessaoReal(t, "admin-apaga-c3", auth.RoleAdmin)
	alvo := sessaoReal(t, "usuario-com-dashboard-c3", auth.RoleViewer)
	if rec := criarDashboard(t, alvo, "Vai sumir"); rec.Code != http.StatusCreated {
		t.Fatalf("criar: status %d (%s)", rec.Code, rec.Body.String())
	}

	rec := pedirComSessao(t, http.MethodDelete, "/api/users?id="+strconv.FormatUint(uint64(alvo.UserID), 10), "", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("apagar usuário: status %d (%s)", rec.Code, rec.Body.String())
	}
	var n int64
	database.DB.Model(&database.Dashboard{}).Where("owner_user_id = ?", alvo.UserID).Count(&n)
	if n != 0 {
		t.Errorf("%d dashboard(s) do usuário apagado ficaram órfãos", n)
	}
}

func TestDashboardAceitaRTT(t *testing.T) {
	c := setupC3(t)
	rec := criarDashboard(t, c.donoA, "Latência", painel(srvC3A, "rtt", "7d", 1))
	if rec.Code != http.StatusCreated {
		t.Fatalf("painel de rtt: status %d, esperado 201 (%s)", rec.Code, rec.Body.String())
	}
	if d := decodeDashboard(t, rec); len(d.Panels) != 1 || d.Panels[0].Metric != "rtt" {
		t.Errorf("painel gravado = %+v", d.Panels)
	}
}
