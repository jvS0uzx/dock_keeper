package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/joaov/vd_stats/internal/audit"
	"github.com/joaov/vd_stats/internal/auth"
	"github.com/joaov/vd_stats/internal/database"
)

// auditTarget é chamada de dentro de handlers que também rodam fora do
// middleware — rota isenta, chamada direta num teste. Sem o no-op, cada um
// desses caminhos viraria panic em produção.
func TestAuditTargetForaDoMiddlewareNaoEstoura(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/sites", nil)
	auditTarget(req, "site", "1", "matriz", nil)
}

// A unidade da criação vem do CORPO, que o middleware não lê nem pode ler: sem
// o handler informá-la, a linha nascia sem unidade e a auditoria deixava de ser
// recortável justamente na ação que cria a unidade.
func TestAuditoriaDeUnidadeGuardaNomeEId(t *testing.T) {
	setupAuditAPI(t)
	limparUnidadesDeAuditoria(t)

	sess := sessaoReal(t, "operador-auditoria", auth.RoleOperator)

	corpo := `{"name":"Filial Auditoria","code":"qa-auditoria"}`
	rec := postComSessao(t, "/api/sites", corpo, sess)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, esperado 201: %s", rec.Code, rec.Body.String())
	}

	l := umaLinha(t, "site.create")
	if l.TargetType != "site" {
		t.Errorf("target_type = %q, esperado site", l.TargetType)
	}
	if l.TargetLabel != "Filial Auditoria" {
		t.Errorf("target_label = %q, esperado o nome da unidade", l.TargetLabel)
	}
	if l.TargetID == "" {
		t.Error("target_id vazio: o id da unidade criada não foi registrado")
	}
	if l.SiteID == nil {
		t.Fatal("site_id nulo: a unidade da ação veio do corpo e não foi registrada")
	}
	if strconv.FormatUint(uint64(*l.SiteID), 10) != l.TargetID {
		t.Errorf("site_id = %d e target_id = %q divergem", *l.SiteID, l.TargetID)
	}
}

// Numa exclusão o rótulo precisa ser lido ANTES de a linha sumir. Lido depois,
// a auditoria guarda "removeu a unidade 7", que ninguém consegue interpretar
// seis meses adiante — que é exatamente quando a auditoria é consultada.
func TestAuditoriaDeExclusaoGuardaONomeAntesDeSumir(t *testing.T) {
	setupAuditAPI(t)
	limparUnidadesDeAuditoria(t)

	sess := sessaoReal(t, "operador-exclusao", auth.RoleOperator)

	site := database.Site{Name: "Filial Que Sai", Code: "qa-auditoria"}
	if err := database.DB.Create(&site).Error; err != nil {
		t.Fatalf("criar unidade: %v", err)
	}

	url := "/api/sites?id=" + strconv.FormatUint(uint64(site.ID), 10)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req.Header.Set("Authorization", "Bearer "+sess.Token)
	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200: %s", rec.Code, rec.Body.String())
	}

	l := umaLinha(t, "site.delete")
	if l.TargetLabel != "Filial Que Sai" {
		t.Errorf("target_label = %q, esperado o nome lido antes da exclusão", l.TargetLabel)
	}
}

// O rótulo do usuário é o nome, e a mesma linha que passa a carregá-lo não pode
// carregar a senha junto: ela vai para a tabela que o administrador consulta.
func TestAuditoriaDeUsuarioGuardaNomeENaoSenha(t *testing.T) {
	setupAuditAPI(t)

	const nome = "usuario-auditoria-qa"
	database.DB.Unscoped().Where("username = ?", nome).Delete(&database.User{})
	t.Cleanup(func() {
		database.DB.Unscoped().Where("username = ?", nome).Delete(&database.User{})
	})

	sess := sessaoReal(t, "admin-auditoria", auth.RoleAdmin)

	corpo := `{"username":"` + nome + `","password":"` + senhaDeTeste + `","role":"viewer"}`
	rec := postComSessao(t, "/api/users", corpo, sess)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, esperado 201: %s", rec.Code, rec.Body.String())
	}

	l := umaLinha(t, "user.create")
	if l.TargetLabel != nome {
		t.Errorf("target_label = %q, esperado %q", l.TargetLabel, nome)
	}
	// Usuário tem concessões em várias unidades; escolher uma para o campo faria
	// a consulta por unidade mostrar a criação de um admin global como evento de
	// uma filial.
	if l.SiteID != nil {
		t.Errorf("site_id = %d, esperado nulo: usuário não pertence a uma unidade", *l.SiteID)
	}
	for _, campo := range []string{l.TargetLabel, l.Detail, l.ActorUsername, l.TargetID} {
		if strings.Contains(campo, senhaDeTeste) {
			t.Fatalf("a senha vazou para a auditoria: %q", campo)
		}
	}
}

