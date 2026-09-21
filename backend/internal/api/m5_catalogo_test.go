package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func TestCatalogoDeMetricasEOContratoDaTela(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "viewer-catalogo", auth.RoleViewer)

	rec := pedirComSessao(t, http.MethodGet, "/api/metrics/catalogo", "", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/metrics/catalogo: status %d (%s)", rec.Code, rec.Body.String())
	}
	var catalogo []struct {
		Nome         string `json:"nome"`
		Rotulo       string `json:"rotulo"`
		Unidade      string `json:"unidade"`
		TemTendencia *bool  `json:"tem_tendencia"`
		Escopo       string `json:"escopo"`
		EmRegra      *bool  `json:"em_regra"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &catalogo); err != nil {
		t.Fatalf("catálogo: %v", err)
	}

	porNome := map[string]int{}
	for i, m := range catalogo {
		porNome[m.Nome] = i
		if m.Rotulo == "" || m.Escopo == "" || m.TemTendencia == nil || m.EmRegra == nil {
			t.Errorf("métrica %q veio manca: %+v", m.Nome, m)
		}
	}
	for _, nome := range []string{"cpu", "mem", "disk", "load", "temperature", "net_rx", "net_tx", "rtt", "latency"} {
		if _, ok := porNome[nome]; !ok {
			t.Errorf("catálogo sem %q", nome)
		}
	}
	if l := catalogo[porNome["latency"]]; *l.TemTendencia || *l.EmRegra {
		t.Errorf("latency = %+v, esperado sem tendência e fora das regras", l)
	}
	if c := catalogo[porNome["cpu"]]; !*c.TemTendencia || c.Escopo != "ambos" || c.Unidade != "%" {
		t.Errorf("cpu = %+v, esperado com tendência, escopo ambos, unidade %%", c)
	}
}

func TestLiveDevolveAJanelaDoBalanceadorEEnxergaAgenteLento(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-live-m5", auth.RoleAdmin)
	srv := servidorDeRename(t, "m5-agente-de-5-min", "203.0.113.63", nil)
	database.DB.Model(&srv).Updates(map[string]any{"kind": "agent", "report_interval_sec": 300})
	t.Cleanup(func() { database.DB.Where("server_id = ?", srv.ID).Delete(&database.MetricServer{}) })
	cpu := 37.0
	database.DB.Create(&database.MetricServer{ServerID: srv.ID, CPUUsagePercent: &cpu, Timestamp: time.Now().UTC().Add(-12 * time.Minute)})

	rec := pedirComSessao(t, http.MethodGet, "/api/metrics/live", "", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("live: status %d", rec.Code)
	}
	var live struct {
		LBWindowSec *int `json:"lb_window_sec"`
		Servers     []struct {
			ID     string   `json:"id"`
			Online bool     `json:"online"`
			CPU    *float64 `json:"cpu"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &live); err != nil {
		t.Fatalf("live: %v", err)
	}
	if live.LBWindowSec == nil || *live.LBWindowSec != 5 {
		t.Errorf("lb_window_sec = %v, esperado 5: a tela escreve \"req / 5s\" por coincidência", live.LBWindowSec)
	}
	for _, s := range live.Servers {
		if s.ID == srv.ID {
			if !s.Online || s.CPU == nil || *s.CPU != 37 {
				t.Errorf("agente de 5 min com amostra de 12 min: online=%v cpu=%v, esperado online com 37 (janela de 15 min)", s.Online, s.CPU)
			}
			return
		}
	}
	t.Error("o live não trouxe o agente")
}
