package rules

import (
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func TestBreachStartSemEstadoComecaAgora(t *testing.T) {
	agora := time.Now()

	if got := breachStart(database.AlertState{}, false, agora); !got.Equal(agora) {
		t.Errorf("sem estado gravado, a sequência = %v, esperado começar agora (%v)", got, agora)
	}
}

func TestBreachStartMantemSequenciaContinua(t *testing.T) {
	agora := time.Now()
	inicio := agora.Add(-4 * time.Minute)

	state := database.AlertState{
		FirstBreachAt: inicio,
		LastBreachAt:  agora.Add(-tickInterval),
	}

	if got := breachStart(state, true, agora); !got.Equal(inicio) {
		t.Errorf("sequência contínua = %v, esperado preservar o início %v", got, inicio)
	}
}

func TestBreachStartZeradoRecomecaAContagem(t *testing.T) {
	agora := time.Now()

	zerado := database.AlertState{
		FirstBreachAt: time.Time{},
		LastBreachAt:  agora.Add(-tickInterval),
	}

	if got := breachStart(zerado, true, agora); !got.Equal(agora) {
		t.Errorf("depois de uma amostra dentro do limite, a sequência = %v, esperado recomeçar em %v", got, agora)
	}
}

func TestBreachStartEstadoEncerradoRecomecaAContagem(t *testing.T) {
	agora := time.Now()

	encerrado := database.AlertState{
		FirstBreachAt: time.Time{},
		LastBreachAt:  time.Time{},
	}

	if got := breachStart(encerrado, true, agora); !got.Equal(agora) {
		t.Errorf("estado encerrado devolveu %v, esperado recomeçar em %v", got, agora)
	}
}

func TestBreachStartToleraTickPerdido(t *testing.T) {
	agora := time.Now()
	inicio := agora.Add(-10 * time.Minute)

	state := database.AlertState{
		FirstBreachAt: inicio,
		LastBreachAt:  agora.Add(-breachGap() + time.Second),
	}

	if got := breachStart(state, true, agora); !got.Equal(inicio) {
		t.Errorf("tick perdido zerou a contagem: sequência = %v, esperado %v", got, inicio)
	}
}

func TestBreachStartBuracoLongoRecomeca(t *testing.T) {
	agora := time.Now()

	state := database.AlertState{
		FirstBreachAt: agora.Add(-2 * time.Hour),
		LastBreachAt:  agora.Add(-time.Hour),
	}

	if got := breachStart(state, true, agora); !got.Equal(agora) {
		t.Errorf("depois de uma hora sem avaliação, a sequência = %v, esperado recomeçar em %v", got, agora)
	}
}

func TestBreachGapAcompanhaOTick(t *testing.T) {
	original := tickInterval
	t.Cleanup(func() { tickInterval = original })

	tickInterval = 5 * time.Minute
	if got := breachGap(); got != 10*time.Minute {
		t.Errorf("com tick de 5min, a tolerância = %v, esperado 10min", got)
	}

	tickInterval = 5 * time.Second
	if got := breachGap(); got != 90*time.Second {
		t.Errorf("com tick de 5s, a tolerância = %v, esperado o piso de 90s", got)
	}
}
