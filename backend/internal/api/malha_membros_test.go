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

func upstreamAntigo(t *testing.T, endereco string, quandoAtras time.Duration) {
	t.Helper()

	linha := database.MetricLoadBalancer{
		UpstreamAddr:  endereco + ":80",
		Status:        "200",
		RequestsCount: 3,
		Timestamp:     time.Now().UTC().Add(-quandoAtras),
	}
	if err := database.DB.Create(&linha).Error; err != nil {
		t.Fatalf("gravar métrica de upstream: %v", err)
	}
	t.Cleanup(func() {
		database.DB.Where("upstream_addr = ?", linha.UpstreamAddr).Delete(&database.MetricLoadBalancer{})
	})
}

func membroNoLive(t *testing.T, sess auth.Session, serverID string) (bool, string) {
	t.Helper()

	rec := pedirComSessao(t, http.MethodGet, "/api/metrics/live", "", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/metrics/live: status %d", rec.Code)
	}
	var resp struct {
		Servers []struct {
			ID     string `json:"id"`
			Behind bool   `json:"behind_lb"`
			Origem string `json:"behind_lb_origem"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta do live: %v", err)
	}
	for _, s := range resp.Servers {
		if s.ID == serverID {
			return s.Behind, s.Origem
		}
	}
	t.Fatalf("servidor %q ausente do live", serverID)
	return false, ""
}

func TestMembroDaMalhaSobreviveAFimDeSemanaSemTrafego(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-malha-memoria", auth.RoleAdmin)
	s := servidorDeRename(t, "vps-com-memoria", "203.0.113.250", nil)
	upstreamAntigo(t, "203.0.113.250", 3*24*time.Hour)

	behind, origem := membroNoLive(t, sess, s.ID)
	if !behind {
		t.Errorf("servidor com upstream há 3 dias saiu da malha; topologia não é fluxo do minuto")
	}
	if origem != "trafego" {
		t.Errorf("origem = %q, esperado trafego", origem)
	}
}

func TestUpstreamVelhoSaiDaMalha(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-malha-velho", auth.RoleAdmin)
	s := servidorDeRename(t, "vps-esquecida", "203.0.113.251", nil)
	upstreamAntigo(t, "203.0.113.251", 30*24*time.Hour)

	behind, origem := membroNoLive(t, sess, s.ID)
	if behind {
		t.Errorf("upstream de 30 dias ainda conta como membro")
	}
	if origem != "nenhum" {
		t.Errorf("origem = %q, esperado nenhum", origem)
	}
}

func TestMembroPorAliasTambemConta(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-malha-alias", auth.RoleAdmin)
	s := servidorDeRename(t, "vps-overlay-malha", "203.0.113.252", nil)
	database.RegistrarAliases(s.ID, []string{"100.100.0.11"})
	upstreamAntigo(t, "100.100.0.11", 2*time.Hour)

	if behind, _ := membroNoLive(t, sess, s.ID); !behind {
		t.Errorf("upstream que bate com alias do servidor não entrou na malha")
	}
}

func TestManualLigadoVenceAusenciaDeTrafego(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-malha-manual", auth.RoleAdmin)
	s := servidorDeRename(t, "vps-recem-entrou", "203.0.113.253", nil)

	rec := pedirComSessao(t, http.MethodPatch, "/api/servers?id="+s.ID, `{"behind_lb":true}`, sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH behind_lb=true: status %d (%s)", rec.Code, rec.Body.String())
	}

	behind, origem := membroNoLive(t, sess, s.ID)
	if !behind {
		t.Errorf("marcação manual não colocou o servidor na malha")
	}
	if origem != "manual" {
		t.Errorf("origem = %q, esperado manual", origem)
	}
}

func TestManualDesligadoVenceTrafegoRecente(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-malha-tirada", auth.RoleAdmin)
	s := servidorDeRename(t, "vps-saiu-do-lb", "203.0.113.254", nil)
	upstreamAntigo(t, "203.0.113.254", time.Hour)

	rec := pedirComSessao(t, http.MethodPatch, "/api/servers?id="+s.ID, `{"behind_lb":false}`, sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH behind_lb=false: status %d (%s)", rec.Code, rec.Body.String())
	}

	behind, origem := membroNoLive(t, sess, s.ID)
	if behind {
		t.Errorf("tráfego recente venceu a marcação manual de fora")
	}
	if origem != "manual" {
		t.Errorf("origem = %q, esperado manual", origem)
	}
}

func TestNuloVoltaAoAutomatico(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-malha-nulo", auth.RoleAdmin)
	s := servidorDeRename(t, "vps-volta-auto", "203.0.113.255", nil)
	upstreamAntigo(t, "203.0.113.255", time.Hour)

	if rec := pedirComSessao(t, http.MethodPatch, "/api/servers?id="+s.ID,
		`{"behind_lb":false}`, sess); rec.Code != http.StatusOK {
		t.Fatalf("PATCH false: status %d", rec.Code)
	}
	if behind, _ := membroNoLive(t, sess, s.ID); behind {
		t.Fatalf("a marcação manual de fora não pegou")
	}

	if rec := pedirComSessao(t, http.MethodPatch, "/api/servers?id="+s.ID,
		`{"behind_lb":null}`, sess); rec.Code != http.StatusOK {
		t.Fatalf("PATCH null: status %d", rec.Code)
	}

	behind, origem := membroNoLive(t, sess, s.ID)
	if !behind {
		t.Errorf("com behind_lb nulo o servidor deveria voltar a valer pela memória de tráfego")
	}
	if origem != "trafego" {
		t.Errorf("origem = %q, esperado trafego", origem)
	}

	var noBanco database.Server
	database.DB.Where("id = ?", s.ID).Take(&noBanco)
	if noBanco.BehindLB != nil {
		t.Errorf("behind_lb no banco = %v, esperado NULL", *noBanco.BehindLB)
	}
}

func TestReadyzContaAlertaPresoSemCanal(t *testing.T) {
	setupAuditAPI(t)
	const chave = "readyz-sem-canal"
	database.DB.Where("key = ?", chave).Delete(&database.Alert{})
	t.Cleanup(func() { database.DB.Where("key = ?", chave).Delete(&database.Alert{}) })

	preso := database.Alert{
		Key: chave, Severity: "critical", Text: "[CRITICO] preso",
		Status: database.AlertStatusOpen, Delivery: database.AlertDeliverySemCanal,
		CreatedAt: time.Now().UTC(),
	}
	if err := database.DB.Create(&preso).Error; err != nil {
		t.Fatalf("criar alerta preso: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	req.Header.Set("Authorization", "Bearer "+testConfig().Token)
	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)

	var corpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("corpo do /readyz: %v", err)
	}
	if corpo["alertas_sem_canal"] == nil || corpo["alertas_sem_canal"].(float64) < 1 {
		t.Errorf("/readyz não conta os alertas presos: %v", corpo["alertas_sem_canal"])
	}
	motivos, _ := corpo["degradado"].([]any)
	achou := false
	for _, m := range motivos {
		if texto, ok := m.(string); ok && strings.Contains(texto, "sem canal") {
			achou = true
		}
	}
	if !achou {
		t.Errorf("o motivo do alerta preso não entrou em degradado: %v", motivos)
	}
}
