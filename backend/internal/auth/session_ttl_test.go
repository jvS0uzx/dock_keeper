package auth

import (
	"testing"
	"time"
)

func TestSessionTTLConfiguravelPorAmbiente(t *testing.T) {
	setupSessaoDB(t)
	t.Setenv("SESSION_TTL", "1h")
	Configure()
	t.Cleanup(func() { sessionTTL = DefaultSessionTTL })

	antes := time.Now()
	s, err := Login(usuarioDeSessao, senhaDeSessao)
	depois := time.Now()
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if s.ExpiresAt.Before(antes.Add(time.Hour)) || s.ExpiresAt.After(depois.Add(time.Hour)) {
		t.Errorf("com SESSION_TTL=1h a sessão expira em %s; esperado login + 1h (entre %s e %s)",
			s.ExpiresAt, antes.Add(time.Hour), depois.Add(time.Hour))
	}
}

func TestSessionTTLPadraoEInvalido(t *testing.T) {
	t.Cleanup(func() { sessionTTL = DefaultSessionTTL })

	for _, valor := range []string{"", "doze", "0s", "-1h"} {
		t.Setenv("SESSION_TTL", valor)
		Configure()
		if sessionTTL != 12*time.Hour {
			t.Errorf("SESSION_TTL=%q resultou em %s; o padrão é 12h", valor, sessionTTL)
		}
	}
}
