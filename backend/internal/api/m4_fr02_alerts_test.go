package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const prefixoAlertaFR02 = "fr02-api:"

func uintStr(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}

func limparAlertasFR02(t *testing.T) {
	t.Helper()
	database.DB.Where("key LIKE ?", prefixoAlertaFR02+"%").Delete(&database.Alert{})
}

func alertaDeTeste(t *testing.T, sufixo string, siteID *uint, status string) database.Alert {
	t.Helper()

	a := database.Alert{
		Key: prefixoAlertaFR02 + sufixo, Severity: "critical", Text: "[CRITICO] teste " + sufixo,
		Status: status, SiteID: siteID, CreatedAt: time.Now().UTC(),
		Delivery: database.AlertDeliveryPendente,
	}
	if err := database.DB.Create(&a).Error; err != nil {
		t.Fatalf("criar alerta de teste: %v", err)
	}
	return a
}

func listarAlertas(t *testing.T, sess auth.Session, query string) []database.Alert {
	t.Helper()

	rec := chamar(t, alertsHandler, sess, http.MethodGet, "/api/alerts"+query, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("listar alertas: status %d (%s)", rec.Code, rec.Body.String())
	}
	var linhas []database.Alert
	if err := json.Unmarshal(rec.Body.Bytes(), &linhas); err != nil {
		t.Fatalf("listar alertas: %v", err)
	}
	return linhas
}

func TestListaDeAlertasRecortaPorUnidade(t *testing.T) {
	c := setupC3(t)
	limparAlertasFR02(t)
	t.Cleanup(func() { limparAlertasFR02(t) })

	daA := alertaDeTeste(t, "unidade-a", &c.siteA, database.AlertStatusOpen)
	daB := alertaDeTeste(t, "unidade-b", &c.siteB, database.AlertStatusOpen)
	semUnidade := alertaDeTeste(t, "global", nil, database.AlertStatusOpen)

	vistos := map[uint]bool{}
	for _, a := range listarAlertas(t, c.viewerA, "?status=open&limit=500") {
		vistos[a.ID] = true
	}
	if !vistos[daA.ID] {
		t.Error("o viewer da unidade A não vê o alerta da própria unidade")
	}
	if vistos[daB.ID] {
		t.Error("o viewer da unidade A vê alerta da unidade B")
	}
	if vistos[semUnidade.ID] {
		t.Error("alerta sem unidade apareceu para quem não tem acesso global")
	}

	globais := map[uint]bool{}
	for _, a := range listarAlertas(t, c.adminGlobal, "?status=open&limit=500") {
		globais[a.ID] = true
	}
	if !globais[daA.ID] || !globais[daB.ID] || !globais[semUnidade.ID] {
		t.Error("o admin global não vê todos os alertas")
	}
}

func TestResumoDeAlertasContaPorEstado(t *testing.T) {
	c := setupC3(t)
	limparAlertasFR02(t)
	t.Cleanup(func() { limparAlertasFR02(t) })

	alertaDeTeste(t, "resumo-open", &c.siteA, database.AlertStatusOpen)
	alertaDeTeste(t, "resumo-acked", &c.siteA, database.AlertStatusAcked)
	falho := alertaDeTeste(t, "resumo-falhou", &c.siteA, database.AlertStatusOpen)
	database.DB.Model(&database.Alert{}).Where("id = ?", falho.ID).
		Update("delivery", database.AlertDeliveryFalhou)

	rec := chamar(t, alertsSummaryHandler, c.viewerA, http.MethodGet, "/api/alerts/summary", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("resumo: status %d (%s)", rec.Code, rec.Body.String())
	}
	var resumo map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &resumo); err != nil {
		t.Fatalf("resumo: %v", err)
	}
	if resumo["open"] != 2 || resumo["acked"] != 1 || resumo["falhou"] != 1 {
		t.Errorf("resumo = %v, esperado open 2, acked 1 e falhou 1", resumo)
	}
}

func TestAckGravaQuemReconheceu(t *testing.T) {
	c := setupC3(t)
	limparAlertasFR02(t)
	t.Cleanup(func() { limparAlertasFR02(t) })

	a := alertaDeTeste(t, "ack", &c.siteA, database.AlertStatusOpen)

	rec := chamar(t, alertAckHandler, c.viewerA, http.MethodPost, "/api/alerts/ack?id="+uintStr(a.ID), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ack: status %d (%s)", rec.Code, rec.Body.String())
	}

	var gravado database.Alert
	if err := database.DB.Where("id = ?", a.ID).Take(&gravado).Error; err != nil {
		t.Fatalf("reler alerta: %v", err)
	}
	if gravado.Status != database.AlertStatusAcked {
		t.Errorf("status = %q, esperado acked", gravado.Status)
	}
	if gravado.AckedBy == nil || *gravado.AckedBy != c.viewerA.UserID {
		t.Errorf("acked_by = %v, esperado %d", gravado.AckedBy, c.viewerA.UserID)
	}
	if gravado.AckedAt == nil {
		t.Error("acked_at não foi gravado")
	}
}

