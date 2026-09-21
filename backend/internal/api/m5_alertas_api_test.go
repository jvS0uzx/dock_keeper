package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func lerResumo(t *testing.T, c cenarioC3, quem string, query string) (int, map[string]int) {
	t.Helper()

	sess := c.viewerA
	if quem == "admin" {
		sess = c.adminGlobal
	}
	rec := chamar(t, alertsSummaryHandler, sess, http.MethodGet, "/api/alerts/summary"+query, "")
	var resumo map[string]int
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &resumo); err != nil {
			t.Fatalf("resumo: %v", err)
		}
	}
	return rec.Code, resumo
}

func TestVisibilidadeDeAlertasValeAntesDoLimite(t *testing.T) {
	c := setupC3(t)
	limparAlertasFR02(t)
	t.Cleanup(func() { limparAlertasFR02(t) })

	daA := alertaDeTeste(t, "b7-unidade-a", &c.siteA, database.AlertStatusOpen)
	database.DB.Model(&database.Alert{}).Where("id = ?", daA.ID).
		Update("created_at", time.Now().UTC().Add(-time.Hour))

	ruido := make([]database.Alert, 0, 2010)
	for i := range 2010 {
		ruido = append(ruido, database.Alert{
			Key: prefixoAlertaFR02 + "b7-ruido-" + itoa(uint(i)), Severity: "critical", Text: "[CRITICO] ruído da unidade B",
			Status: database.AlertStatusOpen, SiteID: &c.siteB, CreatedAt: time.Now().UTC(),
			Delivery: database.AlertDeliveryPendente,
		})
	}
	if err := database.DB.CreateInBatches(&ruido, 500).Error; err != nil {
		t.Fatalf("criar o ruído da unidade B: %v", err)
	}

	linhas := listarAlertas(t, c.viewerA, "?status=open&limit=100")
	if len(linhas) != 1 || linhas[0].ID != daA.ID {
		t.Errorf("o viewer da unidade A recebeu %d alerta(s); o único dele ficou atrás de 2010 alertas de outra unidade", len(linhas))
	}

	status, resumo := lerResumo(t, c, "viewer", "")
	if status != http.StatusOK || resumo["open"] != 1 {
		t.Errorf("resumo do viewer da unidade A = %d %v, esperado open 1", status, resumo)
	}
}

func TestAlertasFiltramPorUnidade(t *testing.T) {
	c := setupC3(t)
	limparAlertasFR02(t)
	t.Cleanup(func() { limparAlertasFR02(t) })

	daA := alertaDeTeste(t, "filtro-a", &c.siteA, database.AlertStatusOpen)
	daB := alertaDeTeste(t, "filtro-b", &c.siteB, database.AlertStatusOpen)
	semUnidade := alertaDeTeste(t, "filtro-global", nil, database.AlertStatusOpen)

	ids := func(linhas []database.Alert) map[uint]bool {
		m := map[uint]bool{}
		for _, a := range linhas {
			m[a.ID] = true
		}
		return m
	}

	soA := ids(listarAlertas(t, c.adminGlobal, "?status=open&limit=500&site_id="+uintStr(c.siteA)))
	if !soA[daA.ID] || soA[daB.ID] || soA[semUnidade.ID] {
		t.Errorf("admin com site_id=A recebeu A=%v B=%v global=%v, esperado só A", soA[daA.ID], soA[daB.ID], soA[semUnidade.ID])
	}
	soGlobal := ids(listarAlertas(t, c.adminGlobal, "?status=open&limit=500&site_id=none"))
	if soGlobal[daA.ID] || !soGlobal[semUnidade.ID] {
		t.Errorf("admin com site_id=none recebeu A=%v global=%v, esperado só os sem unidade", soGlobal[daA.ID], soGlobal[semUnidade.ID])
	}

	rec := chamar(t, alertsHandler, c.viewerA, http.MethodGet, "/api/alerts?site_id="+uintStr(c.siteB), "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("viewer da unidade A pedindo a unidade B: status %d, esperado 403", rec.Code)
	}
	rec = chamar(t, alertsHandler, c.adminGlobal, http.MethodGet, "/api/alerts?site_id=abc", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("site_id=abc: status %d, esperado 400", rec.Code)
	}

	status, resumo := lerResumo(t, c, "admin", "?site_id="+uintStr(c.siteA))
	if status != http.StatusOK || resumo["open"] != 1 {
		t.Errorf("resumo do admin com site_id=A = %d %v, esperado open 1", status, resumo)
	}
	if status, _ := lerResumo(t, c, "viewer", "?site_id="+uintStr(c.siteB)); status != http.StatusForbidden {
		t.Errorf("resumo do viewer da unidade A pedindo a B: status %d, esperado 403", status)
	}
}

