package rules

import (
	"strings"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	srvAusente    = "00000000-0000-0000-0000-0000000000a5"
	prefixoColeta = "m5ausencia"
)

func setupAusencia(t *testing.T) uint {
	t.Helper()
	setupMotorDB(t)

	limpar := func() {
		database.DB.Where("key LIKE ? OR key LIKE ?", "agent_absent:%", "collector_absent:%").Delete(&database.Alert{})
		database.DB.Where("device_id LIKE ?", prefixoColeta+"%").Delete(&database.DeviceCredential{})
		database.DB.Where("server_id = ?", srvAusente).Delete(&database.MetricServer{})
		database.DB.Unscoped().Where("id = ?", srvAusente).Delete(&database.Server{})
		database.DB.Where("code = ?", prefixoColeta).Delete(&database.Site{})
	}
	limpar()
	t.Cleanup(limpar)

	unidade := database.Site{Name: "Filial da ausência", Code: prefixoColeta}
	if err := database.DB.Create(&unidade).Error; err != nil {
		t.Fatalf("criar unidade: %v", err)
	}
	return unidade.ID
}

func estacao(t *testing.T, unidade uint, machineID string, idadeDaAmostra time.Duration) {
	t.Helper()
	estacaoCom(t, unidade, machineID, idadeDaAmostra, true)
}

func estacaoCom(t *testing.T, unidade uint, machineID string, idadeDaAmostra time.Duration, vigiada bool) {
	t.Helper()

	srv := database.Server{
		ID: srvAusente, Name: "pc-recepcao", HostIP: "10.95.0.7", Kind: "agent",
		ReportIntervalSec: 30, SiteID: &unidade, MachineID: machineID, AbsenceAlert: vigiada,
	}
	if err := database.DB.Create(&srv).Error; err != nil {
		t.Fatalf("criar estação: %v", err)
	}
	reportar(t, srvAusente, idadeDaAmostra)
}

func reportar(t *testing.T, serverID string, idade time.Duration) {
	t.Helper()

	cpu := 12.0
	m := database.MetricServer{ServerID: serverID, CPUUsagePercent: &cpu, Timestamp: time.Now().UTC().Add(-idade)}
	if err := database.DB.Create(&m).Error; err != nil {
		t.Fatalf("gravar métrica: %v", err)
	}
}

func credencial(t *testing.T, sufixo, kind string, unidade uint, machineID string, visto time.Duration, intervaloSec int, revogada bool) string {
	t.Helper()

	quando := time.Now().UTC().Add(-visto)
	cred := database.DeviceCredential{
		DeviceID: prefixoColeta + sufixo, SecretHash: strings.Repeat("a", 64), SiteID: unidade, Kind: kind,
		MachineID: machineID, Hostname: "coletor-" + sufixo, LastSeenAt: &quando, ReportIntervalSec: intervaloSec,
	}
	if revogada {
		cred.RevokedAt = &quando
	}
	if err := database.DB.Create(&cred).Error; err != nil {
		t.Fatalf("criar credencial: %v", err)
	}
	return cred.DeviceID
}

func alertasDe(t *testing.T, chave string) []database.Alert {
	t.Helper()

	var linhas []database.Alert
	if err := database.DB.Where("key = ?", chave).Order("id asc").Find(&linhas).Error; err != nil {
		t.Fatalf("ler alertas de %q: %v", chave, err)
	}
	return linhas
}

func haUmaHora() time.Time { return time.Now().Add(-time.Hour) }