// A unidade de uma regra por unidade também só existe no corpo.
func TestAuditoriaDeRegraGuardaNomeEUnidade(t *testing.T) {
	setupAuditAPI(t)
	limparUnidadesDeAuditoria(t)

	sess := sessaoReal(t, "operador-regra", auth.RoleOperator)

	site := database.Site{Name: "Filial Da Regra", Code: "qa-auditoria"}
	if err := database.DB.Create(&site).Error; err != nil {
		t.Fatalf("criar unidade: %v", err)
	}
	t.Cleanup(func() {
		database.DB.Where("name = ?", "regra-auditoria-qa").Delete(&database.AlertRule{})
	})

	corpo := `{"name":"regra-auditoria-qa","metric":"cpu","operator":">","threshold":90,` +
		`"target_site_id":` + strconv.FormatUint(uint64(site.ID), 10) + `}`
	rec := postComSessao(t, "/api/alerts/rules", corpo, sess)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, esperado 201: %s", rec.Code, rec.Body.String())
	}

	l := umaLinha(t, "alert-rule.create")
	if l.TargetLabel != "regra-auditoria-qa" {
		t.Errorf("target_label = %q, esperado o nome da regra", l.TargetLabel)
	}
	if l.SiteID == nil || *l.SiteID != site.ID {
		t.Errorf("site_id = %v, esperada a unidade %d da regra", l.SiteID, site.ID)
	}
}

// O middleware de auditoria corre ANTES do gate de credencial, de propósito,
// para que a recusa por falta de permissão também vire linha. A consequência é
// que a requisição que ele enxerga ainda não carrega sessão: sem o aviso que
// withSession deixa no contexto, TODA escrita autenticada era gravada sem ator
// — e "quem fez" é metade da pergunta que a auditoria existe para responder.
func TestAuditoriaGravaOAtorDaEscritaAutenticada(t *testing.T) {
	setupAuditAPI(t)
	limparUnidadesDeAuditoria(t)

	sess := sessaoReal(t, "operador-com-ator", auth.RoleOperator)

	rec := postComSessao(t, "/api/sites", `{"name":"Filial Do Ator","code":"qa-auditoria"}`, sess)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, esperado 201: %s", rec.Code, rec.Body.String())
	}

	l := umaLinha(t, "site.create")
	if l.ActorUsername != "operador-com-ator" {
		t.Errorf("actor_username = %q, esperado operador-com-ator", l.ActorUsername)
	}
	if l.ActorRole != auth.RoleOperator {
		t.Errorf("actor_role = %q, esperado %q", l.ActorRole, auth.RoleOperator)
	}
	if l.ActorUserID == nil || *l.ActorUserID != sess.UserID {
		t.Errorf("actor_user_id = %v, esperado %d", l.ActorUserID, sess.UserID)
	}
}

// O contraponto: handler que não chama auditTarget continua gerando a linha de
// antes. Sem isso, a mudança teria trocado cobertura por rótulo.
func TestHandlerSemAlvoMantemALinhaDeAntes(t *testing.T) {
	setupAuditAPI(t)

	sess := sessaoReal(t, "operador-sem-alvo", auth.RoleOperator)

	// Corpo sem name e sem code: o handler recusa antes de tocar no banco, então
	// nunca chega a informar alvo nenhum.
	rec := postComSessao(t, "/api/sites", `{}`, sess)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400", rec.Code)
	}

	l := umaLinha(t, "site.create")
	if l.Result != audit.ResultError {
		t.Errorf("resultado = %q, esperado %q", l.Result, audit.ResultError)
	}
	if l.TargetLabel != "" {
		t.Errorf("target_label = %q, esperado vazio: o handler não informou alvo", l.TargetLabel)
	}
	if l.ActorUsername != "operador-sem-alvo" {
		t.Errorf("ator = %q: a linha deixou de ser gravada como antes", l.ActorUsername)
	}
}

// sessaoReal abre a sessão passando por um usuário de verdade no banco.
//
// A sessão persistente relê as concessões a cada requisição, então sessão
// fabricada para usuário inexistente não sobrevive ao primeiro Lookup. Usuário
// sem linha em user_site_accesses recebe o papel da conta valendo globalmente,
// que é o alcance que estes testes precisam.
func sessaoReal(t *testing.T, username, role string) auth.Session {
	t.Helper()

	const senha = "senha-de-teste-auditoria"

	database.DB.Unscoped().Where("username = ?", username).Delete(&database.User{})
	hash, err := auth.HashPassword(senha)
	if err != nil {
		t.Fatalf("hash da senha: %v", err)
	}
	user := database.User{Username: username, PasswordHash: hash, Role: role, Active: true}
	if err := database.DB.Create(&user).Error; err != nil {
		t.Fatalf("criar usuário %s: %v", username, err)
	}
	t.Cleanup(func() {
		database.DB.Unscoped().Where("username = ?", username).Delete(&database.User{})
	})

	sess, err := auth.Login(username, senha)
	if err != nil {
		t.Fatalf("login de %s: %v", username, err)
	}
	t.Cleanup(func() { auth.Logout(sess.Token) })
	return sess
}

func postComSessao(t *testing.T, path, corpo string, sess auth.Session) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sess.Token)

	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	return rec
}

// umaLinha exige que a ação tenha gerado exatamente um registro. Aceitar
// "a primeira de várias" esconderia duplicação de linha, que já apareceu nesta
// tabela quando dois registradores cobriam a mesma rota.
func umaLinha(t *testing.T, action string) database.AuditLog {
	t.Helper()

	linhas := linhasDe(t, action)
	if len(linhas) != 1 {
		t.Fatalf("linhas de %q = %d, esperada 1", action, len(linhas))
	}
	return linhas[0]
}

func limparUnidadesDeAuditoria(t *testing.T) {
	t.Helper()

	limpar := func() {
		database.DB.Where("code = ?", "qa-auditoria").Delete(&database.Site{})
	}
	limpar()
	t.Cleanup(limpar)
}
