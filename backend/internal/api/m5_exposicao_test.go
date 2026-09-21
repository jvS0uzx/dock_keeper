package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/alert"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"gorm.io/gorm"
)

func readyz(t *testing.T, rota string, comCredencial bool) (int, map[string]any) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, rota, nil)
	if comCredencial {
		req.Header.Set("Authorization", "Bearer "+testConfig().Token)
	}
	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	var corpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("corpo de %s: %v", rota, err)
	}
	return rec.Code, corpo
}

func TestReadyzSemCredencialNaoPublicaODetalheDoCanal(t *testing.T) {
	setupAuditAPI(t)
	t.Cleanup(alert.Init)
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("TELEGRAM_CHAT_ID", "-1001234567890")
	alert.Init()

	for _, rota := range []string{"/readyz", "/api/readyz"} {
		statusAnonimo, anonimo := readyz(t, rota, false)
		if _, vazou := anonimo["alertas_detalhe"]; vazou {
			t.Errorf("%s sem credencial publicou alertas_detalhe: %v", rota, anonimo["alertas_detalhe"])
		}
		if _, vazou := anonimo["degradado"]; vazou {
			t.Errorf("%s sem credencial publicou a lista de motivos: %v", rota, anonimo["degradado"])
		}
		for _, campo := range []string{"status", "alertas", "alertas_falhos", "alertas_sem_canal", "logs_descartados"} {
			if _, ok := anonimo[campo]; !ok {
				t.Errorf("%s sem credencial perdeu o campo %s", rota, campo)
			}
		}

		statusComCredencial, completo := readyz(t, rota, true)
		if completo["alertas_detalhe"] == nil {
			t.Errorf("%s com credencial não trouxe alertas_detalhe", rota)
		}
		if statusAnonimo != statusComCredencial {
			t.Errorf("%s: status %d sem credencial e %d com, esperado o mesmo", rota, statusAnonimo, statusComCredencial)
		}
	}
}

func pedidoDe(remoto string, cabecalhos map[string]string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = remoto
	for k, v := range cabecalhos {
		req.Header.Set(k, v)
	}
	return req
}

func TestCabecalhoDeProxySoValeVindoDeProxyConfiavel(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "")

	deFora := pedidoDe("203.0.113.9:4444", map[string]string{"X-Real-IP": "1.2.3.4", "X-Forwarded-For": "1.2.3.4"})
	if got := clientIP(deFora, true); got != "203.0.113.9" {
		t.Errorf("conexão de fora da lista de proxies: clientIP = %q, esperado 203.0.113.9; o cabeçalho foi aceito cru", got)
	}

	cadeia := pedidoDe("10.0.0.7:4444", map[string]string{"X-Forwarded-For": "1.2.3.4, 198.51.100.4, 10.0.0.9"})
	if got := clientIP(cadeia, true); got != "198.51.100.4" {
		t.Errorf("clientIP = %q, esperado 198.51.100.4: o salto mais à direita que não é proxy confiável", got)
	}

	soReal := pedidoDe("127.0.0.1:4444", map[string]string{"X-Real-IP": "198.51.100.8"})
	if got := clientIP(soReal, true); got != "198.51.100.8" {
		t.Errorf("clientIP = %q, esperado o X-Real-IP entregue pelo proxy local", got)
	}

	lixo := pedidoDe("127.0.0.1:4444", map[string]string{"X-Real-IP": "<script>"})
	if got := clientIP(lixo, true); got != "127.0.0.1" {
		t.Errorf("clientIP = %q, esperado 127.0.0.1: cabeçalho que não é IP não vale", got)
	}
}

func TestListaDeProxiesConfiaveisEConfiguravel(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.7/32")

	estacao := pedidoDe("10.0.0.7:4444", map[string]string{"X-Forwarded-For": "9.9.9.9, 192.168.1.50"})
	if got := clientIP(estacao, true); got != "192.168.1.50" {
		t.Errorf("clientIP = %q, esperado 192.168.1.50: com a lista restrita ao proxy, a estação da LAN é o cliente", got)
	}
	outro := pedidoDe("10.0.0.8:4444", map[string]string{"X-Forwarded-For": "9.9.9.9"})
	if got := clientIP(outro, true); got != "10.0.0.8" {
		t.Errorf("clientIP = %q, esperado 10.0.0.8: faixa privada fora da lista não é proxy", got)
	}
}