func TestAckDeAlertaForaDoAlcanceResponde404(t *testing.T) {
	c := setupC3(t)
	limparAlertasFR02(t)
	t.Cleanup(func() { limparAlertasFR02(t) })

	daB := alertaDeTeste(t, "fora-do-alcance", &c.siteB, database.AlertStatusOpen)

	rec := chamar(t, alertAckHandler, c.viewerA, http.MethodPost, "/api/alerts/ack?id="+uintStr(daB.ID), "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("ack fora do alcance: status %d, esperado 404", rec.Code)
	}
}

func TestAckDeAlertaResolvidoResponde409(t *testing.T) {
	c := setupC3(t)
	limparAlertasFR02(t)
	t.Cleanup(func() { limparAlertasFR02(t) })

	a := alertaDeTeste(t, "ja-resolvido", &c.siteA, database.AlertStatusResolved)

	rec := chamar(t, alertAckHandler, c.viewerA, http.MethodPost, "/api/alerts/ack?id="+uintStr(a.ID), "")
	if rec.Code != http.StatusConflict {
		t.Errorf("ack de alerta resolvido: status %d, esperado 409", rec.Code)
	}
}

func TestSessaoDeMaquinaLeMasNaoReconhece(t *testing.T) {
	c := setupC3(t)
	limparAlertasFR02(t)
	t.Cleanup(func() { limparAlertasFR02(t) })

	a := alertaDeTeste(t, "maquina", &c.siteA, database.AlertStatusOpen)
	maquina := auth.Session{Role: auth.RoleAdmin, Accesses: []auth.Access{{Role: auth.RoleAdmin}}}

	if rec := chamar(t, alertsHandler, maquina, http.MethodGet, "/api/alerts?status=open", ""); rec.Code != http.StatusOK {
		t.Errorf("leitura com sessão de máquina: status %d, esperado 200", rec.Code)
	}
	for _, rota := range []struct {
		nome string
		h    http.HandlerFunc
	}{{"ack", alertAckHandler}, {"resolve", alertResolveHandler}} {
		rec := chamar(t, rota.h, maquina, http.MethodPost, "/api/alerts/"+rota.nome+"?id="+uintStr(a.ID), "")
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s com sessão de máquina: status %d, esperado 403", rota.nome, rec.Code)
		}
	}
}

func TestResolveExigeOperador(t *testing.T) {
	c := setupC3(t)
	limparAlertasFR02(t)
	t.Cleanup(func() { limparAlertasFR02(t) })

	a := alertaDeTeste(t, "resolve", &c.siteA, database.AlertStatusOpen)

	if rec := chamar(t, alertResolveHandler, c.viewerA, http.MethodPost, "/api/alerts/resolve?id="+uintStr(a.ID), ""); rec.Code != http.StatusForbidden {
		t.Errorf("resolve por viewer: status %d, esperado 403", rec.Code)
	}

	rec := chamar(t, alertResolveHandler, c.donoA, http.MethodPost, "/api/alerts/resolve?id="+uintStr(a.ID), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("resolve por operador: status %d (%s)", rec.Code, rec.Body.String())
	}
	var gravado database.Alert
	database.DB.Where("id = ?", a.ID).Take(&gravado)
	if gravado.Status != database.AlertStatusResolved || gravado.ResolvedAt == nil {
		t.Errorf("alerta ficou como %q com resolved_at %v, esperado resolved com data", gravado.Status, gravado.ResolvedAt)
	}
}

func TestAckEResolveGeramAuditoria(t *testing.T) {
	setupAuditAPI(t)
	setupC3(t)
	limparAlertasFR02(t)
	t.Cleanup(func() { limparAlertasFR02(t) })

	sess := sessaoReal(t, "operador-fr02", auth.RoleOperator)
	a := alertaDeTeste(t, "auditoria", nil, database.AlertStatusOpen)

	if rec := postComSessao(t, "/api/alerts/ack?id="+uintStr(a.ID), "", sess); rec.Code != http.StatusOK {
		t.Fatalf("ack: status %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := postComSessao(t, "/api/alerts/resolve?id="+uintStr(a.ID), "", sess); rec.Code != http.StatusOK {
		t.Fatalf("resolve: status %d (%s)", rec.Code, rec.Body.String())
	}

	for _, acao := range []string{"alert.ack", "alert.resolve"} {
		var n int64
		database.DB.Model(&database.AuditLog{}).
			Where("action = ? AND target_id = ?", acao, uintStr(a.ID)).Count(&n)
		if n == 0 {
			t.Errorf("nenhuma linha de auditoria %q para o alerta %d", acao, a.ID)
		}
	}
}
