package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/audit"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func enviarComTokenLegado(t *testing.T, rota, corpo, token string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, rota, strings.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerLegacyToken, token)
	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	return rec
}

func TestTokenLegadoDesligadoPorPadraoRecebe401ComOCaminhoDoConvite(t *testing.T) {
	setupAuditAPI(t)
	t.Setenv("AGENT_INGEST_TOKEN", "token-legado-b4")
	t.Setenv("ALLOW_LEGACY_INGEST_TOKEN", "")
	limpar := func() { database.DB.Unscoped().Where("name = ?", "estacao-b4").Delete(&database.Server{}) }
	limpar()
	t.Cleanup(limpar)

	casos := []struct {
		rota, corpo, acao, acaoDoMiddleware string
	}{
		{"/api/ingest/metrics", `{"hostname":"estacao-b4","cpu":1}`, "ingest.legacy_token_disabled", "ingest.metrics"},
		{"/api/ingest/inventory", `{"site_code":"qa-b4","hosts":[]}`, "inventory.legacy_token_disabled", "ingest.inventory"},
	}
	for _, c := range casos {
		rec := enviarComTokenLegado(t, c.rota, c.corpo, "token-legado-b4")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s com token legado desligado: status %d, esperado 401 (%s)", c.rota, rec.Code, rec.Body.String())
			continue
		}
		if !strings.Contains(rec.Body.String(), "/api/enroll") {
			t.Errorf("%s: a recusa %q não aponta o convite (/api/enroll)", c.rota, rec.Body.String())
		}

		linhas := linhasDe(t, c.acao)
		if len(linhas) != 1 || linhas[0].Result != audit.ResultDenied {
			t.Errorf("%s: auditoria %q = %d linha(s), esperada 1 recusa", c.rota, c.acao, len(linhas))
		} else if linhas[0].SourceIP == "" {
			t.Errorf("%s: a recusa não registrou o IP de origem, que é como se acha a estação antiga", c.rota)
		}
		if n := len(linhasDe(t, c.acaoDoMiddleware)); n != 0 {
			t.Errorf("%s: o middleware gravou %d linha(s) extra(s) de %q", c.rota, n, c.acaoDoMiddleware)
		}
	}

	var n int64
	database.DB.Unscoped().Model(&database.Server{}).Where("name = ?", "estacao-b4").Count(&n)
	if n != 0 {
		t.Errorf("o envio recusado criou %d servidor(es)", n)
	}
}

func TestTokenLegadoLigadoSegueAceito(t *testing.T) {
	setupAuditAPI(t)
	t.Setenv("AGENT_INGEST_TOKEN", "token-legado-b4")
	t.Setenv("ALLOW_LEGACY_INGEST_TOKEN", "true")
	t.Cleanup(func() { database.DB.Unscoped().Where("name = ?", "estacao-b4-ok").Delete(&database.Server{}) })

	rec := enviarComTokenLegado(t, "/api/ingest/metrics", `{"hostname":"estacao-b4-ok","cpu":1}`, "token-legado-b4")
	if rec.Code != http.StatusOK {
		t.Errorf("com ALLOW_LEGACY_INGEST_TOKEN=true: status %d, esperado 200 (%s)", rec.Code, rec.Body.String())
	}
}

func TestAllowLegacyIngestTokenSoLigaComVerdadeiro(t *testing.T) {
	casos := map[string]bool{"": false, "false": false, "talvez": false, "true": true, "1": true}
	for valor, esperado := range casos {
		t.Setenv("ALLOW_LEGACY_INGEST_TOKEN", valor)
		if got := legacyIngestAllowed(); got != esperado {
			t.Errorf("ALLOW_LEGACY_INGEST_TOKEN=%q: ligado = %v, esperado %v", valor, got, esperado)
		}
	}
}
