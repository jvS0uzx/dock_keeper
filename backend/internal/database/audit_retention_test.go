package database

import (
	"os"
	"strings"
	"testing"
	"time"
)

// A tabela de métrica chama a coluna de "timestamp" e a auditoria a chama de
// "at". Um SQL com o nome cravado apagaria as linhas erradas, ou nenhuma.
func TestPruneBatchSQLUsaAColunaPedida(t *testing.T) {
	sql := pruneBatchSQL("audit_logs", "at")

	if !strings.Contains(sql, "WHERE at < ?") {
		t.Errorf("SQL não filtra por at: %s", sql)
	}
	if strings.Contains(sql, "timestamp") {
		t.Errorf("SQL da auditoria cita timestamp, coluna que a tabela não tem: %s", sql)
	}
	if !strings.Contains(sql, "LIMIT ?") {
		t.Errorf("SQL sem LIMIT: o DELETE volta a bloquear a tabela inteira: %s", sql)
	}
}

func setupAuditRetentionDB(t *testing.T) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de retenção da auditoria")
	}
	if DB == nil {
		if err := Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	limparAuditoriaDeTeste(t)
	t.Cleanup(func() { limparAuditoriaDeTeste(t) })
}

func limparAuditoriaDeTeste(t *testing.T) {
	t.Helper()
	DB.Where("action LIKE ?", "retencao-teste.%").Delete(&AuditLog{})
}

func contarAuditoria(t *testing.T, action string) int64 {
	t.Helper()

	var n int64
	if err := DB.Model(&AuditLog{}).Where("action = ?", action).Count(&n).Error; err != nil {
		t.Fatalf("contar %q: %v", action, err)
	}
	return n
}

// O prazo da auditoria é próprio e muito mais longo que o das métricas: métrica
// de duas semanas atrás não responde nada, auditoria antiga é o que se consulta
// depois de um incidente descoberto meses depois.
func TestPodaDaAuditoriaRespeitaOPrazoProprio(t *testing.T) {
	setupAuditRetentionDB(t)

	agora := time.Now().UTC()
	linhas := []AuditLog{
		{At: agora.Add(-400 * 24 * time.Hour), Action: "retencao-teste.antiga", Result: "ok", Detail: "{}"},
		{At: agora.Add(-30 * 24 * time.Hour), Action: "retencao-teste.recente", Result: "ok", Detail: "{}"},
	}
	if err := DB.Create(&linhas).Error; err != nil {
		t.Fatalf("criar linhas de teste: %v", err)
	}

	pruneAuditLog(365 * 24 * time.Hour)

	if n := contarAuditoria(t, "retencao-teste.antiga"); n != 0 {
		t.Errorf("linhas antigas = %d, esperada nenhuma", n)
	}
	if n := contarAuditoria(t, "retencao-teste.recente"); n != 1 {
		t.Errorf("linhas recentes = %d, esperada 1: a poda levou o que ainda vale", n)
	}
}

// A retenção de métrica é de dias. Se ela alcançasse a auditoria, a evidência
// sumiria antes de alguém saber que precisava dela.
func TestRetencaoDeMetricaNaoAlcancaAAuditoria(t *testing.T) {
	setupAuditRetentionDB(t)

	agora := time.Now().UTC()
	linha := AuditLog{
		At: agora.Add(-30 * 24 * time.Hour), Action: "retencao-teste.recente", Result: "ok", Detail: "{}",
	}
	if err := DB.Create(&linha).Error; err != nil {
		t.Fatalf("criar linha de teste: %v", err)
	}

	// Sete dias de métrica, um ano de auditoria: os prazos que o painel usa.
	prune(7*24*time.Hour, 365*24*time.Hour)

	if n := contarAuditoria(t, "retencao-teste.recente"); n != 1 {
		t.Errorf("linhas = %d, esperada 1: a poda de métrica levou a auditoria junto", n)
	}
}
