package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func limparUsuario(t *testing.T, username string) {
	t.Helper()

	apagar := func() {
		var u database.User
		if err := database.DB.Where("username = ?", username).Take(&u).Error; err == nil {
			database.DB.Where("user_id = ?", u.ID).Delete(&database.UserSiteAccess{})
			database.DB.Unscoped().Where("id = ?", u.ID).Delete(&database.User{})
		}
	}
	apagar()
	t.Cleanup(apagar)
}

func TestCriarUsuarioComNomeEEmail(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-nome-email", auth.RoleAdmin)
	const username = "joaovitor-teste"
	limparUsuario(t, username)

	corpo := `{"username":"` + username + `","password":"senha-de-teste-1234","role":"admin",` +
		`"nome":"João Vitor Souza","email":"JoaoSouza@Exemplo.com.br"}`
	rec := pedirComSessao(t, http.MethodPost, "/api/users", corpo, sess)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/users: status %d, esperado 201 (%s)", rec.Code, rec.Body.String())
	}

	var criado struct {
		Nome  string `json:"nome"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &criado); err != nil {
		t.Fatalf("resposta do POST: %v", err)
	}
	if criado.Nome != "João Vitor Souza" {
		t.Errorf("resposta trouxe nome=%q", criado.Nome)
	}
	if criado.Email != "joaosouza@example.com" {
		t.Errorf("resposta trouxe email=%q, esperado em minúsculas", criado.Email)
	}

	var noBanco database.User
	if err := database.DB.Where("username = ?", username).Take(&noBanco).Error; err != nil {
		t.Fatalf("usuário não está no banco: %v", err)
	}
	if noBanco.Nome != "João Vitor Souza" || noBanco.Email != "joaosouza@example.com" {
		t.Errorf("banco tem nome=%q email=%q", noBanco.Nome, noBanco.Email)
	}
}

func TestEmailRepetidoRecusa(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-email-dup", auth.RoleAdmin)
	limparUsuario(t, "email-dup-1")
	limparUsuario(t, "email-dup-2")

	primeiro := `{"username":"email-dup-1","password":"senha-de-teste-1234","email":"repetido@exemplo.com"}`
	if rec := pedirComSessao(t, http.MethodPost, "/api/users", primeiro, sess); rec.Code != http.StatusCreated {
		t.Fatalf("primeiro usuário: status %d (%s)", rec.Code, rec.Body.String())
	}

	segundo := `{"username":"email-dup-2","password":"senha-de-teste-1234","email":"REPETIDO@exemplo.com"}`
	rec := pedirComSessao(t, http.MethodPost, "/api/users", segundo, sess)
	if rec.Code != http.StatusConflict {
		t.Fatalf("e-mail repetido: status %d, esperado 409 (%s)", rec.Code, rec.Body.String())
	}
}

func TestUsuarioSemEmailContinuaValendo(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-sem-email", auth.RoleAdmin)
	limparUsuario(t, "sem-email-1")
	limparUsuario(t, "sem-email-2")

	for _, username := range []string{"sem-email-1", "sem-email-2"} {
		corpo := `{"username":"` + username + `","password":"senha-de-teste-1234"}`
		rec := pedirComSessao(t, http.MethodPost, "/api/users", corpo, sess)
		if rec.Code != http.StatusCreated {
			t.Fatalf("%s: status %d, esperado 201 — e-mail vazio não pode colidir (%s)",
				username, rec.Code, rec.Body.String())
		}
	}
}

func TestPatchAtualizaNomeEEmail(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-patch-nome", auth.RoleAdmin)
	const username = "patch-nome-email"
	limparUsuario(t, username)

	criar := `{"username":"` + username + `","password":"senha-de-teste-1234"}`
	rec := pedirComSessao(t, http.MethodPost, "/api/users", criar, sess)
	if rec.Code != http.StatusCreated {
		t.Fatalf("criar: status %d (%s)", rec.Code, rec.Body.String())
	}
	var criado database.User
	database.DB.Where("username = ?", username).Take(&criado)

	patch := `{"nome":"Nome Depois","email":"depois@exemplo.com"}`
	rec = pedirComSessao(t, http.MethodPatch, "/api/users?id="+strconv.Itoa(int(criado.ID)), patch, sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH: status %d, esperado 200 (%s)", rec.Code, rec.Body.String())
	}

	var depois database.User
	database.DB.Where("id = ?", criado.ID).Take(&depois)
	if depois.Nome != "Nome Depois" || depois.Email != "depois@exemplo.com" {
		t.Errorf("banco tem nome=%q email=%q depois do PATCH", depois.Nome, depois.Email)
	}
}

func TestListaEMeDevolvemNomeEEmail(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-me-nome", auth.RoleAdmin)
	const username = "me-nome-email"
	limparUsuario(t, username)

	corpo := `{"username":"` + username + `","password":"senha-de-teste-1234",` +
		`"nome":"Pessoa Listada","email":"listada@exemplo.com"}`
	if rec := pedirComSessao(t, http.MethodPost, "/api/users", corpo, sess); rec.Code != http.StatusCreated {
		t.Fatalf("criar: status %d (%s)", rec.Code, rec.Body.String())
	}

	rec := pedirComSessao(t, http.MethodGet, "/api/users", "", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/users: status %d", rec.Code)
	}
	var lista []struct {
		Username string `json:"username"`
		Nome     string `json:"nome"`
		Email    string `json:"email"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &lista); err != nil {
		t.Fatalf("lista: %v", err)
	}
	achou := false
	for _, u := range lista {
		if u.Username == username {
			achou = true
			if u.Nome != "Pessoa Listada" || u.Email != "listada@exemplo.com" {
				t.Errorf("GET /api/users trouxe nome=%q email=%q", u.Nome, u.Email)
			}
		}
	}
	if !achou {
		t.Fatalf("usuário %q ausente do GET /api/users", username)
	}

	sessDoNovo, err := auth.Login(username, "senha-de-teste-1234")
	if err != nil {
		t.Fatalf("login do usuário novo: %v", err)
	}
	rec = pedirComSessao(t, http.MethodGet, "/api/auth/me", "", sessDoNovo)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/auth/me: status %d", rec.Code)
	}
	var eu struct {
		Nome  string `json:"nome"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &eu); err != nil {
		t.Fatalf("me: %v", err)
	}
	if eu.Nome != "Pessoa Listada" || eu.Email != "listada@exemplo.com" {
		t.Errorf("/api/auth/me trouxe nome=%q email=%q", eu.Nome, eu.Email)
	}
}
