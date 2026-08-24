package audit

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/joaov/vd_stats/internal/database"
)

func setupAuditDB(t *testing.T) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de auditoria")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	limpar(t)
	t.Cleanup(func() { limpar(t) })
}

func limpar(t *testing.T) {
	t.Helper()
	database.DB.Where("action LIKE ?", "teste.%").Delete(&database.AuditLog{})
}

// A coluna é jsonb: string vazia não é documento JSON válido, e o INSERT
// inteiro falharia por causa de um campo acessório — levando embora o registro
// da ação junto com o detalhe.
func TestDetalheVazioGravaObjetoVazio(t *testing.T) {
	setupAuditDB(t)

	id := Record(Entry{Action: "teste.sem-detalhe", Result: ResultOK})
	if id == 0 {
		t.Fatal("a linha não foi gravada")
	}

	var row database.AuditLog
	if err := database.DB.First(&row, id).Error; err != nil {
		t.Fatalf("reler a linha: %v", err)
	}
	if row.Detail != "{}" {
		t.Errorf("detalhe = %q, esperado {}", row.Detail)
	}
}

func TestDetalheSobreviveAoRoundTrip(t *testing.T) {
	setupAuditDB(t)

	id := Record(Entry{
		Action: "teste.com-detalhe",
		Result: ResultOK,
		Detail: map[string]any{"metodo": "POST", "rota": "/api/servers"},
	})
	if id == 0 {
		t.Fatal("a linha não foi gravada")
	}

	var row database.AuditLog
	if err := database.DB.First(&row, id).Error; err != nil {
		t.Fatalf("reler a linha: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(row.Detail), &got); err != nil {
		t.Fatalf("detalhe gravado não é JSON: %q (%v)", row.Detail, err)
	}
	if got["metodo"] != "POST" || got["rota"] != "/api/servers" {
		t.Errorf("detalhe = %v, esperado metodo=POST e rota=/api/servers", got)
	}
}

// Complete precisa MESCLAR: o que se sabia antes da execução — alvo,
// argumentos — continua valendo depois dela, e um Update cru apagaria tudo.
func TestCompleteMesclaODetalheAnterior(t *testing.T) {
	setupAuditDB(t)

	id := Record(Entry{
		Action: "teste.pendente",
		Result: ResultPending,
		Detail: map[string]any{"container": "nginx_proxy"},
	})
	if id == 0 {
		t.Fatal("a linha não foi gravada")
	}

	Complete(id, ResultError, map[string]any{"erro": "exit status 1"})

	var row database.AuditLog
	if err := database.DB.First(&row, id).Error; err != nil {
		t.Fatalf("reler a linha: %v", err)
	}
	if row.Result != ResultError {
		t.Errorf("resultado = %q, esperado %q", row.Result, ResultError)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(row.Detail), &got); err != nil {
		t.Fatalf("detalhe gravado não é JSON: %q (%v)", row.Detail, err)
	}
	if got["container"] != "nginx_proxy" {
		t.Errorf("o detalhe anterior foi perdido: %v", got)
	}
	if got["erro"] != "exit status 1" {
		t.Errorf("o detalhe do resultado não foi gravado: %v", got)
	}
}

// Um User-Agent absurdo não pode fazer o INSERT estourar o size da coluna e
// levar embora o registro da ação junto.
func TestUserAgentGiganteNaoDerrubaORegistro(t *testing.T) {
	setupAuditDB(t)

	gigante := make([]byte, 4000)
	for i := range gigante {
		gigante[i] = 'a'
	}

	id := Record(Entry{Action: "teste.ua-gigante", Result: ResultOK, UserAgent: string(gigante)})
	if id == 0 {
		t.Fatal("a linha não foi gravada: o User-Agent derrubou o registro")
	}
}

// Sem resultado explícito a linha nasce pendente, nunca em branco: "" não
// distinguiria "ainda executando" de "não sabemos".
func TestResultadoVazioViraPendente(t *testing.T) {
	setupAuditDB(t)

	id := Record(Entry{Action: "teste.sem-resultado"})
	if id == 0 {
		t.Fatal("a linha não foi gravada")
	}

	var row database.AuditLog
	if err := database.DB.First(&row, id).Error; err != nil {
		t.Fatalf("reler a linha: %v", err)
	}
	if row.Result != ResultPending {
		t.Errorf("resultado = %q, esperado %q", row.Result, ResultPending)
	}
}
