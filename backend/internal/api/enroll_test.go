package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joaov/vd_stats/internal/database"
)

const (
	codigoFilialA = "qa-enroll-a"
	codigoFilialB = "qa-enroll-b"
)

func setupEnrollDB(t *testing.T) (uint, uint) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de enrollment")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	limparEnroll(t)
	t.Cleanup(func() { limparEnroll(t) })

	a := database.Site{Name: codigoFilialA, Code: codigoFilialA}
	b := database.Site{Name: codigoFilialB, Code: codigoFilialB}
	if err := database.DB.Create(&[]database.Site{a, b}).Error; err != nil {
		t.Fatalf("criar unidades: %v", err)
	}
	database.DB.Where("code = ?", codigoFilialA).First(&a)
	database.DB.Where("code = ?", codigoFilialB).First(&b)
	return a.ID, b.ID
}

func limparEnroll(t *testing.T) {
	t.Helper()

	var sites []database.Site
	database.DB.Where("code IN ?", []string{codigoFilialA, codigoFilialB}).Find(&sites)
	for _, s := range sites {
		database.DB.Where("site_id = ?", s.ID).Delete(&database.DeviceCredential{})
		database.DB.Where("site_id = ?", s.ID).Delete(&database.EnrollmentToken{})
		database.DB.Unscoped().Where("site_id = ?", s.ID).Delete(&database.Server{})
	}
	database.DB.Where("code IN ?", []string{codigoFilialA, codigoFilialB}).Delete(&database.Site{})
}

// emitirConvite cria um convite direto no banco. Não passa pelo handler de
// emissão de propósito: o que estes testes medem é a troca e o uso da
// credencial, e depender do handler faria a falha dele derrubar todos eles.
func emitirConvite(t *testing.T, siteID uint, validade time.Duration) string {
	t.Helper()

	valor, err := newSecret()
	if err != nil {
		t.Fatalf("gerar convite: %v", err)
	}
	token := database.EnrollmentToken{
		TokenHash: hashSecret(valor),
		SiteID:    siteID,
		Kind:      kindAgent,
		ExpiresAt: time.Now().UTC().Add(validade),
	}
	if err := database.DB.Create(&token).Error; err != nil {
		t.Fatalf("gravar convite: %v", err)
	}
	return valor
}

func chamarEnroll(t *testing.T, convite, machineID string) *httptest.ResponseRecorder {
	t.Helper()

	corpo := `{"enrollment_token":"` + convite + `","machine_id":"` + machineID + `","hostname":"estacao-qa","kind":"agent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/enroll", strings.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	testConfig().enrollHandler(rec, req)
	return rec
}

// O convite é de uso único. Sem a queima na MESMA transação da criação, dois
// instaladores concorrentes trocam o mesmo convite por duas credenciais, e "uso
// único" vira promessa.
func TestConviteSoPodeSerUsadoUmaVez(t *testing.T) {
	sedeA, _ := setupEnrollDB(t)
	convite := emitirConvite(t, sedeA, time.Hour)

	primeira := chamarEnroll(t, convite, "maquina-qa-1")
	if primeira.Code != http.StatusCreated {
		t.Fatalf("primeiro enrollment: status = %d, esperado 201 (%s)", primeira.Code, primeira.Body.String())
	}

	segunda := chamarEnroll(t, convite, "maquina-qa-2")
	if segunda.Code != http.StatusUnauthorized {
		t.Errorf("segundo enrollment: status = %d, esperado 401 — o convite foi reaproveitado", segunda.Code)
	}

	var n int64
	database.DB.Model(&database.DeviceCredential{}).Where("site_id = ?", sedeA).Count(&n)
	if n != 1 {
		t.Errorf("credenciais criadas = %d, esperada 1", n)
	}
}

func TestConviteExpiradoERecusado(t *testing.T) {
	sedeA, _ := setupEnrollDB(t)
	convite := emitirConvite(t, sedeA, -time.Minute)

	rec := chamarEnroll(t, convite, "maquina-qa-3")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, esperado 401 para convite expirado", rec.Code)
	}
}

// A unidade sai do CONVITE, não do corpo do pedido. Se viesse do corpo, quem
// tem um convite de uma filial se cadastraria em outra — e o enrollment não
// resolveria nada.
func TestUnidadeVemDoConviteENaoDoCorpo(t *testing.T) {
	sedeA, sedeB := setupEnrollDB(t)
	convite := emitirConvite(t, sedeB, time.Hour)

	rec := chamarEnroll(t, convite, "maquina-qa-4")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, esperado 201 (%s)", rec.Code, rec.Body.String())
	}

	var resp struct {
		DeviceID string `json:"device_id"`
		SiteID   uint   `json:"site_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
	if resp.SiteID != sedeB {
		t.Errorf("unidade = %d, esperada %d (a do convite), não %d", resp.SiteID, sedeB, sedeA)
	}
}

