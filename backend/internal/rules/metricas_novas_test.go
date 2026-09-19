package rules

import (
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func TestMetricValueAceitaTemperaturaERede(t *testing.T) {
	temp, rx, tx := 81.5, 1200.0, 300.0
	m := database.MetricServer{TemperatureC: &temp, NetRxBps: &rx, NetTxBps: &tx}

	esperado := map[string]float64{"temperature": 81.5, "net_rx": 1200, "net_tx": 300}
	for metrica, want := range esperado {
		got, ok := metricValue(m, metrica)
		if !ok || got != want {
			t.Errorf("metricValue(%s) = (%v, %v), esperado (%v, true)", metrica, got, ok, want)
		}
	}
}

func TestMetricaNulaNuncaDisparaNemViraZero(t *testing.T) {
	for _, metrica := range []string{"temperature", "net_rx", "net_tx"} {
		if got, ok := metricValue(database.MetricServer{}, metrica); ok {
			t.Errorf("metricValue(%s) sem medição = (%v, true); NULL não pode virar valor", metrica, got)
		}
	}
}

func TestMetricValueAceitaRTT(t *testing.T) {
	rtt := 42.0
	if got, ok := metricValue(database.MetricServer{RTTMs: &rtt}, "rtt"); !ok || got != 42 {
		t.Errorf("metricValue(rtt) = (%v, %v), esperado (42, true)", got, ok)
	}
	if _, ok := metricValue(database.MetricServer{}, "rtt"); ok {
		t.Error("rtt sem medição virou valor; NULL não dispara")
	}
}
