package database

import (
	"testing"
	"time"
)

func TestLiveWindowForDerivaDoIntervalo(t *testing.T) {
	casos := []struct {
		nome        string
		intervalSec int
		querJanela  time.Duration
	}{
		{"desconhecido usa o piso", 0, MinLiveWindow},
		{"negativo usa o piso", -5, MinLiveWindow},
		{"intervalo curto não desce do piso", 5, MinLiveWindow},
		{"exatamente no piso continua no piso", 10, MinLiveWindow},
		{"intervalo de 15s passa do piso", 15, 45 * time.Second},
		{"intervalo longo vira 3 ciclos", 60, 180 * time.Second},
		{"intervalo de 120s vira 6 minutos", 120, 360 * time.Second},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := LiveWindowFor(c.intervalSec); got != c.querJanela {
				t.Errorf("LiveWindowFor(%d) = %v, esperado %v", c.intervalSec, got, c.querJanela)
			}
		})
	}
}
