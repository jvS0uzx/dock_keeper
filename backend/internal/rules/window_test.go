package rules

import (
	"testing"
	"time"

	"github.com/joaov/vd_stats/internal/database"
)

func metrica(serverID string, idade time.Duration, agora time.Time) database.MetricServer {
	return database.MetricServer{ServerID: serverID, Timestamp: agora.Add(-idade)}
}

// O caso que motivou o E8: host reportando corretamente a cada 120 s ficava
// fora de toda avaliação porque a janela era 60 s fixos.
func TestRecentMetricsAceitaAgenteLento(t *testing.T) {
	agora := time.Now()
	servers := []database.Server{
		{ID: "srv-lento", ReportIntervalSec: 120},
	}
	latest := []database.MetricServer{metrica("srv-lento", 100*time.Second, agora)}

	recent := recentMetrics(latest, servers, agora)

	if _, ok := recent["srv-lento"]; !ok {
		t.Fatal("host com intervalo de 120s e métrica de 100s ficou fora da janela")
	}
}

// A correção não pode virar "aceita tudo": métrica velha continua velha, e um
// host calado precisa continuar suprimindo as regras que dependem dele.
func TestRecentMetricsDescartaMetricaVelha(t *testing.T) {
	agora := time.Now()
	servers := []database.Server{
		{ID: "srv-ssh", ReportIntervalSec: 0},
		{ID: "srv-lento", ReportIntervalSec: 120},
	}
	latest := []database.MetricServer{
		metrica("srv-ssh", 5*time.Minute, agora),
		metrica("srv-lento", 9*time.Minute, agora),
	}

	recent := recentMetrics(latest, servers, agora)

	if len(recent) != 0 {
		t.Fatalf("métricas velhas entraram na janela: %v", recent)
	}
}

// Intervalo desconhecido é a coleta por SSH, cujo ritmo o painel não sabe:
// fica no piso de 30 s, como antes.
func TestRecentMetricsIntervaloDesconhecidoUsaOPiso(t *testing.T) {
	agora := time.Now()
	servers := []database.Server{{ID: "srv-ssh", ReportIntervalSec: 0}}

	dentro := recentMetrics([]database.MetricServer{metrica("srv-ssh", 20*time.Second, agora)}, servers, agora)
	if _, ok := dentro["srv-ssh"]; !ok {
		t.Error("métrica de 20s ficou fora do piso de 30s")
	}

	fora := recentMetrics([]database.MetricServer{metrica("srv-ssh", 45*time.Second, agora)}, servers, agora)
	if _, ok := fora["srv-ssh"]; ok {
		t.Error("métrica de 45s passou pelo piso de 30s")
	}
}

// Métrica de servidor que não está mais cadastrado não pode manter viva a regra
// que aponta para ele.
func TestRecentMetricsIgnoraServidorDesconhecido(t *testing.T) {
	agora := time.Now()
	servers := []database.Server{{ID: "srv-1", ReportIntervalSec: 30}}
	latest := []database.MetricServer{
		metrica("srv-1", 10*time.Second, agora),
		metrica("srv-removido", 10*time.Second, agora),
	}

	recent := recentMetrics(latest, servers, agora)

	if _, ok := recent["srv-removido"]; ok {
		t.Error("métrica órfã de servidor removido entrou no mapa")
	}
	if len(recent) != 1 {
		t.Fatalf("mapa = %v, esperado só srv-1", recent)
	}
}

// Cada host responde pela própria janela: um agente rápido calado não pode ser
// salvo pela tolerância de um agente lento no mesmo tick.
func TestRecentMetricsJanelaEPorHost(t *testing.T) {
	agora := time.Now()
	servers := []database.Server{
		{ID: "srv-rapido", ReportIntervalSec: 10},
		{ID: "srv-lento", ReportIntervalSec: 120},
	}
	latest := []database.MetricServer{
		metrica("srv-rapido", 90*time.Second, agora),
		metrica("srv-lento", 90*time.Second, agora),
	}

	recent := recentMetrics(latest, servers, agora)

	if _, ok := recent["srv-rapido"]; ok {
		t.Error("agente de 10s calado há 90s continuou contando como recente")
	}
	if _, ok := recent["srv-lento"]; !ok {
		t.Error("agente de 120s com métrica de 90s ficou fora da janela dele")
	}
}
