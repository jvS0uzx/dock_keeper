package api

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestHistoricoLongoDeMetricaSemTendenciaResponde400(t *testing.T) {
	sess := setupHistoricoC1(t)

	casos := map[string]url.Values{
		"latency em 30d":   {"metric": {"latency"}, "range": {"30d"}},
		"latency em 90d":   {"metric": {"latency"}, "range": {"90d"}},
		"container em 30d": {"metric": {"cpu"}, "range": {"30d"}, "container_id": {"1"}},
	}
	for nome, q := range casos {
		status, _, corpo := pedirHistorico(t, sess, q)
		if status != http.StatusBadRequest || !strings.Contains(corpo, "7 dias") {
			t.Errorf("%s: status %d (%s), esperado 400 dizendo que só há 7 dias de histórico", nome, status, corpo)
		}
	}

	if status, _, corpo := pedirHistorico(t, sess, url.Values{"metric": {"latency"}, "range": {"7d"}}); status != http.StatusOK {
		t.Errorf("latency em 7d: status %d (%s), esperado 200", status, corpo)
	}
	if status, _, corpo := pedirHistorico(t, sess, url.Values{"range": {"30d"}}); status != http.StatusOK {
		t.Errorf("cpu em 30d: status %d (%s), esperado 200 pela tendência", status, corpo)
	}
}
