package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func pedirComSessao(t *testing.T, metodo, path, corpo string, sess auth.Session) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(metodo, path, strings.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sess.Token)
	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	return rec
}

func regraNoBanco(t *testing.T, nome string) database.AlertRule {
	t.Helper()

	var r database.AlertRule
	if err := database.DB.Where("name = ?", nome).Take(&r).Error; err != nil {
		t.Fatalf("regra %q não está no banco: %v", nome, err)
	}
	return r
}

func regraNoGET(t *testing.T, nome string, sess auth.Session) database.AlertRule {
	t.Helper()

	rec := pedirComSessao(t, http.MethodGet, "/api/alerts/rules", "", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/alerts/rules: status %d", rec.Code)
	}
	var regras []database.AlertRule
	if err := json.Unmarshal(rec.Body.Bytes(), &regras); err != nil {
		t.Fatalf("GET /api/alerts/rules: %v", err)
	}
	for _, r := range regras {
		if r.Name == nome {
			return r
		}
	}
	t.Fatalf("regra %q ausente do GET", nome)
	return database.AlertRule{}
}

func TestRegraCriadaDesligadaPersisteDesligada(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "operador-r1", auth.RoleOperator)
	const nome = "regra-r1-desligada"
	t.Cleanup(func() { database.DB.Where("name = ?", nome).Delete(&database.AlertRule{}) })

	rec := postComSessao(t, "/api/alerts/rules",
		`{"name":"`+nome+`","metric":"cpu","operator":">","threshold":90,"enabled":false}`, sess)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, esperado 201: %s", rec.Code, rec.Body.String())
	}

	if regraNoBanco(t, nome).Enabled {
		t.Error("regra criada com \"enabled\": false foi gravada ligada no banco")
	}
	if regraNoGET(t, nome, sess).Enabled {
		t.Error("regra criada com \"enabled\": false voltou ligada no GET")
	}
}

func TestRegraDesligadaPeloPatchPersiste(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "operador-r1-patch", auth.RoleOperator)
	const nome = "regra-r1-patch"
	t.Cleanup(func() { database.DB.Where("name = ?", nome).Delete(&database.AlertRule{}) })

	rec := postComSessao(t, "/api/alerts/rules", `{"name":"`+nome+`","metric":"cpu","operator":">","threshold":90}`, sess)
	if rec.Code != http.StatusCreated {
		t.Fatalf("criar: status %d: %s", rec.Code, rec.Body.String())
	}
	id := strconv.FormatUint(uint64(regraNoBanco(t, nome).ID), 10)

	rec = pedirComSessao(t, http.MethodPatch, "/api/alerts/rules?id="+id, `{"enabled":false}`, sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH: status %d: %s", rec.Code, rec.Body.String())
	}
	if regraNoBanco(t, nome).Enabled {
		t.Error("PATCH com \"enabled\": false não desligou a regra no banco")
	}
}

func usuarioNoGET(t *testing.T, username string, sess auth.Session) map[string]any {
	t.Helper()

	rec := pedirComSessao(t, http.MethodGet, "/api/users", "", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/users: status %d", rec.Code)
	}
	var usuarios []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &usuarios); err != nil {
		t.Fatalf("GET /api/users: %v", err)
	}
	for _, u := range usuarios {
		if u["username"] == username {
			return u
		}
	}
	t.Fatalf("usuário %q ausente do GET", username)
	return nil
}

func ativoNoBanco(t *testing.T, username string) bool {
	t.Helper()

	var ativo bool
	if err := database.DB.Raw("SELECT active FROM users WHERE username = ?", username).Row().Scan(&ativo); err != nil {
		t.Fatalf("usuário %q não está no banco: %v", username, err)
	}
	return ativo
}

func TestUsuarioCriadoInativoPersisteInativo(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-r1", auth.RoleAdmin)
	const nome = "usuario-r1-inativo"
	database.DB.Unscoped().Where("username = ?", nome).Delete(&database.User{})
	t.Cleanup(func() { database.DB.Unscoped().Where("username = ?", nome).Delete(&database.User{}) })

	rec := postComSessao(t, "/api/users",
		`{"username":"`+nome+`","password":"senha-do-r1-12345","role":"viewer","active":false}`, sess)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, esperado 201: %s", rec.Code, rec.Body.String())
	}

	if ativoNoBanco(t, nome) {
		t.Error("usuário criado com \"active\": false foi gravado ativo no banco")
	}
	if usuarioNoGET(t, nome, sess)["active"] != false {
		t.Error("usuário criado com \"active\": false voltou ativo no GET")
	}
}

func TestUsuarioSemActiveNasceAtivo(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-r1-padrao", auth.RoleAdmin)
	const nome = "usuario-r1-padrao"
	database.DB.Unscoped().Where("username = ?", nome).Delete(&database.User{})
	t.Cleanup(func() { database.DB.Unscoped().Where("username = ?", nome).Delete(&database.User{}) })

	rec := postComSessao(t, "/api/users", `{"username":"`+nome+`","password":"senha-do-r1-12345","role":"viewer"}`, sess)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, esperado 201: %s", rec.Code, rec.Body.String())
	}
	if !ativoNoBanco(t, nome) {
		t.Error("usuário criado sem \"active\" nasceu inativo; o padrão é ativo")
	}
}

func TestUsuarioDesativadoPeloPatchPersiste(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-r1-patch", auth.RoleAdmin)
	const nome = "usuario-r1-patch"
	database.DB.Unscoped().Where("username = ?", nome).Delete(&database.User{})
	t.Cleanup(func() { database.DB.Unscoped().Where("username = ?", nome).Delete(&database.User{}) })

	rec := postComSessao(t, "/api/users", `{"username":"`+nome+`","password":"senha-do-r1-12345","role":"viewer"}`, sess)
	if rec.Code != http.StatusCreated {
		t.Fatalf("criar: status %d: %s", rec.Code, rec.Body.String())
	}
	var id uint
	database.DB.Raw("SELECT id FROM users WHERE username = ?", nome).Row().Scan(&id)

	rec = pedirComSessao(t, http.MethodPatch, "/api/users?id="+strconv.FormatUint(uint64(id), 10), `{"active":false}`, sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH: status %d: %s", rec.Code, rec.Body.String())
	}
	if ativoNoBanco(t, nome) {
		t.Error("PATCH com \"active\": false não desativou o usuário no banco")
	}
}
