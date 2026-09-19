package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const srvC1 = "00000000-0000-0000-0000-0000000000c1"

func setupHistoricoC1(t *testing.T) auth.Session {
	t.Helper()
	setupAuditAPI(t)

	limpar := func() {
		database.DB.Where("server_id = ?", srvC1).Delete(&database.MetricServer{})
		database.DB.Where("server_id = ?", srvC1).Delete(&database.MetricServerTrend{})
	}
	limpar()
	t.Cleanup(limpar)
	return sessaoReal(t, "admin-historico-c1", auth.RoleAdmin)
}

func amostraBruta(t *testing.T, at time.Time, cpu float64) {
	t.Helper()
	if err := database.DB.Create(&database.MetricServer{ServerID: srvC1, CPUUsagePercent: &cpu, Timestamp: at}).Error; err != nil {
		t.Fatalf("criar amostra bruta: %v", err)
	}
}

func baldeDeTrend(t *testing.T, bucket time.Time, cpu float64) {
	t.Helper()
	if err := database.DB.Create(&database.MetricServerTrend{ServerID: srvC1, Bucket: bucket, CPUAvg: &cpu, Samples: 1}).Error; err != nil {
		t.Fatalf("criar balde de trend: %v", err)
	}
}

func pedirHistorico(t *testing.T, sess auth.Session, extra url.Values) (int, []historyPoint, string) {
	t.Helper()

	q := url.Values{"server_id": {srvC1}, "metric": {"cpu"}}
	for k, v := range extra {
		q[k] = v
	}
	rec := pedirComSessao(t, http.MethodGet, "/api/metrics/history?"+q.Encode(), "", sess)
	var pontos []historyPoint
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &pontos); err != nil {
			t.Fatalf("histórico: %v", err)
		}
	}
	return rec.Code, pontos, rec.Body.String()
}

func valores(pontos []historyPoint) []float64 {
	out := make([]float64, 0, len(pontos))
	for _, p := range pontos {
		out = append(out, p.Value)
	}
	return out
}

func TestHistorico30dLeDaTrend(t *testing.T) {
	sess := setupHistoricoC1(t)
	agora := time.Now().UTC()
	baldeDeTrend(t, agora.Add(-10*24*time.Hour).Truncate(time.Hour), 40)
	amostraBruta(t, agora.Add(-10*time.Minute), 99)

	code, pontos, corpo := pedirHistorico(t, sess, url.Values{"range": {"30d"}})
	if code != http.StatusOK {
		t.Fatalf("range=30d: status %d (%s)", code, corpo)
	}
	if v := valores(pontos); len(v) != 1 || v[0] != 40 {
		t.Errorf("range=30d = %v, esperado só o balde da trend (40), sem a amostra bruta (99)", v)
	}
}

func TestHistorico90dAgregaEmBaldesDeSeisHoras(t *testing.T) {
	sess := setupHistoricoC1(t)
	base := time.Unix((time.Now().Add(-20*24*time.Hour).Unix()/21600)*21600, 0).UTC()
	for i, cpu := range []float64{10, 20, 30, 40, 50, 60} {
		baldeDeTrend(t, base.Add(time.Duration(i)*time.Hour), cpu)
	}
	baldeDeTrend(t, base.Add(6*time.Hour), 90)

	code, pontos, corpo := pedirHistorico(t, sess, url.Values{"range": {"90d"}})
	if code != http.StatusOK {
		t.Fatalf("range=90d: status %d (%s)", code, corpo)
	}
	if v := valores(pontos); len(v) != 2 || v[0] != 35 || v[1] != 90 {
		t.Errorf("range=90d = %v, esperado [35 90]: seis horas viram um ponto só", v)
	}
	if len(pontos) == 2 && !pontos[0].Ts.Equal(base) {
		t.Errorf("o balde de 6 h começou em %s, esperado %s", pontos[0].Ts, base)
	}
}