// O coração do item S7: a credencial da unidade A não pode escrever na unidade
// B. Sob o token compartilhado isso era livre — bastava mudar o site_code do
// corpo — e uma estação comprometida em qualquer filial injetava métrica falsa
// em qualquer outra.
func TestCredencialDeUmaUnidadeNaoEscreveEmOutra(t *testing.T) {
	sedeA, _ := setupEnrollDB(t)
	convite := emitirConvite(t, sedeA, time.Hour)

	rec := chamarEnroll(t, convite, "maquina-qa-5")
	if rec.Code != http.StatusCreated {
		t.Fatalf("enrollment falhou: %s", rec.Body.String())
	}
	var cred struct {
		DeviceID string `json:"device_id"`
		Token    string `json:"device_token"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&cred); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}

	corpo := `{"hostname":"estacao-qa","site_code":"` + codigoFilialB + `","cpu":10,"mem_total":100,"mem_used":10}`
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/metrics", strings.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDeviceID, cred.DeviceID)
	req.Header.Set(headerDeviceToken, cred.Token)

	ingest := httptest.NewRecorder()
	IngestHandler(ingest, req)

	if ingest.Code != http.StatusConflict {
		t.Fatalf("status = %d, esperado 409: a credencial da unidade A gravou na unidade B", ingest.Code)
	}

	// O envio é descartado inteiro: aceitar parte dele é aceitar a parte que o
	// atacante escolheu.
	var n int64
	database.DB.Unscoped().Model(&database.Server{}).Where("name = ?", "estacao-qa").Count(&n)
	if n != 0 {
		t.Errorf("servidores criados = %d, esperado nenhum — o envio recusado gravou mesmo assim", n)
	}
}

// Revogar um dispositivo não pode afetar os demais, e a revogação precisa valer
// no envio seguinte, não só na tela.
func TestCredencialRevogadaERecusada(t *testing.T) {
	sedeA, _ := setupEnrollDB(t)
	convite := emitirConvite(t, sedeA, time.Hour)

	rec := chamarEnroll(t, convite, "maquina-qa-6")
	if rec.Code != http.StatusCreated {
		t.Fatalf("enrollment falhou: %s", rec.Body.String())
	}
	var cred struct {
		DeviceID string `json:"device_id"`
		Token    string `json:"device_token"`
	}
	json.NewDecoder(rec.Body).Decode(&cred)

	agora := time.Now().UTC()
	database.DB.Model(&database.DeviceCredential{}).
		Where("device_id = ?", cred.DeviceID).Update("revoked_at", agora)

	corpo := `{"hostname":"estacao-qa","cpu":10,"mem_total":100,"mem_used":10}`
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/metrics", strings.NewReader(corpo))
	req.Header.Set(headerDeviceID, cred.DeviceID)
	req.Header.Set(headerDeviceToken, cred.Token)

	ingest := httptest.NewRecorder()
	IngestHandler(ingest, req)

	if ingest.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, esperado 401 para credencial revogada", ingest.Code)
	}
}

// O contraponto que impede a correção de virar quebra: o token compartilhado
// continua funcionando durante a transição. Derrubá-lo no dia do deploy
// silenciaria todo agente instalado, e um painel de monitoramento que emudece é
// pior que um inseguro, porque ninguém percebe.
func TestTokenCompartilhadoContinuaAceito(t *testing.T) {
	sedeA, _ := setupEnrollDB(t)
	_ = sedeA

	t.Setenv("AGENT_INGEST_TOKEN", "token-compartilhado-de-teste")

	corpo := `{"hostname":"estacao-qa","site_code":"` + codigoFilialA + `","cpu":10,"mem_total":100,"mem_used":10}`
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/metrics", strings.NewReader(corpo))
	req.Header.Set(headerLegacyToken, "token-compartilhado-de-teste")

	rec := httptest.NewRecorder()
	IngestHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, esperado 200: o token compartilhado parou de funcionar (%s)",
			rec.Code, rec.Body.String())
	}
}
