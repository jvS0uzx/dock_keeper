package auth

import (
	"errors"
	"os"
	"testing"

	"github.com/joaov/vd_stats/internal/database"
)

const (
	timingUserAtivo     = "teste-timing-ativo"
	timingUserInativo   = "teste-timing-inativo"
	timingSenhaCorreta  = "senha-de-teste-1234"
	timingSenhaIncorret = "senha-errada-9876"
)

// setupTimingDB liga no Postgres de desenvolvimento. Sem DATABASE_URL o teste é
// pulado: a suíte precisa passar numa máquina sem banco.
//
// A cobertura é de integração porque Login consulta a tabela de usuários
// diretamente — a ordem entre o bcrypt e a conferência de Active não aparece
// num teste de unidade.
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
	// A desativação é um UPDATE separado porque User.Active tem `default:true`:
	// o GORM omite o campo do INSERT quando o valor é o zero de bool, e o banco
	// grava o padrão. Criar com Active: false devolve um usuário ativo.
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

// TestContaDesativadaComSenhaErradaNaoSeDistingue trava a correção do canal
// lateral de tempo: conferir Active antes do bcrypt respondia em microssegundos
// para conta desativada, enquanto senha errada e usuário inexistente gastavam os
// 60-100 ms do bcrypt de custo 10 — e esse intervalo revela que a conta existe.
//
// O erro devolvido é o observável que prova a ordem: com o bcrypt primeiro, quem
// erra a senha recebe ErrInvalidCredentials mesmo que a conta esteja desativada.
func TestContaDesativadaComSenhaErradaNaoSeDistingue(t *testing.T) {
	setupTimingDB(t)

	_, err := Login(timingUserInativo, timingSenhaIncorret)
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("conta desativada com senha errada devolveu %v, esperado ErrInvalidCredentials", err)
	}
}

// TestContaDesativadaComSenhaCertaContinuaRecusada garante que a correção não
// virou permissão: quem sabe a senha continua sabendo que a conta foi desativada,
// o que é informação que ele já tinha.
func TestContaDesativadaComSenhaCertaContinuaRecusada(t *testing.T) {
	setupTimingDB(t)

	_, err := Login(timingUserInativo, timingSenhaCorreta)
	if !errors.Is(err, ErrUserInactive) {
		t.Errorf("conta desativada com senha certa devolveu %v, esperado ErrUserInactive", err)
	}
}

// TestContaAtivaContinuaEntrando é o controle: sem ele os dois testes acima
// passariam com um Login que recusa tudo.
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