func TestPeriodoCustomizadoCurtoLeDaSerieBrutaComLimiteSuperior(t *testing.T) {
	sess := setupHistoricoC1(t)
	agora := time.Now().UTC()
	amostraBruta(t, agora.Add(-2*time.Hour), 70)
	amostraBruta(t, agora.Add(-10*time.Minute), 99)
	baldeDeTrend(t, agora.Add(-2*time.Hour).Truncate(time.Hour), 11)

	code, pontos, corpo := pedirHistorico(t, sess, url.Values{
		"from": {agora.Add(-3 * time.Hour).Format(time.RFC3339)},
		"to":   {agora.Add(-1 * time.Hour).Format(time.RFC3339)},
	})
	if code != http.StatusOK {
		t.Fatalf("período de 2 h: status %d (%s)", code, corpo)
	}
	if v := valores(pontos); len(v) != 1 || v[0] != 70 {
		t.Errorf("período de 2 h = %v, esperado [70]: só a amostra bruta dentro de from/to", v)
	}
}

func TestPeriodoCustomizadoLongoLeDaTrend(t *testing.T) {
	sess := setupHistoricoC1(t)
	agora := time.Now().UTC()
	baldeDeTrend(t, agora.Add(-3*24*time.Hour).Truncate(time.Hour), 25)
	baldeDeTrend(t, agora.Add(-12*time.Hour).Truncate(time.Hour), 77)
	amostraBruta(t, agora.Add(-3*24*time.Hour), 99)

	code, pontos, corpo := pedirHistorico(t, sess, url.Values{
		"from": {agora.Add(-5 * 24 * time.Hour).Format(time.RFC3339)},
		"to":   {agora.Add(-24 * time.Hour).Format(time.RFC3339)},
	})
	if code != http.StatusOK {
		t.Fatalf("período de 4 dias: status %d (%s)", code, corpo)
	}
	if v := valores(pontos); len(v) != 1 || v[0] != 25 {
		t.Errorf("período de 4 dias = %v, esperado [25]: trend dentro de from/to, sem a bruta", v)
	}
}

func TestPeriodoCustomizadoSemToVaiAteAgora(t *testing.T) {
	sess := setupHistoricoC1(t)
	amostraBruta(t, time.Now().UTC().Add(-5*time.Minute), 42)

	code, pontos, corpo := pedirHistorico(t, sess, url.Values{
		"from": {time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)},
	})
	if code != http.StatusOK {
		t.Fatalf("from sem to: status %d (%s)", code, corpo)
	}
	if v := valores(pontos); len(v) != 1 || v[0] != 42 {
		t.Errorf("from sem to = %v, esperado [42]", v)
	}
}

func TestPeriodoCustomizadoInvalidoResponde400(t *testing.T) {
	sess := setupHistoricoC1(t)
	agora := time.Now().UTC()
	rfc := func(d time.Duration) string { return agora.Add(d).Format(time.RFC3339) }

	casos := map[string]url.Values{
		"range e from juntos": {"range": {"1h"}, "from": {rfc(-time.Hour)}},
		"from igual a to":     {"from": {rfc(-time.Hour)}, "to": {rfc(-time.Hour)}},
		"from depois de to":   {"from": {rfc(-time.Hour)}, "to": {rfc(-2 * time.Hour)}},
		"mais de 400 dias":    {"from": {rfc(-401 * 24 * time.Hour)}, "to": {rfc(0)}},
		"from inválido":       {"from": {"ontem"}},
		"to inválido":         {"from": {rfc(-time.Hour)}, "to": {"2026-13-40"}},
		"to sem from":         {"to": {rfc(0)}},
		"range desconhecido":  {"range": {"2y"}},
	}
	for nome, q := range casos {
		code, _, corpo := pedirHistorico(t, sess, q)
		if code != http.StatusBadRequest {
			t.Errorf("%s: status %d, esperado 400 (%s)", nome, code, corpo)
		}
	}
}
