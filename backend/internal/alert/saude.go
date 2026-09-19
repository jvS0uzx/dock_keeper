package alert

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/safego"
)

const (
	EstadoDesligado = "desligado"
	EstadoOK        = "ok"
	EstadoDegradado = "degradado"
)

var (
	saudeBase = 30 * time.Second
	saudeMax  = 5 * time.Minute
)

type Saude struct {
	Estado       string     `json:"estado"`
	Detalhe      string     `json:"detalhe,omitempty"`
	VerificadoEm *time.Time `json:"verificado_em,omitempty"`
}

var (
	saudeMu      sync.Mutex
	estado       = EstadoDesligado
	detalhe      string
	verificadoEm time.Time
)

func Status() Saude {
	saudeMu.Lock()
	defer saudeMu.Unlock()

	s := Saude{Estado: estado, Detalhe: detalhe}
	if !verificadoEm.IsZero() {
		em := verificadoEm
		s.VerificadoEm = &em
	}
	return s
}

func marcarDesligado(motivo string) {
	saudeMu.Lock()
	defer saudeMu.Unlock()

	estado, detalhe, verificadoEm = EstadoDesligado, motivo, time.Time{}
}

func marcarOK() {
	saudeMu.Lock()
	defer saudeMu.Unlock()

	anterior := estado
	estado, detalhe, verificadoEm = EstadoOK, "", agora()
	if anterior == EstadoDegradado {
		log.Println("[Alert] Telegram voltou a responder: canal ok")
	}
}

func marcarDegradado(err error) {
	saudeMu.Lock()
	defer saudeMu.Unlock()

	anterior := estado
	estado, detalhe, verificadoEm = EstadoDegradado, err.Error(), agora()
	if anterior != EstadoDegradado {
		log.Printf("[Alert] canal do Telegram degradado: %v", err)
	}
}

func verificarCanal() (string, error) {
	nome, err := verifyBot()
	if err != nil {
		marcarDegradado(err)
		return "", err
	}
	if err := verifyChat(); err != nil {
		marcarDegradado(err)
		return "", err
	}

	marcarOK()
	return nome, nil
}

func StartHealthWatch(ctx context.Context) <-chan struct{} {
	if !enabled {
		fim := make(chan struct{})
		close(fim)
		return fim
	}
	return safego.Run(ctx, "alert:saude", vigiarCanal)
}

func vigiarCanal(ctx context.Context) {
	espera := saudeBase
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(espera):
		}

		if _, err := verificarCanal(); err == nil {
			espera = saudeMax
			continue
		}
		espera = min(espera*2, saudeMax)
	}
}