func TestAgenteCaladoGeraAusenciaComOrigem(t *testing.T) {
	unidade := setupAusencia(t)
	estacao(t, unidade, "", 5*time.Minute)

	vigiarAusencia(haUmaHora(), time.Now())

	linhas := alertasDe(t, "agent_absent:"+srvAusente)
	if len(linhas) != 1 {
		t.Fatalf("%d alerta(s) de ausência para a estação calada há 5 min com intervalo de 30 s, esperado 1", len(linhas))
	}
	a := linhas[0]
	if a.Severity != SeverityHigh || !strings.HasPrefix(a.Text, "[ALERTA]") || !strings.Contains(a.Text, "pc-recepcao") {
		t.Errorf("alerta = %s %q, esperado [ALERTA] com o nome da estação: estação desligada não é crítico", a.Severity, a.Text)
	}
	if a.ServerID == nil || *a.ServerID != srvAusente || a.SiteID == nil || *a.SiteID != unidade {
		t.Errorf("origem = servidor %v unidade %v, esperado a estação e a filial", a.ServerID, a.SiteID)
	}

	vigiarAusencia(haUmaHora(), time.Now())
	if n := len(alertasDe(t, "agent_absent:"+srvAusente)); n != 1 {
		t.Errorf("a segunda passada abriu outro alerta: %d linhas", n)
	}
}

func TestEstacaoSoAlertaAusenciaSeForMarcada(t *testing.T) {
	unidade := setupAusencia(t)
	estacaoCom(t, unidade, "", 5*time.Minute, false)

	vigiarAusencia(haUmaHora(), time.Now())

	if n := len(alertasDe(t, "agent_absent:"+srvAusente)); n != 0 {
		t.Errorf("estação sem absence_alert gerou %d alerta(s): toda estação desligada às 18h viraria alerta", n)
	}
}

func TestAgenteQueVoltaResolveAAusencia(t *testing.T) {
	unidade := setupAusencia(t)
	estacao(t, unidade, "", 5*time.Minute)
	vigiarAusencia(haUmaHora(), time.Now())
	database.DB.Model(&database.Alert{}).Where("key = ?", "agent_absent:"+srvAusente).
		Updates(map[string]any{"delivery": database.AlertDeliveryEnviado, "last_notified_at": time.Now().UTC()})

	reportar(t, srvAusente, 0)
	vigiarAusencia(haUmaHora(), time.Now())

	linhas := alertasDe(t, "agent_absent:"+srvAusente)
	if len(linhas) != 1 || linhas[0].Status != database.AlertStatusResolved {
		t.Fatalf("a estação voltou a reportar e a ausência não se resolveu: %+v", linhas)
	}
	ok := alertasDe(t, "agent_absent:"+srvAusente+":recuperacao")
	if len(ok) != 1 || !strings.Contains(ok[0].Text, "voltou a reportar") {
		t.Errorf("recuperação = %+v, esperado um aviso de que voltou a reportar", ok)
	}
}

func TestAusenciaIgnoraServidorApagadoERevogado(t *testing.T) {
	unidade := setupAusencia(t)
	estacao(t, unidade, "maquina-revogada", 5*time.Minute)
	credencial(t, "-rev", "agent", unidade, "maquina-revogada", 5*time.Minute, 0, true)

	vigiarAusencia(haUmaHora(), time.Now())
	if n := len(alertasDe(t, "agent_absent:"+srvAusente)); n != 0 {
		t.Fatalf("%d alerta(s) para estação de credencial revogada, esperado 0", n)
	}

	database.DB.Where("device_id = ?", prefixoColeta+"-rev").Delete(&database.DeviceCredential{})
	vigiarAusencia(haUmaHora(), time.Now())
	if n := len(alertasDe(t, "agent_absent:"+srvAusente)); n != 1 {
		t.Fatalf("preparação falhou: sem a revogação a estação deveria alertar, %d linha(s)", n)
	}

	database.DB.Where("id = ?", srvAusente).Delete(&database.Server{})
	vigiarAusencia(haUmaHora(), time.Now())
	linhas := alertasDe(t, "agent_absent:"+srvAusente)
	if len(linhas) != 1 || linhas[0].Status != database.AlertStatusResolved {
		t.Errorf("servidor apagado continuou com a ausência aberta: %+v", linhas)
	}

	database.DB.Where("key LIKE ?", "agent_absent:%").Delete(&database.Alert{})
	vigiarAusencia(haUmaHora(), time.Now())
	if n := len(alertasDe(t, "agent_absent:"+srvAusente)); n != 0 {
		t.Errorf("%d alerta(s) de ausência para servidor apagado, esperado 0", n)
	}
}