func TestIngestaoGravaOIPDaEstacaoENaoODoProxy(t *testing.T) {
	setupAuditAPI(t)
	t.Setenv("AGENT_INGEST_TOKEN", "token-m5-m8")
	t.Setenv("ALLOW_LEGACY_INGEST_TOKEN", "true")
	t.Setenv("TRUST_PROXY_HEADERS", "true")
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.5/32")
	zerarLimiteDeIngestao()
	limparServidorCadastrado(t, "m5-estacao-atras-do-nginx")

	req := httptest.NewRequest(http.MethodPost, "/api/ingest/metrics",
		strings.NewReader(`{"hostname":"m5-estacao-atras-do-nginx","machine_id":"m5-m8","cpu":1}`))
	req.RemoteAddr = "172.18.0.5:51000"
	req.Header.Set("X-Forwarded-For", "192.168.40.21")
	req.Header.Set(headerLegacyToken, "token-m5-m8")
	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ingestão: status %d (%s)", rec.Code, rec.Body.String())
	}

	if srv := servidorPorNome(t, "m5-estacao-atras-do-nginx"); srv.HostIP != "192.168.40.21" {
		t.Errorf("host_ip = %q, esperado 192.168.40.21: toda estação fica com o IP do nginx", srv.HostIP)
	}
}

func TestRecusaAnonimaDeIngestaoTemTeto(t *testing.T) {
	setupAuditAPI(t)
	t.Setenv("AGENT_INGEST_TOKEN", "token-m5-m10")
	t.Setenv("ALLOW_LEGACY_INGEST_TOKEN", "false")
	t.Setenv("INGEST_RATE_MAX_UNAUTH", "5")
	zerarLimiteDeIngestao()
	limpar := func() {
		database.DB.Where("source_ip = ?", "198.51.100.66").Delete(&database.AuditLog{})
	}
	limpar()
	t.Cleanup(limpar)

	ultimo := 0
	for _, rota := range []string{"/api/ingest/metrics", "/api/ingest/inventory"} {
		for range 20 {
			req := httptest.NewRequest(http.MethodPost, rota, strings.NewReader(`{}`))
			req.RemoteAddr = "198.51.100.66:40000"
			req.Header.Set(headerLegacyToken, "token-m5-m10")
			rec := httptest.NewRecorder()
			Routes(testConfig()).ServeHTTP(rec, req)
			ultimo = rec.Code
		}
	}
	if ultimo != http.StatusTooManyRequests {
		t.Errorf("40ª recusa anônima: status %d, esperado 429", ultimo)
	}

	var linhas int64
	database.DB.Model(&database.AuditLog{}).Where("source_ip = ?", "198.51.100.66").Count(&linhas)
	if linhas > 6 {
		t.Errorf("40 requisições anônimas gravaram %d linhas de auditoria, esperado no máximo 6: as 5 recusas do teto e uma linha do bloqueio", linhas)
	}
}

func zerarLimiteDeIngestao() {
	ingestLimiter.mu.Lock()
	defer ingestLimiter.mu.Unlock()
	clear(ingestLimiter.failures)
}

func TestErroAoLerAProntidaoViraMotivoENaoZero(t *testing.T) {
	setupAuditAPI(t)
	original := database.DB
	database.DB = original.Session(&gorm.Session{NewDB: true}).Table("tabela_que_nao_existe_m5")
	t.Cleanup(func() { database.DB = original })

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	corpo := corpoReadyz(req, true, "ok", "")

	if _, zero := corpo["alertas_falhos"].(int64); zero {
		t.Errorf("alertas_falhos = %v com a leitura falhando: zero aqui é mentira", corpo["alertas_falhos"])
	}
	motivos, _ := corpo["degradado"].([]string)
	achou := false
	for _, m := range motivos {
		if strings.Contains(m, "não foi possível ler") {
			achou = true
		}
	}
	if !achou {
		t.Errorf("degradado = %v, esperado o motivo da leitura que falhou", corpo["degradado"])
	}
}
