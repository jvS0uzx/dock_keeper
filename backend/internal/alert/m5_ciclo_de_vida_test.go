package alert

import (
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func envelhecer(t *testing.T, chave string, idade time.Duration) {
	t.Helper()

	quando := time.Now().UTC().Add(-idade)
	err := database.DB.Model(&database.Alert{}).Where("key = ?", chave).Updates(map[string]any{
		"created_at": quando, "last_attempt_at": quando, "last_seen_at": quando,
	}).Error
	if err != nil {
		t.Fatalf("envelhecer %q: %v", chave, err)
	}
}

func TestSemCanalAntigoNaoCalaAChave(t *testing.T) {
	semCanal(t)
	t.Setenv("ALERT_RESUME_HOURS", "24")
	chave := prefixoFila + "r1-sem-canal-antigo"

	Enqueue(Entrada{Key: chave, Text: "[CRITICO] VPS inalcançável na terça"})
	dispatchPending(time.Now().UTC())
	if preso := alertaDaChave(t, chave); preso.Delivery != database.AlertDeliverySemCanal {
		t.Fatalf("preparação falhou: delivery = %q", preso.Delivery)
	}
	envelhecer(t, chave, 72*time.Hour)

	if !Enqueue(Entrada{Key: chave, Text: "[CRITICO] VPS inalcançável na sexta"}) {
		t.Fatal("alerta sem_canal de 3 dias calou a chave: o incidente de hoje foi recusado")
	}
	novo := alertaDaChave(t, chave)
	if novo.Text != "[CRITICO] VPS inalcançável na sexta" || novo.Delivery != database.AlertDeliveryPendente {
		t.Errorf("último alerta da chave = %q/%s, esperado o de sexta como pendente", novo.Text, novo.Delivery)
	}
}

func TestResolvidoAMaoNaoCalaAChave(t *testing.T) {
	semCanal(t)
	chave := prefixoFila + "r1-resolvido-a-mao"

	Enqueue(Entrada{Key: chave, Text: "[CRITICO] primeiro incidente"})
	dispatchPending(time.Now().UTC())
	database.DB.Model(&database.Alert{}).Where("key = ?", chave).
		Updates(map[string]any{"status": database.AlertStatusResolved, "resolved_at": time.Now().UTC()})

	if !Enqueue(Entrada{Key: chave, Text: "[CRITICO] segundo incidente"}) {
		t.Fatal("alerta sem_canal resolvido à mão continuou calando a chave")
	}
	linhas := alertasDaChave(t, chave)
	if len(linhas) != 2 || linhas[1].Status != database.AlertStatusOpen {
		t.Fatalf("alertas da chave = %d, esperado 2 com o segundo aberto", len(linhas))
	}
}

func TestIncidenteLongoViraUmAlertaSo(t *testing.T) {
	falso := setupFila(t)
	cooldown = 30 * time.Minute
	chave := prefixoFila + "um-incidente"

	base := time.Now().UTC().Add(-8 * time.Hour)
	var ultimo time.Time
	for i := range 16 {
		ultimo = base.Add(time.Duration(i) * 31 * time.Minute)
		agora = func() time.Time { return ultimo }
		Notify(chave, "[ALERTA] cpu acima do limite")
		dispatchPending(ultimo)
	}

	linhas := alertasDaChave(t, chave)
	if len(linhas) != 1 {
		t.Fatalf("8 horas de cpu alta viraram %d alertas, esperado 1", len(linhas))
	}
	if linhas[0].RenotifyCount != 15 {
		t.Errorf("renotify_count = %d, esperado 15", linhas[0].RenotifyCount)
	}
	if linhas[0].LastNotifiedAt == nil || linhas[0].LastNotifiedAt.Sub(ultimo).Abs() > time.Second {
		t.Errorf("last_notified_at = %v, esperado %v", linhas[0].LastNotifiedAt, ultimo)
	}
	if linhas[0].Status != database.AlertStatusOpen || linhas[0].Delivery != database.AlertDeliveryEnviado {
		t.Errorf("alerta ficou %s/%s, esperado open/enviado", linhas[0].Status, linhas[0].Delivery)
	}
	if n := falso.enviosCom("[ALERTA] cpu acima do limite"); n != 16 {
		t.Errorf("o Telegram recebeu %d avisos, esperado 16: a renotificação continua saindo", n)
	}
}

func TestAlertaReconhecidoNaoRenotifica(t *testing.T) {
	falso := setupFila(t)
	cooldown = 30 * time.Minute
	chave := prefixoFila + "reconhecido"

	base := time.Now().UTC().Add(-2 * time.Hour)
	agora = func() time.Time { return base }
	Notify(chave, "[ALERTA] disco enchendo")
	dispatchPending(base)
	database.DB.Model(&database.Alert{}).Where("key = ?", chave).Update("status", database.AlertStatusAcked)

	depois := base.Add(time.Hour)
	agora = func() time.Time { return depois }
	if Notify(chave, "[ALERTA] disco enchendo") {
		t.Error("alerta reconhecido foi renotificado")
	}
	dispatchPending(depois)

	if n := len(alertasDaChave(t, chave)); n != 1 {
		t.Errorf("%d alertas para a chave, esperado 1: reconhecer não abre incidente novo", n)
	}
	if n := falso.enviosCom("[ALERTA] disco enchendo"); n != 1 {
		t.Errorf("%d avisos, esperado 1", n)
	}
}

func TestRecuperacaoNasceResolvidaESaiPeloTelegram(t *testing.T) {
	falso := setupFila(t)
	chave := prefixoFila + "ok-resolvido"
	servidor, unidade := "11111111-2222-3333-4444-555555555555", uint(7)

	base := time.Now().UTC()
	agora = func() time.Time { return base }
	Enqueue(Entrada{Key: chave, Text: "[CRITICO] cpu acima do limite", ServerID: &servidor, SiteID: &unidade})
	dispatchPending(base)

	Recovered(Entrada{Key: chave, Text: "[INFO] Recuperado - cpu voltou"})

	ok := alertasDaChave(t, chave+":recuperacao")
	if len(ok) != 1 {
		t.Fatalf("%d mensagens de recuperação, esperado 1", len(ok))
	}
	if ok[0].Status != database.AlertStatusResolved || ok[0].ResolvedAt == nil {
		t.Errorf("o [OK] nasceu %q (resolved_at=%v), esperado resolved", ok[0].Status, ok[0].ResolvedAt)
	}
	if ok[0].ServerID == nil || *ok[0].ServerID != servidor || ok[0].SiteID == nil || *ok[0].SiteID != unidade {
		t.Errorf("o [OK] perdeu a origem: servidor=%v unidade=%v", ok[0].ServerID, ok[0].SiteID)
	}

	dispatchPending(base.Add(time.Second))
	if n := falso.enviosCom("[INFO] Recuperado - cpu voltou"); n != 1 {
		t.Errorf("o [OK] saiu %d vez(es) pelo Telegram, esperado 1", n)
	}
}

func TestRecuperacaoDeAlertaNuncaAnunciadoNaoMandaOK(t *testing.T) {
	falso := setupFila(t)
	chave := prefixoFila + "ok-sem-anuncio"

	base := time.Now().UTC()
	agora = func() time.Time { return base }
	Notify(chave, "[CRITICO] caiu e voltou em dois segundos")
	Recovered(Entrada{Key: chave, Text: "[INFO] Recuperado"})
	dispatchPending(base.Add(time.Second))

	if n := len(alertasDaChave(t, chave+":recuperacao")); n != 0 {
		t.Errorf("%d [OK] para um problema que nunca foi anunciado, esperado 0", n)
	}
	if n := falso.enviosCom("[CRITICO] caiu e voltou em dois segundos"); n != 0 {
		t.Errorf("alerta já resolvido saiu %d vez(es) pelo Telegram", n)
	}
	if a := alertaDaChave(t, chave); a.Delivery != database.AlertDeliveryDispensado {
		t.Errorf("delivery = %q, esperado dispensado", a.Delivery)
	}
}

func TestResolvidoAMaoAntesDaEntregaNaoSai(t *testing.T) {
	falso := setupFila(t)
	chave := prefixoFila + "resolvido-antes"

	base := time.Now().UTC()
	agora = func() time.Time { return base }
	Notify(chave, "[ALERTA] operador resolveu antes do despacho")
	database.DB.Model(&database.Alert{}).Where("key = ?", chave).
		Updates(map[string]any{"status": database.AlertStatusResolved, "resolved_at": base})

	dispatchPending(base.Add(time.Second))

	if n := falso.enviosCom("[ALERTA] operador resolveu antes do despacho"); n != 0 {
		t.Errorf("alerta resolvido saiu %d vez(es) pelo Telegram, esperado 0", n)
	}
	if a := alertaDaChave(t, chave); a.Delivery != database.AlertDeliveryDispensado {
		t.Errorf("delivery = %q, esperado dispensado: pendente para sempre é mentira", a.Delivery)
	}
}

func TestFalhouRecenteERetomadoQuandoOCanalVolta(t *testing.T) {
	falso := setupFila(t)
	falso.falhar.Store(true)
	t.Setenv("ALERT_MAX_ATTEMPTS", "1")
	t.Setenv("ALERT_RESUME_HOURS", "24")
	recente, antigo := prefixoFila+"falhou-recente", prefixoFila+"falhou-antigo"

	base := time.Now().UTC().Add(-time.Hour)
	agora = func() time.Time { return base }
	Notify(recente, "[CRITICO] falhou há uma hora")
	Notify(antigo, "[CRITICO] falhou na semana passada")
	dispatchPending(base)
	if a := alertaDaChave(t, recente); a.Delivery != database.AlertDeliveryFalhou {
		t.Fatalf("preparação falhou: delivery = %q", a.Delivery)
	}
	envelhecer(t, antigo, 72*time.Hour)

	falso.falhar.Store(false)
	depois := base.Add(30 * time.Minute)
	agora = func() time.Time { return depois }
	marcarOK()
	dispatchPending(depois)

	if a := alertaDaChave(t, recente); a.Delivery != database.AlertDeliveryEnviado {
		t.Errorf("falhou de uma hora ficou em %q com o canal de volta, esperado enviado", a.Delivery)
	}
	if a := alertaDaChave(t, antigo); a.Delivery != database.AlertDeliveryFalhou {
		t.Errorf("falhou de 72 h foi retomado com janela de 24 h: delivery = %q", a.Delivery)
	}
	if n := falso.enviosCom("[CRITICO] falhou há uma hora"); n != 2 {
		t.Errorf("envios = %d, esperado 2 (a falha e a retomada)", n)
	}
}

func TestFalhouNaoMartelaOCanalQueContinuaRecusando(t *testing.T) {
	falso := setupFila(t)
	falso.falhar.Store(true)
	t.Setenv("ALERT_MAX_ATTEMPTS", "1")
	chave := prefixoFila + "falhou-sem-martelar"

	base := time.Now().UTC().Add(-time.Hour)
	agora = func() time.Time { return base }
	Notify(chave, "[CRITICO] recusado sempre")
	dispatchPending(base)

	for i := 1; i <= 5; i++ {
		quando := base.Add(time.Duration(i) * 5 * time.Second)
		agora = func() time.Time { return quando }
		dispatchPending(quando)
	}
	if n := falso.enviosCom("[CRITICO] recusado sempre"); n != 1 {
		t.Errorf("envios = %d, esperado 1: sem sinal de canal de volta não há nova tentativa", n)
	}
}

func TestNovoIncidenteNoCooldownApareceNoPainelEEsperaParaAvisar(t *testing.T) {
	falso := setupFila(t)
	cooldown = 30 * time.Minute
	chave := prefixoFila + "flap"

	base := time.Now().UTC().Add(-time.Hour)
	agora = func() time.Time { return base }
	Notify(chave, "[CRITICO] caiu")
	dispatchPending(base)
	Recovered(Entrada{Key: chave, Text: "[INFO] Recuperado"})

	cinco := base.Add(5 * time.Minute)
	agora = func() time.Time { return cinco }
	if !Notify(chave, "[CRITICO] caiu de novo") {
		t.Fatal("incidente novo dentro do cooldown não foi registrado: o painel fica sem o alerta aberto")
	}
	dispatchPending(cinco)
	if n := falso.enviosCom("[CRITICO] caiu de novo"); n != 0 {
		t.Errorf("o cooldown não segurou o aviso: %d envio(s)", n)
	}
	a := alertaDaChave(t, chave)
	if a.Status != database.AlertStatusOpen || a.NextAttemptAt == nil || a.NextAttemptAt.Before(base.Add(29*time.Minute)) {
		t.Errorf("alerta %s com próxima tentativa %v, esperado open e aviso só depois do cooldown", a.Status, a.NextAttemptAt)
	}

	passou := base.Add(31 * time.Minute)
	agora = func() time.Time { return passou }
	dispatchPending(passou)
	if n := falso.enviosCom("[CRITICO] caiu de novo"); n != 1 {
		t.Errorf("cooldown vencido e o incidente aberto não foi avisado: %d envio(s)", n)
	}
}

func TestAbaixoDoMinimoNaoEnfileira(t *testing.T) {
	setupFila(t)
	t.Setenv("ALERT_MIN_SEVERITY", "critical")
	chave := prefixoFila + "abaixo-do-minimo"

	if Enqueue(Entrada{Key: chave, Text: "[ALERTA] container parado", Severity: "high"}) {
		t.Error("gatilho high passou com ALERT_MIN_SEVERITY=critical")
	}
	if n := len(alertasDaChave(t, chave)); n != 0 {
		t.Errorf("%d alerta(s) gravados abaixo do mínimo, esperado 0", n)
	}
	if !Enqueue(Entrada{Key: chave, Text: "[CRITICO] host fora", Severity: "critical"}) {
		t.Error("alerta critical recusado com ALERT_MIN_SEVERITY=critical")
	}
}

func TestChavesAbertasDevolveSoOQueEstaAberto(t *testing.T) {
	setupFila(t)
	aberta, fechada := prefixoFila+"abertas:a", prefixoFila+"abertas:b"

	Notify(aberta, "[ALERTA] aberto")
	Notify(fechada, "[ALERTA] fechado")
	Recovered(Entrada{Key: fechada})

	chaves := ChavesAbertas(prefixoFila + "abertas:")
	if len(chaves) != 1 || chaves[0] != aberta {
		t.Errorf("chaves abertas = %v, esperado só %q", chaves, aberta)
	}
}
