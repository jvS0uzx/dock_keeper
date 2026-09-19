package ssh

import (
	"testing"
	"time"
)

func semJitter(d time.Duration) time.Duration { return d }

func TestBackoffDobraAteOTeto(t *testing.T) {
	b := newBackoff(5*time.Minute, semJitter)

	esperado := []time.Duration{5, 10, 20, 40, 80, 160, 300, 300}
	for i, seg := range esperado {
		if got := b.next(time.Second); got != seg*time.Second {
			t.Errorf("espera %d = %s, esperado %s", i+1, got, seg*time.Second)
		}
	}
}

func TestBackoffVoltaAoInicioDepoisDeSessaoSaudavel(t *testing.T) {
	b := newBackoff(5*time.Minute, semJitter)
	for range 4 {
		b.next(time.Second)
	}

	if got := b.next(60 * time.Second); got != 5*time.Second {
		t.Errorf("depois de uma sessão de 60 s a espera foi %s, esperado 5s", got)
	}
	if got := b.next(59 * time.Second); got != 10*time.Second {
		t.Errorf("sessão de 59 s não é saudável: espera %s, esperado 10s", got)
	}
}

func TestBackoffJitterFicaEmVintePorCento(t *testing.T) {
	nominal := []time.Duration{5, 10, 20, 40, 80, 160, 300, 300}

	for rodada := 0; rodada < 200; rodada++ {
		b := newBackoff(5*time.Minute, jitter)
		for _, seg := range nominal {
			d := seg * time.Second
			got := b.next(time.Second)
			piso := time.Duration(float64(d) * 0.8)
			teto := time.Duration(float64(d) * 1.2)
			if got < piso || got > teto {
				t.Fatalf("espera %s fora de ±20%% de %s", got, d)
			}
		}
	}
}

func TestReconnectMaxConfiguravelPorAmbiente(t *testing.T) {
	t.Setenv("SSH_RECONNECT_MAX", "1m")
	if got := reconnectMax(); got != time.Minute {
		t.Errorf("SSH_RECONNECT_MAX=1m resultou em %s", got)
	}

	for _, valor := range []string{"", "x", "0s", "-1m"} {
		t.Setenv("SSH_RECONNECT_MAX", valor)
		if got := reconnectMax(); got != 5*time.Minute {
			t.Errorf("SSH_RECONNECT_MAX=%q resultou em %s; o padrão é 5m", valor, got)
		}
	}

	t.Setenv("SSH_RECONNECT_MAX", "1m")
	b := newBackoff(reconnectMax(), semJitter)
	var ultimo time.Duration
	for range 6 {
		ultimo = b.next(time.Second)
	}
	if ultimo != time.Minute {
		t.Errorf("com teto de 1m a espera chegou a %s", ultimo)
	}
}
