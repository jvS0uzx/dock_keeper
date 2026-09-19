package alert

import (
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func semCanal(t *testing.T) *telegramFalso {
	t.Helper()

	falso := setupFila(t)
	enabled = false
	marcarDesligado("Telegram não configurado")
	return falso
}

func alertaDaChave(t *testing.T, chave string) database.Alert {
	t.Helper()

	var a database.Alert
	if err := database.DB.Where("key = ?", chave).Order("id desc").Take(&a).Error; err != nil {
		t.Fatalf("alerta %q não está no banco: %v", chave, err)
	}
	return a
}

func TestSemCanalNaoDizQueEntregou(t *testing.T) {
	semCanal(t)
	chave := prefixoFila + "sem-canal-inalcancavel"

	if !Enqueue(Entrada{Key: chave, Text: "[CRITICO] VPS inalcançável"}) {
		t.Fatalf("o alerta não foi enfileirado")
	}
	dispatchPending(time.Now().UTC())

	a := alertaDaChave(t, chave)
	if a.Delivery == database.AlertDeliverySemCanal {
		if a.Status != database.AlertStatusOpen {
			t.Errorf("status = %q, o alerta precisa continuar aberto e visível", a.Status)
		}
		return
	}
	t.Fatalf("delivery = %q sem canal configurado; nada foi entregue, então não pode dizer %q",
		a.Delivery, a.Delivery)
}

func TestCanalDeVoltaRetomaOAlertaPreso(t *testing.T) {
	falso := semCanal(t)
	chave := prefixoFila + "sem-canal-retomada"

	Enqueue(Entrada{Key: chave, Text: "[CRITICO] preso sem canal"})
	dispatchPending(time.Now().UTC())
	if preso := alertaDaChave(t, chave); preso.Delivery != database.AlertDeliverySemCanal {
		t.Fatalf("preparação falhou: delivery = %q", preso.Delivery)
	}

	enabled = true
	marcarOK()
	dispatchPending(time.Now().UTC())

	a := alertaDaChave(t, chave)
	if a.Delivery != database.AlertDeliveryEnviado {
		t.Errorf("com o canal de volta o alerta preso ficou em %q, esperado enviado", a.Delivery)
	}
	if falso.enviosCom("[CRITICO] preso sem canal") == 0 {
		t.Errorf("o canal voltou e nada foi enviado de fato")
	}
}

func TestRetomadaRespeitaAJanela(t *testing.T) {
	semCanal(t)
	chave := prefixoFila + "sem-canal-antigo"

	Enqueue(Entrada{Key: chave, Text: "[CRITICO] incidente da semana passada"})
	dispatchPending(time.Now().UTC())

	velho := time.Now().UTC().Add(-72 * time.Hour)
	database.DB.Model(&database.Alert{}).Where("key = ?", chave).Update("created_at", velho)

	enabled = true
	marcarOK()
	t.Setenv("ALERT_RESUME_HOURS", "24")
	dispatchPending(time.Now().UTC())

	a := alertaDaChave(t, chave)
	if a.Delivery != database.AlertDeliverySemCanal {
		t.Errorf("alerta de 72 h foi retomado com janela de 24 h: delivery = %q", a.Delivery)
	}
}

func TestSupressaoEnxergaOAlertaPresoSemCanal(t *testing.T) {
	semCanal(t)
	chave := prefixoFila + "sem-canal-duplicata"

	Enqueue(Entrada{Key: chave, Text: "[CRITICO] primeiro"})
	dispatchPending(time.Now().UTC())

	if Enqueue(Entrada{Key: chave, Text: "[CRITICO] segundo"}) {
		t.Errorf("a mesma chave enfileirou de novo com um alerta preso sem canal; isso duplica sem parar")
	}

	var quantos int64
	database.DB.Model(&database.Alert{}).Where("key = ?", chave).Count(&quantos)
	if quantos != 1 {
		t.Errorf("%d alertas com a mesma chave, esperado 1", quantos)
	}
}
