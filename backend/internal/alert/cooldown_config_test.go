package alert

import (
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func isolarCooldownEmMemoria(t *testing.T) {
	t.Helper()

	original := database.DB
	database.DB = nil
	resetState()
	t.Cleanup(func() {
		database.DB = original
		agora = time.Now
		cooldown = defaultCooldown
		resetState()
	})
}

func TestAlertCooldownConfiguravelPorAmbiente(t *testing.T) {
	isolarCooldownEmMemoria(t)
	t.Setenv("ALERT_COOLDOWN", "1m")
	configure()

	base := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	agora = func() time.Time { return base }
	if !claimSlot("d4:cooldown") {
		t.Fatal("o primeiro aviso da chave foi suprimido")
	}

	agora = func() time.Time { return base.Add(59 * time.Second) }
	if claimSlot("d4:cooldown") {
		t.Error("com ALERT_COOLDOWN=1m o segundo aviso saiu 59 s depois do primeiro")
	}

	agora = func() time.Time { return base.Add(61 * time.Second) }
	if !claimSlot("d4:cooldown") {
		t.Error("com ALERT_COOLDOWN=1m o segundo aviso continuou suprimido 61 s depois do primeiro")
	}
}

func TestAlertCooldownPadraoEInvalido(t *testing.T) {
	isolarCooldownEmMemoria(t)

	for _, valor := range []string{"", "abc", "0s", "-5m"} {
		t.Setenv("ALERT_COOLDOWN", valor)
		configure()
		if cooldown != 30*time.Minute {
			t.Errorf("ALERT_COOLDOWN=%q resultou em %s; o padrão é 30m", valor, cooldown)
		}
	}
}
