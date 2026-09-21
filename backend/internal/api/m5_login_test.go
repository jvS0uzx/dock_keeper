package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func tentarLogin(cfg Config, identificador, senha string) int {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"username":"`+identificador+`","password":"`+senha+`"}`))
	rec := httptest.NewRecorder()
	cfg.loginHandler(rec, req)
	return rec.Code
}

func TestLimitadorContaUsernameEEmailComoAMesmaConta(t *testing.T) {
	setupAuditAPI(t)
	limparUsuario(t, "b1-conta")
	hash, err := auth.HashPassword("senha-certa-b1-1234")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u := database.User{Username: "b1-conta", Email: "b1@exemplo.com", PasswordHash: hash, Role: auth.RoleViewer, Active: true}
	if err := database.DB.Create(&u).Error; err != nil {
		t.Fatalf("criar usuário: %v", err)
	}

	cfg := testConfig()
	cfg.logins = newLoginLimiter(15*time.Minute, 100, 4)

	for range 2 {
		tentarLogin(cfg, "b1-conta", "errada")
		tentarLogin(cfg, "B1@exemplo.com", "errada")
	}
	if status := tentarLogin(cfg, "b1@exemplo.com", "senha-certa-b1-1234"); status != http.StatusTooManyRequests {
		t.Errorf("4 falhas na mesma conta (2 pelo username, 2 pelo e-mail) e a 5ª tentativa respondeu %d, esperado 429", status)
	}

	for range 4 {
		tentarLogin(cfg, "b1-ninguem", "errada")
	}
	if status := tentarLogin(cfg, "b1-ninguem", "errada"); status != http.StatusTooManyRequests {
		t.Errorf("conta inexistente deixou de ser limitada: status %d", status)
	}
}

func TestUsernameNaoPodeSerOEmailDeOutroUsuario(t *testing.T) {
	c := setupC3(t)
	for _, nome := range []string{"b2-dono-do-email", "b2@exemplo.com", "b2-segundo", "b2-terceiro@exemplo.com"} {
		limparUsuario(t, nome)
	}
	criar := func(corpo string) int {
		return chamar(t, usersHandler, c.adminGlobal, http.MethodPost, "/api/users", corpo).Code
	}

	if status := criar(`{"username":"b2-dono-do-email","password":"senha-de-teste-1234","email":"b2@exemplo.com"}`); status != http.StatusCreated {
		t.Fatalf("preparação: status %d", status)
	}
	if status := criar(`{"username":"b2@exemplo.com","password":"senha-de-teste-1234"}`); status != http.StatusConflict {
		t.Errorf("username igual ao e-mail de outro usuário: status %d, esperado 409", status)
	}
	if status := criar(`{"username":"b2-segundo","password":"senha-de-teste-1234","email":"b2-dono-do-email@exemplo.com"}`); status != http.StatusCreated {
		t.Fatalf("preparação do segundo: status %d", status)
	}

	if status := criar(`{"username":"b2-terceiro@exemplo.com","password":"senha-de-teste-1234"}`); status != http.StatusCreated {
		t.Fatalf("preparação do terceiro: status %d", status)
	}
	var segundo database.User
	database.DB.Where("username = ?", "b2-segundo").Take(&segundo)
	rec := chamar(t, usersHandler, c.adminGlobal, http.MethodPatch, "/api/users?id="+uintStr(segundo.ID), `{"email":"b2-terceiro@exemplo.com"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("e-mail igual ao username de outro usuário na edição: status %d, esperado 409", rec.Code)
	}
}
