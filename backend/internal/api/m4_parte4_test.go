package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/observabilidade"
)

func TestTokenDeMaquinaLeMasNaoEscrevePorPadrao(t *testing.T) {
	setupAuditAPI(t)

	if rec := comTokenDeMaquina(t, http.MethodGet, "/api/servers", ""); rec.Code != http.StatusOK {
		t.Errorf("leitura com token de máquina: status %d, esperado 200 (%s)", rec.Code, rec.Body.String())
	}

	rec := comTokenDeMaquina(t, http.MethodPost, "/api/sites", `{"name":"Unidade SG02","code":"sg02"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("escrita com token de máquina: status %d, esperado 403 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "API_TOKEN_ALLOW_WRITE") {
		t.Errorf("a recusa não explica como liberar: %s", rec.Body.String())
	}

	var n int64
	database.DB.Model(&database.Site{}).Where("code = ?", "sg02").Count(&n)
	if n != 0 {
		t.Errorf("a unidade foi criada apesar do 403")
	}
}

func TestTokenDeMaquinaEscreveComAVariavelLigadaEFicaNaAuditoria(t *testing.T) {
	setupAuditAPI(t)
	t.Setenv("API_TOKEN_ALLOW_WRITE", "true")
	t.Cleanup(func() {
		database.DB.Where("code = ?", "sg02-ok").Delete(&database.Site{})
		database.DB.Where("action = ? AND actor_username = ?", "site.create", "api-token").Delete(&database.AuditLog{})
	})

	rec := comTokenDeMaquina(t, http.MethodPost, "/api/sites", `{"name":"Unidade SG02 ok","code":"sg02-ok"}`)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("escrita liberada: status %d (%s)", rec.Code, rec.Body.String())
	}

	var linhas int64
	database.DB.Model(&database.AuditLog{}).
		Where("action = ? AND actor_username = ?", "site.create", "api-token").Count(&linhas)
	if linhas == 0 {
		t.Error("a escrita da máquina não gerou linha de auditoria")
	}
}

func TestIngestaoAcimaDoTetoRecebe429(t *testing.T) {
	sedeA, _ := setupEnrollDB(t)
	cred := credencialDoTipo(t, sedeA, kindAgent, "maquina-sg03")
	t.Setenv("INGEST_RATE_MAX", "3")
	t.Setenv("INGEST_RATE_WINDOW", "1m")

	corpo := `{"hostname":"estacao-sg03","site_code":"` + codigoFilialA + `","cpu":10,"mem_total":100,"mem_used":10}`
	for i := range 3 {
		if rec := enviarComCredencial(t, "/api/ingest/metrics", corpo, cred); rec.Code != http.StatusOK {
			t.Fatalf("envio %d: status %d, esperado 200 (%s)", i+1, rec.Code, rec.Body.String())
		}
	}

	rec := enviarComCredencial(t, "/api/ingest/metrics", corpo, cred)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("quarto envio: status %d, esperado 429 (%s)", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("a recusa por taxa não traz Retry-After")
	}

	ingestLimiter.mu.Lock()
	ingestLimiter.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	ingestLimiter.mu.Unlock()
	t.Cleanup(func() {
		ingestLimiter.mu.Lock()
		ingestLimiter.now = time.Now
		ingestLimiter.mu.Unlock()
	})

	if rec := enviarComCredencial(t, "/api/ingest/metrics", corpo, cred); rec.Code != http.StatusOK {
		t.Errorf("depois da janela: status %d, esperado 200 (%s)", rec.Code, rec.Body.String())
	}
}

func TestEnrollAcimaDoTetoRecebe429(t *testing.T) {
	setupEnrollDB(t)
	t.Setenv("INGEST_RATE_MAX_ENROLL", "2")
	t.Setenv("INGEST_RATE_WINDOW", "1m")

	ingestLimiter.mu.Lock()
	delete(ingestLimiter.failures, "enroll-ip:192.0.2.1")
	ingestLimiter.mu.Unlock()

	var ultimo *httptest.ResponseRecorder
	for range 3 {
		ultimo = chamarEnroll(t, "convite-que-nao-existe", "maquina-enroll-sg03")
	}
	if ultimo.Code != http.StatusTooManyRequests {
		t.Errorf("terceiro enroll: status %d, esperado 429 (%s)", ultimo.Code, ultimo.Body.String())
	}
}

func lerReadyz(t *testing.T) (int, map[string]any) {
	t.Helper()

	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	var corpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("corpo do /readyz: %v", err)
	}
	return rec.Code, corpo
}

func TestReadyzDizDegradadoComAlertaFalho(t *testing.T) {
	setupAuditAPI(t)
	t.Cleanup(func() { database.DB.Where("key = ?", "p4-falho").Delete(&database.Alert{}) })

	if _, corpo := lerReadyz(t); corpo["degradado"] != nil {
		t.Skipf("o painel já está degradado por outro motivo: %v", corpo["degradado"])
	}

	err := database.DB.Create(&database.Alert{
		Key: "p4-falho", Severity: "critical", Text: "[CRITICO] entrega falhou",
		Status: database.AlertStatusOpen, CreatedAt: time.Now().UTC(),
		Delivery: database.AlertDeliveryFalhou,
	}).Error
	if err != nil {
		t.Fatalf("criar alerta falho: %v", err)
	}

	code, corpo := lerReadyz(t)
	if code != http.StatusOK {
		t.Errorf("status = %d, esperado 200: alerta falho não derruba a prontidão", code)
	}
	if corpo["alertas_falhos"] != float64(1) {
		t.Errorf("alertas_falhos = %v, esperado 1", corpo["alertas_falhos"])
	}
	motivos, ok := corpo["degradado"].([]any)
	if !ok || len(motivos) == 0 {
		t.Fatalf("degradado = %v, esperado a lista de motivos", corpo["degradado"])
	}
	if !strings.Contains(motivos[0].(string), "entrega falhou") {
		t.Errorf("motivo = %v, esperado citar a entrega falhou", motivos[0])
	}
}

func TestReadyzTrazLogsDescartadosComoNumero(t *testing.T) {
	setupAuditAPI(t)

	_, corpo := lerReadyz(t)
	if _, ok := corpo["logs_descartados"].(float64); !ok {
		t.Errorf("logs_descartados = %v (%T), esperado número", corpo["logs_descartados"], corpo["logs_descartados"])
	}
}

func TestMetricasExpoemContadores(t *testing.T) {
	setupAuditAPI(t)
	antes := observabilidade.AlertasEnfileirados.Value()
	observabilidade.AlertasEnfileirados.Add(1)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+testConfig().Token)
	Routes(testConfig()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics: status %d (%s)", rec.Code, rec.Body.String())
	}
	corpo := rec.Body.String()
	for _, contador := range []string{
		"dockkeeper_alertas_enfileirados", "dockkeeper_alertas_entregues", "dockkeeper_alertas_falhos",
		"dockkeeper_logs_descartados", "dockkeeper_sessoes_ssh_abertas", "dockkeeper_reconexoes_ssh",
		"dockkeeper_panicos_recuperados", "dockkeeper_migracoes_aplicadas",
	} {
		if !strings.Contains(corpo, contador) {
			t.Errorf("/metrics não expõe %s", contador)
		}
	}
	if !strings.Contains(corpo, "dockkeeper_alertas_enfileirados "+strconv.FormatInt(antes+1, 10)) {
		t.Errorf("o contador de alertas enfileirados não subiu: %s", corpo)
	}
}
