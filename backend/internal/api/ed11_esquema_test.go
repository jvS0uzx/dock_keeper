package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const corpoDouradoDoAgente = `{"schema":1,"hostname":"estacao-ed11","cpu":12.5,"mem_used":1024,"mem_total":4096,"load1":0.75,"disk_used":2048,"disk_total":8192,"uptime":3600,"temperature_c":41.5,"net_rx_bps":100,"net_tx_bps":200,"addresses":["10.0.0.5"],"os":"linux","platform":"debian 12","arch":"amd64","logged_user":"joao","site_code":"qa-enroll-a","agent_version":"1.0.0","machine_id":"abc123-ed11","report_interval_sec":5}`

func corpoDeMetricaComEsquema(esquema string) string {
	return `{` + esquema + `"hostname":"estacao-ed11","site_code":"` + codigoFilialA + `","cpu":10,"mem_total":100,"mem_used":10}`
}

func corpoDeInventarioComEsquema(esquema string) string {
	return `{` + esquema + `"site_code":"` + codigoFilialA + `","collector_version":"ed11","hosts":[{"ip":"192.168.78.10","hostname":"impressora-ed11"}]}`
}

func TestIngestaoAceitaEsquemaLegadoEUmERecusaOResto(t *testing.T) {
	sedeA, _ := setupEnrollDB(t)
	t.Cleanup(func() {
		database.DB.Where("ip = ?", "192.168.78.10").Delete(&database.NetworkHost{})
	})
	agente := credencialDoTipo(t, sedeA, kindAgent, "maquina-ed11-agente")
	coletor := credencialDoTipo(t, sedeA, kindCollector, "maquina-ed11-coletor")

	rotas := []struct {
		rota  string
		corpo func(string) string
		cred  credencialDeTeste
	}{
		{"/api/ingest/metrics", corpoDeMetricaComEsquema, agente},
		{"/api/ingest/inventory", corpoDeInventarioComEsquema, coletor},
	}
	casos := []struct {
		nome     string
		esquema  string
		status   int
		mensagem string
	}{
		{"ausente", ``, http.StatusOK, ""},
		{"zero", `"schema":0,`, http.StatusOK, ""},
		{"um", `"schema":1,`, http.StatusOK, ""},
		{"dois", `"schema":2,`, http.StatusBadRequest, "schema 2 não suportado; este painel aceita 1"},
		{"negativo", `"schema":-1,`, http.StatusBadRequest, "schema -1 não suportado; este painel aceita 1"},
	}
	for _, r := range rotas {
		for _, c := range casos {
			zerarLimiteDeIngestao()
			rec := enviarComCredencial(t, r.rota, r.corpo(c.esquema), r.cred)
			if rec.Code != c.status {
				t.Errorf("%s com schema %s: status %d, esperado %d (%s)", r.rota, c.nome, rec.Code, c.status, rec.Body.String())
				continue
			}
			if c.mensagem != "" && !strings.Contains(rec.Body.String(), c.mensagem) {
				t.Errorf("%s com schema %s: corpo %s sem %q", r.rota, c.nome, rec.Body.String(), c.mensagem)
			}
		}
	}

	var n int64
	database.DB.Model(&database.Server{}).Where("name = ?", "estacao-ed11").Count(&n)
	if n != 1 {
		t.Errorf("servidores estacao-ed11 = %d, esperado 1", n)
	}
}

func TestPainelAceitaOCorpoDouradoDoAgente(t *testing.T) {
	sedeA, _ := setupEnrollDB(t)
	agente := credencialDoTipo(t, sedeA, kindAgent, "abc123-ed11")

	rec := enviarComCredencial(t, "/api/ingest/metrics", corpoDouradoDoAgente, agente)
	if rec.Code != http.StatusOK {
		t.Fatalf("corpo dourado do agente: status %d (%s)", rec.Code, rec.Body.String())
	}
	var s database.Server
	if err := database.DB.Where("name = ?", "estacao-ed11").First(&s).Error; err != nil {
		t.Fatalf("servidor do corpo dourado não foi gravado: %v", err)
	}
	if s.AgentVersion != "1.0.0" || s.MachineID != "abc123-ed11" {
		t.Errorf("servidor gravado com agent_version %q machine_id %q", s.AgentVersion, s.MachineID)
	}
}
