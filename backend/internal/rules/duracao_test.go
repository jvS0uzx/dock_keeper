package rules

import (
	"testing"
	"time"

	"github.com/joaov/vd_stats/internal/database"
)

// breachStart é o coração da regra de duração: sem ele, cada avaliação só
// enxerga a própria amostra e "acima de X por 5 minutos" seria indistinguível
// de "acima de X agora".
func TestBreachStartSemEstadoComecaAgora(t *testing.T) {
	agora := time.Now()

	if got := breachStart(database.AlertState{}, false, agora); !got.Equal(agora) {
		t.Errorf("sem estado gravado, a sequência = %v, esperado começar agora (%v)", got, agora)
	}
}

// Violação observada no tick anterior é a mesma sequência: é o caso normal, e é
// o que faz a contagem avançar em vez de reiniciar a cada avaliação.
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

// O caso decisivo do E9: acima, abaixo, acima. A amostra dentro do limite zera
// o estado em flushSettled, e a violação seguinte precisa recomeçar do zero —
// senão "por 5 minutos seguidos" viraria "5 minutos somados ao longo do dia", e
// um host oscilando alertaria como se estivesse constantemente ruim.
//
// LastBreachAt fica RECENTE de propósito: é o que isola esta guarda da guarda de
// intervalo. Com as duas marcas zeradas, o intervalo até a época zero já passa
// de breachGap e o teste passaria mesmo sem a conferência de FirstBreachAt —
// provando o comportamento por acidente da representação do tempo, não por
// decisão. Assim, zerar só first_breach_at continua bastando.
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

// O estado como flushSettled de fato o grava, com as duas marcas zeradas. É o
// contraponto do teste acima: lá se prova que zerar first_breach_at basta, aqui
// que o par que o código realmente escreve também recomeça a contagem.
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

// Um tick perdido — coleta atrasada, banco lento — não pode zerar uma contagem
// de cinco minutos. É por isso que a tolerância são dois ticks e não um.
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

// O outro buraco, que o zeramento explícito não cobre: enquanto o painel esteve
// fora do ar nenhuma amostra foi avaliada, então não há registro de interrupção
// para ler. Sem esta guarda, um reinício depois de uma hora fora dispararia na
// primeira avaliação como se tivesse observado a hora inteira.
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

// A tolerância acompanha o tick configurado: um painel avaliando a cada 5
// minutos não pode considerar interrompida uma sequência por causa de um piso
// pensado para ticks de 30 segundos.
func TestBreachGapAcompanhaOTick(t *testing.T) {
	original := tickInterval
	t.Cleanup(func() { tickInterval = original })

	tickInterval = 5 * time.Minute
	if got := breachGap(); got != 10*time.Minute {
		t.Errorf("com tick de 5min, a tolerância = %v, esperado 10min", got)
	}

	// Tick curto não derruba a tolerância abaixo do piso: com 5 s de tick, dois
	// ticks seriam 10 s, e qualquer soluço zeraria toda contagem em andamento.
	tickInterval = 5 * time.Second
	if got := breachGap(); got != 90*time.Second {
		t.Errorf("com tick de 5s, a tolerância = %v, esperado o piso de 90s", got)
	}
}
