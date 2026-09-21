package alert

import (
	"log"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	CanalTelegram = "telegram"
	CanalWebhook  = "webhook"
	CanalNtfy     = "ntfy"
)

type Canal interface {
	Nome() string
	Ativo() bool
	Entregar(database.Alert) error
}

var registro = []Canal{canalTelegram{}, canalWebhook{}, canalNtfy{}}

func canaisAtivos() []Canal {
	ativos := make([]Canal, 0, len(registro))
	for _, c := range registro {
		if c.Ativo() {
			ativos = append(ativos, c)
		}
	}
	return ativos
}

func CanaisAtivos() []string {
	ativos := canaisAtivos()
	nomes := make([]string, 0, len(ativos))
	for _, c := range ativos {
		nomes = append(nomes, c.Nome())
	}
	return nomes
}

func Send(msg string) {
	ativos := canaisAtivos()
	if len(ativos) == 0 {
		log.Printf("[Alert] (sem canal configurado) %s", msg)
		return
	}

	alerta := database.Alert{Text: msg, Severity: severityFromText(msg), Status: database.AlertStatusOpen}
	for _, c := range ativos {
		if err := c.Entregar(alerta); err != nil {
			log.Printf("[Alert] falha ao enviar por %s: %v", c.Nome(), err)
		}
	}
}
