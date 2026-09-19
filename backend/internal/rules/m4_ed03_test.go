package rules

import (
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func TestRegraDeCPUNaoDisparaComAmostraSemMedicao(t *testing.T) {
	setupMotorDB(t)

	regra := database.AlertRule{
		Name: prefixoRegra + "cpu-nula", Target: srvDuracao, Metric: "cpu",
		Operator: "<", Threshold: 90, Enabled: true, Severity: SeverityWarning,
	}
	if err := database.DB.Create(&regra).Error; err != nil {
		t.Fatalf("criar regra: %v", err)
	}

	if err := database.DB.Create(&database.MetricServer{
		ServerID: srvDuracao, MemUsedBytes: 10, MemTotalBytes: 100,
		Timestamp: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("gravar métrica sem cpu: %v", err)
	}

	buf := capturarLog(t)
	evaluate()

	if n := avisos(buf, "cpu-nula", "Regra"); n != 0 {
		t.Errorf("a regra cpu < 90 disparou %d vez(es) com cpu NULL: ausência de medição virou zero", n)
	}
}

func TestRegraDeCPUDisparaComZeroMedido(t *testing.T) {
	setupMotorDB(t)

	regra := database.AlertRule{
		Name: prefixoRegra + "cpu-zero", Target: srvDuracao, Metric: "cpu",
		Operator: "<", Threshold: 90, Enabled: true, Severity: SeverityWarning,
	}
	if err := database.DB.Create(&regra).Error; err != nil {
		t.Fatalf("criar regra: %v", err)
	}

	zero := 0.0
	if err := database.DB.Create(&database.MetricServer{
		ServerID: srvDuracao, CPUUsagePercent: &zero,
		Timestamp: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("gravar métrica com cpu zero: %v", err)
	}

	buf := capturarLog(t)
	evaluate()

	if n := avisos(buf, "cpu-zero", "Regra"); n == 0 {
		t.Error("a regra cpu < 90 não disparou com cpu 0 medido: máquina ociosa é medição")
	}
}
