package auth

import (
	"errors"
	"os"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	timingUserAtivo     = "teste-timing-ativo"
	timingUserInativo   = "teste-timing-inativo"
	timingSenhaCorreta  = "senha-de-teste-1234"
	timingSenhaIncorret = "senha-errada-9876"
)

func setupTimingDB(t *testing.T) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de login")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	limparUsuariosDeTiming(t)
	t.Cleanup(func() { limparUsuariosDeTiming(t) })

	hash, err := HashPassword(timingSenhaCorreta)
	if err != nil {
		t.Fatalf("hash da senha de teste: %v", err)
	}
	usuarios := []database.User{
		{Username: timingUserAtivo, PasswordHash: hash, Role: RoleViewer, Active: true},
		{Username: timingUserInativo, PasswordHash: hash, Role: RoleViewer, Active: true},
	}
	if err := database.DB.Create(&usuarios).Error; err != nil {
		t.Fatalf("criar usuários de teste: %v", err)
	}
	if err := database.DB.Model(&database.User{}).
		Where("username = ?", timingUserInativo).
		Update("active", false).Error; err != nil {
		t.Fatalf("desativar usuário de teste: %v", err)
	}
}

func limparUsuariosDeTiming(t *testing.T) {
	t.Helper()
	nomes := []string{timingUserAtivo, timingUserInativo}
	database.DB.Unscoped().Where("username IN ?", nomes).Delete(&database.User{})
}

func TestContaDesativadaComSenhaErradaNaoSeDistingue(t *testing.T) {
	setupTimingDB(t)

	_, err := Login(timingUserInativo, timingSenhaIncorret)
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("conta desativada com senha errada devolveu %v, esperado ErrInvalidCredentials", err)
	}
}

func TestContaDesativadaComSenhaCertaContinuaRecusada(t *testing.T) {
	setupTimingDB(t)

	_, err := Login(timingUserInativo, timingSenhaCorreta)
	if !errors.Is(err, ErrUserInactive) {
		t.Errorf("conta desativada com senha certa devolveu %v, esperado ErrUserInactive", err)
	}
}

func TestContaAtivaContinuaEntrando(t *testing.T) {
	setupTimingDB(t)

	sess, err := Login(timingUserAtivo, timingSenhaCorreta)
	if err != nil {
		t.Fatalf("conta ativa com senha certa foi recusada: %v", err)
	}
	if sess.Token == "" {
		t.Error("login bem-sucedido não devolveu token de sessão")
	}
	Logout(sess.Token)
}
