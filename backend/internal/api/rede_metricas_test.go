package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const srvRede = "00000000-0000-0000-0000-0000000000a4"

func setupRedeDB(t *testing.T) auth.Session {
	t.Helper()
	setupAuditAPI(t)

	limpar := func() {
		database.DB.Where("server_id = ?", srvRede).Delete(&database.MetricServer{})
		database.DB.Where("server_id = ?", srvRede).Delete(&database.MetricServerTrend{})
		database.DB.Unscoped().Where("id = ?", srvRede).Delete(&database.Server{})
		database.DB.Unscoped().Where("name = ?", "estacao-rede-a4").Delete(&database.Server{})
	}
	limpar()
	t.Cleanup(limpar)

	if err := database.DB.Create(&database.Server{ID: srvRede, Name: "host-rede-a4", HostIP: "203.0.113.44", Kind: "ssh"}).Error; err != nil {
		t.Fatalf("criar servidor: %v", err)
	}
	return sessaoReal(t, "admin-rede-a4", auth.RoleAdmin)
}

func ptrFloat(v float64) *float64 { return &v }

func TestIngestAceitaRedeOpcional(t *testing.T) {
	setupRedeDB(t)
	t.Setenv("AGENT_INGEST_TOKEN", "token-rede-a4")
	t.Setenv("ALLOW_LEGACY_INGEST_TOKEN", "true")

	enviar := func(corpo string) {
		req := httptest.NewRequest(http.MethodPost, "/api/ingest/metrics", strings.NewReader(corpo))
		req.Header.Set(headerLegacyToken, "token-rede-a4")
		rec := httptest.NewRecorder()
		Routes(testConfig()).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("ingest: status %d (%s)", rec.Code, rec.Body.String())
		}
	}
	enviar(`{"hostname":"estacao-rede-a4","cpu":1,"net_rx_bps":1234.5,"net_tx_bps":99}`)

	var srv database.Server
	if err := database.DB.Where("name = ?", "estacao-rede-a4").First(&srv).Error; err != nil {
		t.Fatalf("servidor do agente: %v", err)
	}
	var m database.MetricServer
	database.DB.Where("server_id = ?", srv.ID).Order("id desc").First(&m)
	if m.NetRxBps == nil || *m.NetRxBps != 1234.5 || m.NetTxBps == nil || *m.NetTxBps != 99 {
		t.Errorf("rede gravada = (%v, %v), esperado (1234.5, 99)", m.NetRxBps, m.NetTxBps)
	}

	enviar(`{"hostname":"estacao-rede-a4","cpu":1}`)
	var semRede database.MetricServer
	database.DB.Where("server_id = ?", srv.ID).Order("id desc").First(&semRede)
	if semRede.NetRxBps != nil || semRede.NetTxBps != nil {
		t.Errorf("envio sem rede gravou (%v, %v); ausente tem de ser NULL", semRede.NetRxBps, semRede.NetTxBps)
	}
	database.DB.Where("server_id = ?", srv.ID).Delete(&database.MetricServer{})
}

func historico(t *testing.T, sess auth.Session, metrica, faixa string) []historyPoint {
	t.Helper()

	rec := pedirComSessao(t, http.MethodGet, "/api/metrics/history?server_id="+srvRede+"&metric="+metrica+"&range="+faixa, "", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("history %s %s: status %d (%s)", metrica, faixa, rec.Code, rec.Body.String())
	}
	var pontos []historyPoint
	if err := json.Unmarshal(rec.Body.Bytes(), &pontos); err != nil {
		t.Fatalf("history: %v", err)
	}
	return pontos
}

func TestHistoricoDeRedeNaSerieBrutaENaTrend(t *testing.T) {
	sess := setupRedeDB(t)

	agora := time.Now().UTC()
	if err := database.DB.Create(&database.MetricServer{
		ServerID: srvRede, NetRxBps: ptrFloat(1000), NetTxBps: ptrFloat(250), Timestamp: agora.Add(-10 * time.Minute),
	}).Error; err != nil {
		t.Fatalf("criar amostra: %v", err)
	}
	if err := database.DB.Create(&database.MetricServerTrend{
		ServerID: srvRede, Bucket: agora.Add(-48 * time.Hour).Truncate(time.Hour), NetRxAvg: ptrFloat(500), NetTxAvg: ptrFloat(125),
	}).Error; err != nil {
		t.Fatalf("criar trend: %v", err)
	}

	casos := []struct {
		metrica, faixa string
		valor          float64
	}{
		{"net_rx", "1h", 1000}, {"net_tx", "1h", 250},
		{"net_rx", "7d", 500}, {"net_tx", "7d", 125},
	}
	for _, c := range casos {
		pontos := historico(t, sess, c.metrica, c.faixa)
		if len(pontos) != 1 || pontos[0].Value != c.valor {
			t.Errorf("%s em %s = %+v, esperado um ponto com %v bytes/s", c.metrica, c.faixa, pontos, c.valor)
		}
	}
}

