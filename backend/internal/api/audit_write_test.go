package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/joaov/vd_stats/internal/database"
	"gorm.io/gorm"
)

// senhaDeTeste é procurada literalmente na linha gravada. Precisa ser um valor
// improvável de aparecer por acaso.
const senhaDeTeste = "senha-que-nao-pode-vazar-8f3a21"

func setupAuditAPI(t *testing.T) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de auditoria")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	limparAuditoria(t)
	t.Cleanup(func() { limparAuditoria(t) })
}

// limparAuditoria isola cada teste do anterior.
//
// Apaga tudo, menos a família que o teste de retenção de internal/database
// semeia: os dois binários de teste rodam em paralelo contra o mesmo banco, e um
// DELETE cego levaria embora as linhas que aquele teste acabou de criar.
//
// Uma lista de ações exatas não serve: basta um teste vizinho gravar site.delete
// para a limpeza deixar passar o que a contagem enxerga.
// familiasDeOutrosPacotes são os prefixos de ação que a limpeza daqui precisa
// poupar.
//
// Os binários de teste de internal/api, internal/audit e internal/database
// rodam em paralelo contra o mesmo banco. Aqui a limpeza tem que ser cega — o
// middleware gera linha para rota que ele não conhece, com nome imprevisível —,
// e um DELETE realmente cego levaria junto as linhas que os outros pacotes
// semearam segundos antes, quebrando testes que não têm defeito nenhum.
var familiasDeOutrosPacotes = []string{
	"retencao-teste.%", // internal/database
	"teste.%",          // internal/audit
}

func limparAuditoria(t *testing.T) {
	t.Helper()

	tx := database.DB.Session(&gorm.Session{})
	for _, familia := range familiasDeOutrosPacotes {
		tx = tx.Where("action NOT LIKE ?", familia)
	}
	tx.Delete(&database.AuditLog{})
}

func linhasDe(t *testing.T, action string) []database.AuditLog {
	t.Helper()

	var linhas []database.AuditLog
	if err := database.DB.Where("action = ?", action).Order("id desc").Find(&linhas).Error; err != nil {
		t.Fatalf("consultar auditoria de %q: %v", action, err)
	}
	return linhas
}

// O teste que justifica a regra de allowlist: a senha vai no CORPO de
// POST /api/auth/login, e uma auditoria que copiasse o corpo viraria um
// depósito de senha em claro na mesma tabela que o administrador consulta.
func TestSenhaDoLoginNaoEntraNaAuditoria(t *testing.T) {
	setupAuditAPI(t)

	corpo := `{"username":"usuario-inexistente-do-teste","password":"` + senhaDeTeste + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)

	linhas := linhasDe(t, "auth.login")
	if len(linhas) == 0 {
		t.Fatal("a tentativa de login não foi auditada")
	}
	for _, l := range linhas {
		campos := []string{
			l.ActorUsername, l.ActorRole, l.SourceIP, l.UserAgent,
			l.Action, l.TargetType, l.TargetID, l.TargetLabel, l.Result, l.Detail,
		}
		for _, campo := range campos {
			if strings.Contains(campo, senhaDeTeste) {
				t.Fatalf("a senha vazou para a auditoria: %q", campo)
			}
		}
	}
}

// Login recusado é o sinal que a auditoria existe para capturar.
func TestLoginRecusadoGravaDenied(t *testing.T) {
	setupAuditAPI(t)

	corpo := `{"username":"usuario-inexistente-do-teste","password":"` + senhaDeTeste + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", rec.Code)
	}

	linhas := linhasDe(t, "auth.login")
	if len(linhas) != 1 {
		t.Fatalf("linhas gravadas = %d, esperada 1", len(linhas))
	}
	if linhas[0].Result != "denied" {
		t.Errorf("resultado = %q, esperado denied", linhas[0].Result)
	}
	if linhas[0].SourceIP == "" {
		t.Error("a linha não guardou o IP de origem, que é o que identifica quem tentou")
	}
}

// Escrita recusada por falta de credencial precisa deixar rastro: é a linha que
// revela alguém tentando alcançar o que não pode.
func TestEscritaSemCredencialGravaDenied(t *testing.T) {
	setupAuditAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/sites", strings.NewReader(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)

	linhas := linhasDe(t, "site.create")
	if len(linhas) != 1 {
		t.Fatalf("linhas gravadas = %d, esperada 1", len(linhas))
	}
	if linhas[0].Result != "denied" {
		t.Errorf("resultado = %q, esperado denied (status %d)", linhas[0].Result, rec.Code)
	}
}

// GET não pode gerar linha: é o polling do painel, e auditá-lo afogaria a
// tabela com o que ninguém consulta.
func TestLeituraNaoGeraLinha(t *testing.T) {
	setupAuditAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)

	var n int64
	if err := database.DB.Model(&database.AuditLog{}).
		Where("action LIKE ?", "site.%").Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("linhas gravadas para GET = %d, esperada nenhuma", n)
	}
}

// A ingestão é a exceção deliberada: milhares de push por minuto num parque de
// algumas centenas de hosts. Sucesso não gera linha.
func TestIngestaoBemSucedidaNaoGeraLinha(t *testing.T) {
	setupAuditAPI(t)
	t.Setenv("AGENT_INGEST_TOKEN", "token-de-teste-da-ingestao")

	corpo := `{"hostname":"host-de-teste-auditoria","cpu":1.0,"report_interval_sec":5}`
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/metrics", strings.NewReader(corpo))
	req.Header.Set("X-Agent-Token", "token-de-teste-da-ingestao")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200 (corpo: %s)", rec.Code, rec.Body.String())
	}

	if linhas := linhasDe(t, "ingest.metrics"); len(linhas) != 0 {
		t.Errorf("linhas gravadas = %d, esperada nenhuma: o sucesso da ingestão afogaria a tabela", len(linhas))
	}

	database.DB.Where("name = ?", "host-de-teste-auditoria").Delete(&database.Server{})
}

// A recusa da ingestão, ao contrário, é exatamente o que se quer ver: token
// inválido significa agente forjado ou credencial vazada.
func TestIngestaoRecusadaGeraLinha(t *testing.T) {
	setupAuditAPI(t)
	t.Setenv("AGENT_INGEST_TOKEN", "token-de-teste-da-ingestao")

	req := httptest.NewRequest(http.MethodPost, "/api/ingest/metrics", strings.NewReader(`{}`))
	req.Header.Set("X-Agent-Token", "token-errado")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", rec.Code)
	}

	linhas := linhasDe(t, "ingest.metrics")
	if len(linhas) != 1 {
		t.Fatalf("linhas gravadas = %d, esperada 1", len(linhas))
	}
	if linhas[0].Result != "denied" {
		t.Errorf("resultado = %q, esperado denied", linhas[0].Result)
	}
	if strings.Contains(linhas[0].Detail, "token") {
		t.Errorf("o detalhe carregou o token: %q", linhas[0].Detail)
	}
}