func TestAlertaDevolveNomeDoServidorEDaUnidade(t *testing.T) {
	c := setupC3(t)
	limparAlertasFR02(t)
	t.Cleanup(func() { limparAlertasFR02(t) })

	comOrigem := alertaDeTeste(t, "nomes-origem", &c.siteA, database.AlertStatusOpen)
	servidor := srvC3A
	database.DB.Model(&database.Alert{}).Where("id = ?", comOrigem.ID).Update("server_id", servidor)
	semOrigem := alertaDeTeste(t, "nomes-sem-origem", nil, database.AlertStatusOpen)

	rec := chamar(t, alertsHandler, c.adminGlobal, http.MethodGet, "/api/alerts?status=open&limit=500", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("listar: status %d (%s)", rec.Code, rec.Body.String())
	}
	var linhas []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &linhas); err != nil {
		t.Fatalf("listar: %v", err)
	}

	achou := 0
	for _, l := range linhas {
		switch uint(l["id"].(float64)) {
		case comOrigem.ID:
			achou++
			if l["server_name"] != "host-c3-a" || l["site_name"] != "qa-c3-a" {
				t.Errorf("server_name=%v site_name=%v, esperado host-c3-a e qa-c3-a: a tela mostra UUID", l["server_name"], l["site_name"])
			}
		case semOrigem.ID:
			achou++
			nomeServidor, temServidor := l["server_name"]
			nomeUnidade, temUnidade := l["site_name"]
			if !temServidor || !temUnidade || nomeServidor != nil || nomeUnidade != nil {
				t.Errorf("alerta sem origem: server_name=%v site_name=%v, esperado os dois campos presentes e null", nomeServidor, nomeUnidade)
			}
		}
	}
	if achou != 2 {
		t.Errorf("a listagem devolveu %d dos 2 alertas do teste", achou)
	}
}

func TestInventarioGravaOIntervaloDeclaradoPeloColetor(t *testing.T) {
	sedeA, _ := setupEnrollDB(t)
	t.Cleanup(func() {
		database.DB.Where("ip = ?", "192.168.77.10").Delete(&database.NetworkHost{})
	})
	zerarLimiteDeIngestao()
	coletor := credencialDoTipo(t, sedeA, kindCollector, "maquina-m5-intervalo")

	corpo := `{"site_code":"` + codigoFilialA + `","collector_version":"m5","report_interval_sec":3600,"hosts":[{"ip":"192.168.77.10","hostname":"impressora-m5"}]}`
	if rec := enviarComCredencial(t, "/api/ingest/inventory", corpo, coletor); rec.Code != http.StatusOK {
		t.Fatalf("inventário: status %d (%s)", rec.Code, rec.Body.String())
	}

	var cred database.DeviceCredential
	if err := database.DB.Where("device_id = ?", coletor.DeviceID).Take(&cred).Error; err != nil {
		t.Fatalf("ler a credencial: %v", err)
	}
	if cred.ReportIntervalSec != 3600 {
		t.Errorf("report_interval_sec = %d, esperado 3600: sem ele o alerta de ausência assume 15 min", cred.ReportIntervalSec)
	}
}

func TestViewerNaoReconheceAlerta(t *testing.T) {
	c := setupC3(t)
	limparAlertasFR02(t)
	t.Cleanup(func() { limparAlertasFR02(t) })

	a := alertaDeTeste(t, "m9-viewer", &c.siteA, database.AlertStatusOpen)

	rec := chamar(t, alertAckHandler, c.viewerA, http.MethodPost, "/api/alerts/ack?id="+uintStr(a.ID), "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("viewer reconhecendo alerta: status %d, esperado 403", rec.Code)
	}
	var gravado database.Alert
	database.DB.Where("id = ?", a.ID).Take(&gravado)
	if gravado.Status != database.AlertStatusOpen {
		t.Errorf("status = %q depois do ack de viewer, esperado open", gravado.Status)
	}

	rec = chamar(t, alertAckHandler, c.donoA, http.MethodPost, "/api/alerts/ack?id="+uintStr(a.ID), "")
	if rec.Code != http.StatusOK {
		t.Errorf("operador da unidade reconhecendo: status %d, esperado 200 (%s)", rec.Code, rec.Body.String())
	}
}

func TestCredencialSemTipoNaoPassaEmRotaNenhuma(t *testing.T) {
	semTipo := deviceAuth{DeviceID: "dev-sem-tipo"}
	for _, rota := range []string{kindAgent, kindCollector} {
		if semTipo.allowsKind(rota) {
			t.Errorf("credencial de tipo vazio foi aceita na rota de %s", rota)
		}
	}
	if !(deviceAuth{Legacy: true}).allowsKind(kindCollector) {
		t.Error("token legado deixou de ser aceito")
	}
}