func TestColetorCaladoGeraAusenciaPeloIntervaloDeInventario(t *testing.T) {
	unidade := setupAusencia(t)
	calado := credencial(t, "-calado", "collector", unidade, "m-calado", 50*time.Minute, 0, false)
	lento := credencial(t, "-lento", "collector", unidade, "m-lento", 50*time.Minute, 3600, false)
	revogado := credencial(t, "-revogado", "collector", unidade, "m-revogado", 50*time.Minute, 0, true)
	credencial(t, "-antiga", "collector", unidade, "m-reinstalado", 50*time.Minute, 0, false)
	credencial(t, "-nova", "collector", unidade, "m-reinstalado", time.Minute, 0, false)

	vigiarAusencia(haUmaHora(), time.Now())

	linhas := alertasDe(t, "collector_absent:"+calado)
	if len(linhas) != 1 {
		t.Fatalf("%d alerta(s) para coletor calado há 50 min com intervalo padrão de 15, esperado 1", len(linhas))
	}
	if linhas[0].SiteID == nil || *linhas[0].SiteID != unidade || !strings.HasPrefix(linhas[0].Text, "[CRITICO]") {
		t.Errorf("alerta = %q unidade %v, esperado [CRITICO] com a filial de origem", linhas[0].Text, linhas[0].SiteID)
	}
	for nome, chave := range map[string]string{
		"intervalo declarado de 1 h": lento, "revogado": revogado, "reinstalado": prefixoColeta + "-antiga",
	} {
		if n := len(alertasDe(t, "collector_absent:"+chave)); n != 0 {
			t.Errorf("coletor %s gerou %d alerta(s), esperado 0", nome, n)
		}
	}
}

func TestAusenciaEsperaUmaJanelaDepoisDoPainelSubir(t *testing.T) {
	unidade := setupAusencia(t)
	estacao(t, unidade, "", 5*time.Minute)

	vigiarAusencia(time.Now().Add(-10*time.Second), time.Now())

	if n := len(alertasDe(t, "agent_absent:"+srvAusente)); n != 0 {
		t.Errorf("painel no ar há 10 s já acusou %d ausência(s): enquanto ele esteve fora ninguém conseguia reportar", n)
	}
}

func TestAusenciaDesligadaNaoSobeAVigia(t *testing.T) {
	t.Setenv("ABSENCE_ALERT", "false")
	if StartAbsenceWatch(time.Hour) {
		t.Error("ABSENCE_ALERT=false e a vigia de ausência subiu")
	}
}

func TestRegraAbaixoDoMinimoNaoLogaACadaTick(t *testing.T) {
	setupMotorDB(t)
	regra := database.AlertRule{
		Name: prefixoRegra + "abaixo", Target: srvDuracao, Metric: "cpu",
		Operator: ">", Threshold: 50, Enabled: true, Severity: SeverityInfo,
	}
	if err := database.DB.Create(&regra).Error; err != nil {
		t.Fatalf("criar regra: %v", err)
	}
	amostra(t, 90, 0)

	buf := capturarLog(t)
	for range 3 {
		evaluate()
	}
	if n := avisos(buf, "abaixo", "abaixo do mínimo"); n != 1 {
		t.Errorf("regra abaixo do mínimo logou %d vez(es) em 3 ticks, esperado 1", n)
	}
}

func TestMotorAvaliaAgenteDeCincoMinutos(t *testing.T) {
	setupMotorDB(t)
	database.DB.Model(&database.Server{}).Where("id = ?", srvDuracao).Update("report_interval_sec", 300)
	regra := criarRegra(t, "agente-lento", 0)
	amostra(t, 90, 12*time.Minute)

	evaluate()

	if !estadoDe(t, regra).Active {
		t.Error("amostra de 12 min de um agente que reporta a cada 5 min ficou fora do motor: a janela dele é de 15 min")
	}
}
