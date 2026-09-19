package api

import (
	"testing"
	"time"
)

func TestHostOfflineAfterConfiguravelPorAmbiente(t *testing.T) {
	t.Setenv("API_TOKEN", "token-de-teste-d4")
	t.Setenv("HOST_OFFLINE_AFTER", "10m")
	t.Cleanup(func() { hostOfflineAfter = defaultHostOfflineAfter })
	if _, err := LoadConfig(":0"); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	agora := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	if hostOnline(agora.Add(-11*time.Minute), agora) {
		t.Error("com HOST_OFFLINE_AFTER=10m, host visto há 11 min apareceu online")
	}
	if !hostOnline(agora.Add(-9*time.Minute), agora) {
		t.Error("com HOST_OFFLINE_AFTER=10m, host visto há 9 min apareceu offline")
	}
}

func TestHostOfflineAfterPadraoEInvalido(t *testing.T) {
	t.Setenv("API_TOKEN", "token-de-teste-d4")
	t.Cleanup(func() { hostOfflineAfter = defaultHostOfflineAfter })

	for _, valor := range []string{"", "meia-hora", "0s", "-10m"} {
		t.Setenv("HOST_OFFLINE_AFTER", valor)
		if _, err := LoadConfig(":0"); err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if hostOfflineAfter != 30*time.Minute {
			t.Errorf("HOST_OFFLINE_AFTER=%q resultou em %s; o padrão é 30m", valor, hostOfflineAfter)
		}
	}
}
