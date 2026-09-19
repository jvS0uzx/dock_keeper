package auth

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	usuarioDeSessao = "teste-sessao-dona"
	senhaDeSessao   = "senha-de-sessao-1234"
	unidadeDeSessao = "teste-sessao-unidade"
)

func setupSessaoDB(t *testing.T) database.User {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de sessão")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	limparSessao(t)
	t.Cleanup(func() { limparSessao(t) })

	hash, err := HashPassword(senhaDeSessao)
	if err != nil {
		t.Fatalf("hash da senha: %v", err)
	}
	user := database.User{
		Username: usuarioDeSessao, PasswordHash: hash, Role: RoleViewer, Active: true,
	}
	if err := database.DB.Create(&user).Error; err != nil {
		t.Fatalf("criar a dona da sessão: %v", err)
	}
	return user
}

func limparSessao(t *testing.T) {
	t.Helper()

	var ids []uint
	database.DB.Model(&database.User{}).Where("username = ?", usuarioDeSessao).Pluck("id", &ids)
	if len(ids) > 0 {
		database.DB.Where("user_id IN ?", ids).Delete(&database.UserSession{})
		database.DB.Where("user_id IN ?", ids).Delete(&database.UserSiteAccess{})
	}
	database.DB.Unscoped().Where("username = ?", usuarioDeSessao).Delete(&database.User{})
	database.DB.Where("code = ?", unidadeDeSessao).Delete(&database.Site{})
}

func TestSessaoInseridaNoBancoResolveSemEstadoEmMemoria(t *testing.T) {
	user := setupSessaoDB(t)

	const token = "token-de-outro-processo-abcdef"
	agora := time.Now()
	row := database.UserSession{
		TokenHash:  tokenHash(token),
		UserID:     user.ID,
		Role:       RoleViewer,
		Username:   user.Username,
		ExpiresAt:  agora.Add(sessionTTL),
		CreatedAt:  agora,
		LastSeenAt: agora,
	}
	if err := database.DB.Create(&row).Error; err != nil {
		t.Fatalf("inserir a sessão: %v", err)
	}

	sess, ok := Lookup(token)
	if !ok {
		t.Fatal("sessão gravada por outro processo não resolveu: o login não sobrevive a um reinício")
	}
	if sess.UserID != user.ID || sess.Username != user.Username {
		t.Errorf("sessão resolvida = %d/%q, esperado %d/%q", sess.UserID, sess.Username, user.ID, user.Username)
	}
}

func TestTokenEmClaroNaoVaiParaOBanco(t *testing.T) {
	user := setupSessaoDB(t)

	sess, err := CreateSession(user.ID, user.Username, []Access{{SiteID: nil, Role: RoleViewer}})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	var row database.UserSession
	if err := database.DB.First(&row, "user_id = ?", user.ID).Error; err != nil {
		t.Fatalf("a sessão não foi gravada: %v", err)
	}
	for campo, valor := range map[string]string{
		"token_hash": row.TokenHash,
		"username":   row.Username,
		"role":       row.Role,
	} {
		if strings.Contains(valor, sess.Token) {
			t.Errorf("o token em claro vazou para a coluna %s", campo)
		}
	}
}

func TestConcessaoRemovidaValeNaRequisicaoSeguinte(t *testing.T) {
	user := setupSessaoDB(t)

	unidade := database.Site{Name: unidadeDeSessao, Code: unidadeDeSessao}
	if err := database.DB.Create(&unidade).Error; err != nil {
		t.Fatalf("criar unidade: %v", err)
	}
	concessao := database.UserSiteAccess{UserID: user.ID, SiteID: &unidade.ID, Role: RoleAdmin}
	if err := database.DB.Create(&concessao).Error; err != nil {
		t.Fatalf("criar concessão: %v", err)
	}

	sess, err := CreateSession(user.ID, user.Username, []Access{{SiteID: &unidade.ID, Role: RoleAdmin}})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	vivo, ok := Lookup(sess.Token)
	if !ok {
		t.Fatal("a sessão recém-criada não resolve")
	}
	if got := RoleForSite(vivo.Accesses, &unidade.ID); got != RoleAdmin {
		t.Fatalf("papel na unidade = %q, esperado %q antes da revogação", got, RoleAdmin)
	}

	if err := database.DB.Where("id = ?", concessao.ID).
		Delete(&database.UserSiteAccess{}).Error; err != nil {
		t.Fatalf("remover concessão: %v", err)
	}

	depois, ok := Lookup(sess.Token)
	if !ok {
		t.Fatal("a sessão morreu ao perder a concessão; deveria cair para o papel da conta")
	}
	if got := RoleForSite(depois.Accesses, &unidade.ID); got != RoleViewer {
		t.Errorf("papel na unidade = %q, esperado %q: a concessão revogada continuou valendo",
			got, RoleViewer)
	}
}

func TestSessaoDeUsuarioApagadoMorre(t *testing.T) {
	user := setupSessaoDB(t)

	sess, err := CreateSession(user.ID, user.Username, []Access{{SiteID: nil, Role: RoleViewer}})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, ok := Lookup(sess.Token); !ok {
		t.Fatal("a sessão recém-criada não resolve")
	}

	database.DB.Unscoped().Where("id = ?", user.ID).Delete(&database.User{})

	if _, ok := Lookup(sess.Token); ok {
		t.Error("a sessão sobreviveu à remoção da conta")
	}
}

func TestSessaoDeUsuarioDesativadoMorre(t *testing.T) {
	user := setupSessaoDB(t)

	sess, err := CreateSession(user.ID, user.Username, []Access{{SiteID: nil, Role: RoleViewer}})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	database.DB.Model(&database.User{}).Where("id = ?", user.ID).Update("active", false)

	if _, ok := Lookup(sess.Token); ok {
		t.Error("a sessão sobreviveu à desativação da conta")
	}
}

func TestSessaoVencidaEPodadaNoLoginSeguinte(t *testing.T) {
	user := setupSessaoDB(t)

	velha := database.UserSession{
		TokenHash: tokenHash("token-vencido-xyz"),
		UserID:    user.ID,
		Role:      RoleViewer,
		Username:  user.Username,
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	if err := database.DB.Create(&velha).Error; err != nil {
		t.Fatalf("inserir sessão vencida: %v", err)
	}

	if _, err := CreateSession(user.ID, user.Username, []Access{{SiteID: nil, Role: RoleViewer}}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	var n int64
	database.DB.Model(&database.UserSession{}).
		Where("token_hash = ?", velha.TokenHash).Count(&n)
	if n != 0 {
		t.Error("a sessão vencida continuou na tabela depois de um login novo")
	}
}

func TestSessaoVencidaNaoResolve(t *testing.T) {
	user := setupSessaoDB(t)

	const token = "token-que-ja-venceu-123"
	row := database.UserSession{
		TokenHash: tokenHash(token),
		UserID:    user.ID,
		Role:      RoleViewer,
		Username:  user.Username,
		ExpiresAt: time.Now().Add(-time.Minute),
	}
	if err := database.DB.Create(&row).Error; err != nil {
		t.Fatalf("inserir sessão vencida: %v", err)
	}

	if _, ok := Lookup(token); ok {
		t.Error("sessão vencida resolveu")
	}
}
