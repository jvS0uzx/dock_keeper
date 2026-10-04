package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const corpoDouradoDasMetricas = `{"schema":1,"hostname":"estacao-01","cpu":12.5,"mem_used":1024,"mem_total":4096,"load1":0.75,"disk_used":2048,"disk_total":8192,"uptime":3600,"temperature_c":41.5,"net_rx_bps":100,"net_tx_bps":200,"addresses":["10.0.0.5"],"os":"linux","platform":"debian 12","arch":"amd64","logged_user":"joao","site_code":"matriz","agent_version":"1.0.0","machine_id":"abc123","report_interval_sec":5}`

func TestCollectDeclaraOEsquemaDasMetricas(t *testing.T) {
	capturarLog(t)
	if p := collect("estacao-01", "matriz", 5); p.Schema != 1 {
		t.Errorf("collect declarou schema %d, esperado 1", p.Schema)
	}
}

func TestCorpoEnviadoPeloAgenteConfereComODourado(t *testing.T) {
	capturarLog(t)
	cpu, load, temp, rx, tx := 12.5, 0.75, 41.5, 100.0, 200.0
	coletado := collect("estacao-01", "matriz", 5)
	fixo := metricsPayload{
		Schema: coletado.Schema, Hostname: "estacao-01", CPU: &cpu, MemUsed: 1024, MemTotal: 4096,
		Load1: &load, DiskUsed: 2048, DiskTotal: 8192, Uptime: 3600, TemperatureC: &temp,
		NetRxBps: &rx, NetTxBps: &tx, Addresses: []string{"10.0.0.5"},
		OS: "linux", Platform: "debian 12", Arch: "amd64", LoggedUser: "joao", SiteCode: "matriz",
		AgentVersion: "1.0.0", MachineID: "abc123", ReportIntervalSec: 5,
	}

	corpos := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		select {
		case corpos <- b:
		default:
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	a := agenteDeTeste(srv.URL, 10*time.Millisecond)
	a.coletar = func() metricsPayload { return fixo }
	var recebido []byte
	go func() {
		recebido = <-corpos
		cancel()
	}()
	esperarRun(t, a, ctx, 2*time.Second)

	var compacto bytes.Buffer
	if err := json.Compact(&compacto, recebido); err != nil {
		t.Fatalf("corpo enviado não é JSON: %v\n%s", err, recebido)
	}
	if compacto.String() != corpoDouradoDasMetricas {
		t.Errorf("corpo enviado mudou:\n got %s\nwant %s", compacto.String(), corpoDouradoDasMetricas)
	}
}
