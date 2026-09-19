package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/audit"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func conviteDoTipo(t *testing.T, siteID uint, kind string) string {
	t.Helper()

	valor, err := newSecret()
	if err != nil {
		t.Fatalf("gerar convite: %v", err)
	}
	token := database.EnrollmentToken{
		TokenHash: hashSecret(valor),
		SiteID:    siteID,
		Kind:      kind,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := database.DB.Create(&token).Error; err != nil {
		t.Fatalf("gravar convite: %v", err)
	}
	return valor
}

func enrollCom(t *testing.T, corpo string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/enroll", strings.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	return rec
}

type credencialDeTeste struct {
	DeviceID string `json:"device_id"`
	Token    string `json:"device_token"`
}

func credencialDoTipo(t *testing.T, siteID uint, kind, machineID string) credencialDeTeste {
	t.Helper()

	convite := conviteDoTipo(t, siteID, kind)
	rec := enrollCom(t, `{"enrollment_token":"`+convite+`","machine_id":"`+machineID+`","hostname":"estacao-r2"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll de %s: status %d (%s)", kind, rec.Code, rec.Body.String())
	}
	var cred credencialDeTeste
	if err := json.NewDecoder(rec.Body).Decode(&cred); err != nil {
		t.Fatalf("resposta do enroll: %v", err)
	}
	return cred
}

func enviarComCredencial(t *testing.T, rota, corpo string, cred credencialDeTeste) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, rota, strings.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDeviceID, cred.DeviceID)
	req.Header.Set(headerDeviceToken, cred.Token)
	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	return rec
}

func corpoDeInventario() string {
	return `{"site_code":"` + codigoFilialA + `","collector_version":"r2","hosts":[{"ip":"192.168.77.10","hostname":"impressora-r2"}]}`
}

func corpoDeMetrica() string {
	return `{"hostname":"estacao-r2-metrica","site_code":"` + codigoFilialA + `","cpu":10,"mem_total":100,"mem_used":10}`
}

func unicaRecusa(t *testing.T, action, deviceID string) {
	t.Helper()

	linhas := linhasDe(t, action)
	if len(linhas) != 1 {
		t.Fatalf("linhas de auditoria %q = %d, esperada 1", action, len(linhas))
	}
	if linhas[0].Result != audit.ResultDenied {
		t.Errorf("%s: resultado %q, esperado %q", action, linhas[0].Result, audit.ResultDenied)
	}
	if linhas[0].TargetID != deviceID {
		t.Errorf("%s: alvo %q, esperado o dispositivo %q", action, linhas[0].TargetID, deviceID)
	}
}

func TestCredencialDeAgenteNaoEnviaInventario(t *testing.T) {
	sedeA, _ := setupEnrollDB(t)
	setupAuditAPI(t)
	cred := credencialDoTipo(t, sedeA, kindAgent, "maquina-r2-agente")

	rec := enviarComCredencial(t, "/api/ingest/inventory", corpoDeInventario(), cred)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("credencial agent no inventário: status %d, esperado 403 (%s)", rec.Code, rec.Body.String())
	}
	unicaRecusa(t, "inventory.kind_mismatch", cred.DeviceID)
	if n := len(linhasDe(t, "ingest.inventory")); n != 0 {
		t.Errorf("a recusa gerou %d linha(s) extras do middleware (ingest.inventory); esperada só a do handler", n)
	}

	var n int64
	database.DB.Model(&database.NetworkHost{}).Where("ip = ?", "192.168.77.10").Count(&n)
	if n != 0 {
		t.Errorf("o envio recusado gravou %d host(s) no inventário", n)
	}
}

func TestCredencialDeColetorNaoEnviaMetrica(t *testing.T) {
	sedeA, _ := setupEnrollDB(t)
	setupAuditAPI(t)
	cred := credencialDoTipo(t, sedeA, kindCollector, "maquina-r2-coletor")

	rec := enviarComCredencial(t, "/api/ingest/metrics", corpoDeMetrica(), cred)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("credencial collector nas métricas: status %d, esperado 403 (%s)", rec.Code, rec.Body.String())
	}
	unicaRecusa(t, "ingest.kind_mismatch", cred.DeviceID)
	if n := len(linhasDe(t, "ingest.metrics")); n != 0 {
		t.Errorf("a recusa gerou %d linha(s) extras do middleware (ingest.metrics); esperada só a do handler", n)
	}

	var n int64
	database.DB.Unscoped().Model(&database.Server{}).Where("name = ?", "estacao-r2-metrica").Count(&n)
	if n != 0 {
		t.Errorf("o envio recusado criou %d servidor(es)", n)
	}
}

func TestCredencialDoTipoCertoContinuaAceita(t *testing.T) {
	sedeA, _ := setupEnrollDB(t)
	t.Cleanup(func() {
		database.DB.Where("ip = ?", "192.168.77.10").Delete(&database.NetworkHost{})
	})
	agente := credencialDoTipo(t, sedeA, kindAgent, "maquina-r2-ok-agente")
	coletor := credencialDoTipo(t, sedeA, kindCollector, "maquina-r2-ok-coletor")

	if rec := enviarComCredencial(t, "/api/ingest/metrics", corpoDeMetrica(), agente); rec.Code != http.StatusOK {
		t.Errorf("agent nas métricas: status %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := enviarComCredencial(t, "/api/ingest/inventory", corpoDeInventario(), coletor); rec.Code != http.StatusOK {
		t.Errorf("collector no inventário: status %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestTokenLegadoContinuaAceitoNoInventario(t *testing.T) {
	setupEnrollDB(t)
	t.Cleanup(func() {
		database.DB.Where("ip = ?", "192.168.77.10").Delete(&database.NetworkHost{})
	})
	t.Setenv("AGENT_INGEST_TOKEN", "token-legado-r2")
	t.Setenv("ALLOW_LEGACY_INGEST_TOKEN", "true")

	req := httptest.NewRequest(http.MethodPost, "/api/ingest/inventory", strings.NewReader(corpoDeInventario()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerLegacyToken, "token-legado-r2")
	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("token legado no inventário: status %d, esperado 200 (%s)", rec.Code, rec.Body.String())
	}
}

func TestEnrollComKindDivergenteRecebe409SemConsumirConvite(t *testing.T) {
	sedeA, _ := setupEnrollDB(t)
	convite := conviteDoTipo(t, sedeA, kindAgent)

	rec := enrollCom(t, `{"enrollment_token":"`+convite+`","machine_id":"maquina-r2-enroll","kind":"collector"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("enroll com kind divergente: status %d, esperado 409 (%s)", rec.Code, rec.Body.String())
	}

	rec = enrollCom(t, `{"enrollment_token":"`+convite+`","machine_id":"maquina-r2-enroll","kind":"agent"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("segundo enroll com o kind certo: status %d, esperado 201 — o convite foi consumido pela recusa (%s)",
			rec.Code, rec.Body.String())
	}
}

func TestEnrollSemKindNoCorpoSegueAceito(t *testing.T) {
	sedeA, _ := setupEnrollDB(t)
	convite := conviteDoTipo(t, sedeA, kindCollector)

	rec := enrollCom(t, `{"enrollment_token":"`+convite+`","machine_id":"maquina-r2-semkind"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll sem kind: status %d, esperado 201 (%s)", rec.Code, rec.Body.String())
	}
}
