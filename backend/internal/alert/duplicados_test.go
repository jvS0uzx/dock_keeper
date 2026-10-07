package alert

import (
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func abertosDaChave(t *testing.T, chave string) []database.Alert {
	t.Helper()

	var linhas []database.Alert
	if err := database.DB.Where("key = ? AND status <> ?", chave, database.AlertStatusResolved).
		Order("id asc").Find(&linhas).Error; err != nil {
		t.Fatalf("ler alertas abertos de %q: %v", chave, err)
	}
	return linhas
}

func linhaAberta(t *testing.T, chave, texto string, visto time.Time) database.Alert {
	t.Helper()

	a := database.Alert{
		Key: chave, Severity: "high", Text: texto, Status: database.AlertStatusOpen,
		Delivery: database.AlertDeliveryEnviado, CreatedAt: visto, LastSeenAt: &visto, LastNotifiedAt: &visto,
	}
	if err := database.DB.Create(&a).Error; err != nil {
		t.Fatalf("gravar alerta aberto de %q: %v", chave, err)
	}
	return a
}

func TestIncidenteForaDaJanelaNaoDeixaODuploAberto(t *testing.T) {
	setupFila(t)
	t.Setenv("ALERT_RESUME_HOURS", "24")
	chave := prefixoFila + "duplo-fora-da-janela"

	Enqueue(Entrada{Key: chave, Text: "[ALERTA] Container web está exited na terça"})
	dispatchPending(time.Now().UTC())
	envelhecer(t, chave, 72*time.Hour)
	antigo := alertaDaChave(t, chave)

	if !Enqueue(Entrada{Key: chave, Text: "[ALERTA] Container web está exited na sexta"}) {
		t.Fatal("o incidente de sexta não foi enfileirado")
	}

	abertos := abertosDaChave(t, chave)
	if len(abertos) != 1 || abertos[0].Text != "[ALERTA] Container web está exited na sexta" {
		t.Fatalf("%d alerta(s) aberto(s) para a chave, esperado só o de sexta: %+v", len(abertos), abertos)
	}

	var velho database.Alert
	database.DB.First(&velho, antigo.ID)
	if velho.Status != database.AlertStatusResolved || velho.ResolvedAt == nil {
		t.Fatalf("o alerta de terça ficou %s (resolved_at=%v), esperado resolved", velho.Status, velho.ResolvedAt)
	}
	if velho.ResolvedAt.Sub(*antigo.LastSeenAt).Abs() > time.Second {
		t.Errorf("resolved_at = %v, esperado o último momento em que o painel viu a condição (%v)", velho.ResolvedAt, antigo.LastSeenAt)
	}
	if velho.Text != antigo.Text {
		t.Errorf("o texto do alerta de terça mudou para %q", velho.Text)
	}
	if n := len(alertasDaChave(t, chave+sufixoRecuperacao)); n != 0 {
		t.Errorf("%d aviso(s) de recuperação para o alerta substituído; o incidente continua vivo na linha nova", n)
	}
}

func TestRecuperadoFechaTodasAsLinhasAbertasDaChave(t *testing.T) {
	setupFila(t)
	chave := prefixoFila + "duplo-recuperado"
	base := time.Now().UTC().Add(-3 * time.Hour)
	linhaAberta(t, chave, "[ALERTA] Container web está exited", base)
	linhaAberta(t, chave, "[ALERTA] Container web está exited", base.Add(time.Hour))

	Recovered(Entrada{Key: chave, Text: "[INFO] Recuperado - Container web voltou a rodar"})

	if abertos := abertosDaChave(t, chave); len(abertos) != 0 {
		t.Errorf("%d linha(s) da chave continuaram abertas depois da recuperação", len(abertos))
	}
	if n := len(alertasDaChave(t, chave+sufixoRecuperacao)); n != 1 {
		t.Errorf("%d aviso(s) de recuperação, esperado 1 para as duas linhas", n)
	}
}

func TestDuplicadosAntigosFicamSoComOMaisRecente(t *testing.T) {
	setupFila(t)
	chave := prefixoFila + "duplo-legado"
	outra := prefixoFila + "duplo-legado-outra"
	base := time.Now().UTC().Add(-10 * 24 * time.Hour)

	primeiro := linhaAberta(t, chave, "[ALERTA] primeiro", base)
	segundo := linhaAberta(t, chave, "[ALERTA] segundo", base.Add(48*time.Hour))
	database.DB.Model(&database.Alert{}).Where("id = ?", segundo.ID).Update("status", database.AlertStatusAcked)
	recente := linhaAberta(t, chave, "[ALERTA] recente", base.Add(96*time.Hour))
	sozinha := linhaAberta(t, outra, "[ALERTA] sem duplicata", base)

	fecharDuplicados()

	abertos := abertosDaChave(t, chave)
	if len(abertos) != 1 || abertos[0].ID != recente.ID {
		t.Fatalf("abertos da chave = %+v, esperado só o #%d, o mais recente", abertos, recente.ID)
	}
	for _, a := range []database.Alert{primeiro, segundo} {
		var linha database.Alert
		database.DB.First(&linha, a.ID)
		if linha.Status != database.AlertStatusResolved || linha.ResolvedAt == nil || linha.ResolvedAt.Sub(*a.LastSeenAt).Abs() > time.Second {
			t.Errorf("#%d ficou %s com resolved_at=%v, esperado resolved em %v", a.ID, linha.Status, linha.ResolvedAt, a.LastSeenAt)
		}
	}
	if abertos := abertosDaChave(t, outra); len(abertos) != 1 || abertos[0].ID != sozinha.ID {
		t.Errorf("a chave sem duplicata foi mexida: %+v", abertos)
	}
	if n := len(alertasDaChave(t, chave+sufixoRecuperacao)); n != 0 {
		t.Errorf("%d aviso(s) de recuperação ao fechar duplicatas", n)
	}

	fecharDuplicados()
	if abertos := abertosDaChave(t, chave); len(abertos) != 1 || abertos[0].ID != recente.ID {
		t.Errorf("a segunda passada mudou o resultado: %+v", abertos)
	}
}
