package auth

import (
	"errors"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func usuarioComEmail(t *testing.T, username, email, senha string) database.User {
	t.Helper()

	hash, err := HashPassword(senha)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	database.DB.Unscoped().Where("username = ?", username).Delete(&database.User{})
	u := database.User{Username: username, Email: email, PasswordHash: hash, Role: RoleAdmin, Active: true}
	if err := database.DB.Create(&u).Error; err != nil {
		t.Fatalf("criar usuário: %v", err)
	}
	t.Cleanup(func() {
		database.DB.Where("user_id = ?", u.ID).Delete(&database.UserSession{})
		database.DB.Unscoped().Where("username = ?", username).Delete(&database.User{})
	})
	return u
}

func TestLoginPorEmail(t *testing.T) {
	setupTimingDB(t)
	const senha = "senha-de-teste-1234"
	usuarioComEmail(t, "login-por-email", "pessoa@exemplo.com", senha)

	sess, err := Login("pessoa@exemplo.com", senha)
	if err != nil {
		t.Fatalf("login pelo e-mail falhou: %v", err)
	}
	if sess.Username != "login-por-email" {
		t.Errorf("sessão veio com username=%q", sess.Username)
	}
}

func TestLoginPorEmailIgnoraMaiuscula(t *testing.T) {
	setupTimingDB(t)
	const senha = "senha-de-teste-1234"
	usuarioComEmail(t, "login-email-maiuscula", "pessoa2@exemplo.com", senha)

	if _, err := Login("Pessoa2@Exemplo.com", senha); err != nil {
		t.Fatalf("login com e-mail em maiúscula falhou: %v", err)
	}
}

func TestLoginPorUsernameContinuaValendo(t *testing.T) {
	setupTimingDB(t)
	const senha = "senha-de-teste-1234"
	usuarioComEmail(t, "login-username", "pessoa3@exemplo.com", senha)

	if _, err := Login("login-username", senha); err != nil {
		t.Fatalf("login pelo username falhou: %v", err)
	}
}

func TestLoginDeUsuarioSemEmailNaoCasaComVazio(t *testing.T) {
	setupTimingDB(t)
	const senha = "senha-de-teste-1234"
	usuarioComEmail(t, "login-sem-email", "", senha)

	if _, err := Login("", senha); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("login com identificador vazio devolveu %v, esperado credencial inválida", err)
	}
}

func TestLoginComEmailInexistenteRecusa(t *testing.T) {
	setupTimingDB(t)

	if _, err := Login("ninguem@exemplo.com", "senha-de-teste-1234"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("e-mail inexistente devolveu %v, esperado credencial inválida", err)
	}
}
