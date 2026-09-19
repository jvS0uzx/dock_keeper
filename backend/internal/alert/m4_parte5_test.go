package alert

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/observabilidade"
)

func TestFalhaAoEnfileirarDescartaSemChamarOTelegram(t *testing.T) {
	falso := setupFila(t)

	original := database.DB
	quebrado := original.Session(&gorm.Session{NewDB: true}).Table("tabela_que_nao_existe_alerts")
	database.DB = quebrado
	t.Cleanup(func() { database.DB = original })

	antes := observabilidade.AlertasDescartados.Value()
	if Notify(prefixoFila+"insert-quebrado", "[CRITICO] banco fora") {
		t.Error("Notify devolveu true com o INSERT falhando")
	}

	if n := observabilidade.AlertasDescartados.Value(); n != antes+1 {
		t.Errorf("contador de descartados = %d, esperado %d", n, antes+1)
	}
	if n := falso.enviosCom("[CRITICO] banco fora"); n != 0 {
		t.Errorf("o Telegram foi chamado %d vez(es) no caminho de quem alerta", n)
	}
}

func TestPodaTiraResolvidoVelhoEDeixaAberto(t *testing.T) {
	setupFila(t)
	t.Setenv("ALERT_RETENTION_DAYS", "30")

	velho := time.Now().UTC().Add(-60 * 24 * time.Hour)
	linhas := []database.Alert{
		{Key: prefixoFila + "poda-resolvido", Severity: "info", Text: "resolvido velho",
			Status: database.AlertStatusResolved, CreatedAt: velho, Delivery: database.AlertDeliveryEnviado},
		{Key: prefixoFila + "poda-aberto", Severity: "critical", Text: "aberto velho",
			Status: database.AlertStatusOpen, CreatedAt: velho, Delivery: database.AlertDeliveryEnviado},
		{Key: prefixoFila + "poda-recente", Severity: "info", Text: "resolvido recente",
			Status: database.AlertStatusResolved, CreatedAt: time.Now().UTC(), Delivery: database.AlertDeliveryEnviado},
	}
	if err := database.DB.Create(&linhas).Error; err != nil {
		t.Fatalf("criar alertas: %v", err)
	}

	database.PruneAlertsParaTeste(30 * 24 * time.Hour)

	for _, caso := range []struct {
		chave string
		quer  int
	}{
		{prefixoFila + "poda-resolvido", 0},
		{prefixoFila + "poda-aberto", 1},
		{prefixoFila + "poda-recente", 1},
	} {
		if n := len(alertasDaChave(t, caso.chave)); n != caso.quer {
			t.Errorf("%s: %d linha(s), esperado %d", caso.chave, n, caso.quer)
		}
	}
}
