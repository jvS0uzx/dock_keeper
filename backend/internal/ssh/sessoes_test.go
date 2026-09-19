package ssh

import (
	"errors"
	"testing"
)

func TestLimiteDeSessoesPorHost(t *testing.T) {
	t.Setenv("SSH_MAX_SESSIONS_PER_HOST", "2")
	alvo := Target{ID: "srv-limite-a"}
	outro := Target{ID: "srv-limite-b"}

	r1, err := AcquireSession(alvo)
	if err != nil {
		t.Fatalf("primeira sessão recusada: %v", err)
	}
	r2, err := AcquireSession(alvo)
	if err != nil {
		t.Fatalf("segunda sessão recusada: %v", err)
	}
	if _, err := AcquireSession(alvo); !errors.Is(err, ErrSessionLimit) {
		t.Fatalf("terceira sessão simultânea com limite 2: erro %v, esperado ErrSessionLimit", err)
	}

	r3, err := AcquireSession(outro)
	if err != nil {
		t.Errorf("o limite de um host bloqueou outro host: %v", err)
	} else {
		r3()
	}

	r1()
	r4, err := AcquireSession(alvo)
	if err != nil {
		t.Errorf("sessão liberada não devolveu a vaga: %v", err)
	} else {
		r4()
	}
	r2()
}

func TestLimiteDeSessoesPadrao(t *testing.T) {
	for _, valor := range []string{"", "x", "0", "-3"} {
		t.Setenv("SSH_MAX_SESSIONS_PER_HOST", valor)
		if got := maxSessionsPerHost(); got != 6 {
			t.Errorf("SSH_MAX_SESSIONS_PER_HOST=%q resultou em %d; o padrão é 6", valor, got)
		}
	}
}
