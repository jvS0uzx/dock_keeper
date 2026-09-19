package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func metricasDoHost(t *testing.T, hostname string) []database.MetricServer {
	t.Helper()

	var servidor database.Server
	if err := database.DB.Where("name = ?", hostname).Take(&servidor).Error; err != nil {
		return nil
	}
	var metricas []database.MetricServer
	database.DB.Where("server_id = ?", servidor.ID).Order("id desc").Find(&metricas)
	return metricas
}

func TestIngestaoSemCPUGravaNuloEAceitaOResto(t *testing.T) {
	sedeA, _ := setupEnrollDB(t)
	cred := credencialDoTipo(t, sedeA, kindAgent, "maquina-m4-sem-cpu")

	const host = "estacao-m4-sem-cpu"
	rec := enviarComCredencial(t, "/api/ingest/metrics",
		`{"hostname":"`+host+`","site_code":"`+codigoFilialA+`","mem_total":100,"mem_used":10}`, cred)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200: a amostra sem cpu ainda traz memória, disco e rede (%s)",
			rec.Code, rec.Body.String())
	}
	metricas := metricasDoHost(t, host)
	if len(metricas) != 1 {
		t.Fatalf("%d amostra(s) gravadas para %q, esperada 1", len(metricas), host)
	}
	if metricas[0].CPUUsagePercent != nil {
		t.Errorf("cpu gravada = %v, esperado NULL", *metricas[0].CPUUsagePercent)
	}
	if metricas[0].LoadAvg1 != nil {
		t.Errorf("load gravado = %v, esperado NULL", *metricas[0].LoadAvg1)
	}
	if metricas[0].MemTotalBytes != 100 {
		t.Errorf("mem_total = %d, esperado 100", metricas[0].MemTotalBytes)
	}
}

func TestIngestaoComCPUZeroExplicitoEAceita(t *testing.T) {
	sedeA, _ := setupEnrollDB(t)
	cred := credencialDoTipo(t, sedeA, kindAgent, "maquina-m4-cpu-zero")

	const host = "estacao-m4-cpu-zero"
	rec := enviarComCredencial(t, "/api/ingest/metrics",
		`{"hostname":"`+host+`","site_code":"`+codigoFilialA+`","cpu":0,"load1":0,"mem_total":100,"mem_used":10}`, cred)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200 para máquina ociosa (%s)", rec.Code, rec.Body.String())
	}
	metricas := metricasDoHost(t, host)
	if len(metricas) != 1 {
		t.Fatalf("%d amostras gravadas, esperada 1", len(metricas))
	}
	if metricas[0].CPUUsagePercent == nil || *metricas[0].CPUUsagePercent != 0 {
		t.Errorf("cpu gravada = %v, esperado 0", metricas[0].CPUUsagePercent)
	}
	if metricas[0].LoadAvg1 == nil || *metricas[0].LoadAvg1 != 0 {
		t.Errorf("load gravado = %v, esperado 0", metricas[0].LoadAvg1)
	}
}

func TestLiveDevolveCPUNulaSemMedicao(t *testing.T) {
	sess := setupRedeDB(t)

	if err := database.DB.Create(&database.MetricServer{
		ServerID: srvRede, MemUsedBytes: 10, MemTotalBytes: 100, Timestamp: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("criar amostra: %v", err)
	}

	rec := pedirComSessao(t, http.MethodGet, "/api/metrics/live", "", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("live: status %d", rec.Code)
	}
	var corpo struct {
		Servers []map[string]any `json:"servers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("live: %v", err)
	}
	for _, s := range corpo.Servers {
		if s["id"] != srvRede {
			continue
		}
		if s["cpu"] != nil {
			t.Errorf("live trouxe cpu=%v, esperado null: sem medição não é zero", s["cpu"])
		}
		if s["load1"] != nil {
			t.Errorf("live trouxe load1=%v, esperado null", s["load1"])
		}
		return
	}
	t.Fatal("servidor de teste ausente do /api/metrics/live")
}

func TestHistoricoDeCPUPulaAmostraSemMedicao(t *testing.T) {
	sess := setupRedeDB(t)

	agora := time.Now().UTC()
	cpu := 40.0
	amostras := []database.MetricServer{
		{ServerID: srvRede, CPUUsagePercent: &cpu, Timestamp: agora.Add(-2 * time.Minute)},
		{ServerID: srvRede, Timestamp: agora.Add(-time.Minute)},
	}
	if err := database.DB.Create(&amostras).Error; err != nil {
		t.Fatalf("criar amostras: %v", err)
	}

	rec := pedirComSessao(t, http.MethodGet,
		"/api/metrics/history?server_id="+srvRede+"&metric=cpu&range=1h", "", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("histórico: status %d (%s)", rec.Code, rec.Body.String())
	}
	var pontos []struct {
		Value float64 `json:"value"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &pontos); err != nil {
		t.Fatalf("histórico: %v", err)
	}
	if len(pontos) != 1 {
		t.Fatalf("%d pontos, esperado 1: a amostra sem medição não vira ponto", len(pontos))
	}
	if pontos[0].Value != 40 {
		t.Errorf("valor = %v, esperado 40 (a amostra nula não pode puxar a média para baixo)", pontos[0].Value)
	}
}