func TestLiveTrazRedePorServidor(t *testing.T) {
	sess := setupRedeDB(t)

	if err := database.DB.Create(&database.MetricServer{
		ServerID: srvRede, CPUUsagePercent: ptrFloat(3), NetRxBps: ptrFloat(2048), NetTxBps: ptrFloat(512), Timestamp: time.Now().UTC(),
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
		if s["net_rx_bps"] != 2048.0 || s["net_tx_bps"] != 512.0 {
			t.Errorf("live trouxe net_rx_bps=%v net_tx_bps=%v, esperado 2048 e 512", s["net_rx_bps"], s["net_tx_bps"])
		}
		return
	}
	t.Fatal("servidor de teste ausente do /api/metrics/live")
}

func TestRegraAceitaMetricasNovas(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "operador-a3", auth.RoleOperator)
	t.Cleanup(func() { database.DB.Where("name LIKE ?", "regra-a3-%").Delete(&database.AlertRule{}) })

	for _, metrica := range []string{"temperature", "net_rx", "net_tx"} {
		rec := postComSessao(t, "/api/alerts/rules",
			`{"name":"regra-a3-`+metrica+`","metric":"`+metrica+`","operator":">","threshold":80}`, sess)
		if rec.Code != http.StatusCreated {
			t.Errorf("regra com métrica %s: status %d, esperado 201 (%s)", metrica, rec.Code, rec.Body.String())
		}
	}
}

func TestRTTNoHistoricoNoLiveENaRegra(t *testing.T) {
	sess := setupRedeDB(t)
	agora := time.Now().UTC()
	if err := database.DB.Create(&database.MetricServer{ServerID: srvRede, RTTMs: ptrFloat(18.5), Timestamp: agora.Add(-10 * time.Minute)}).Error; err != nil {
		t.Fatalf("amostra antiga: %v", err)
	}
	if err := database.DB.Create(&database.MetricServer{ServerID: srvRede, RTTMs: ptrFloat(21), Timestamp: agora}).Error; err != nil {
		t.Fatalf("amostra atual: %v", err)
	}
	if err := database.DB.Create(&database.MetricServerTrend{
		ServerID: srvRede, Bucket: agora.Add(-48 * time.Hour).Truncate(time.Hour), RTTAvg: ptrFloat(9), RTTMax: ptrFloat(15),
	}).Error; err != nil {
		t.Fatalf("trend: %v", err)
	}

	if p := historico(t, sess, "rtt", "7d"); len(p) != 1 || p[0].Value != 9 {
		t.Errorf("rtt em 7d = %+v, esperado um ponto com 9 ms", p)
	}
	if p := historico(t, sess, "rtt", "1h"); len(p) == 0 {
		t.Errorf("rtt em 1h veio vazio")
	}

	rec := pedirComSessao(t, http.MethodGet, "/api/metrics/live", "", sess)
	var corpo struct {
		Servers []map[string]any `json:"servers"`
	}
	json.Unmarshal(rec.Body.Bytes(), &corpo)
	achou := false
	for _, s := range corpo.Servers {
		if s["id"] == srvRede {
			achou = true
			if s["rtt_ms"] != 21.0 {
				t.Errorf("live trouxe rtt_ms=%v, esperado 21", s["rtt_ms"])
			}
		}
	}
	if !achou {
		t.Error("servidor de teste ausente do live")
	}

	t.Cleanup(func() { database.DB.Where("name = ?", "regra-e2-rtt").Delete(&database.AlertRule{}) })
	if rec := postComSessao(t, "/api/alerts/rules", `{"name":"regra-e2-rtt","metric":"rtt","operator":">","threshold":200}`, sess); rec.Code != http.StatusCreated {
		t.Errorf("regra com métrica rtt: status %d, esperado 201 (%s)", rec.Code, rec.Body.String())
	}
}
