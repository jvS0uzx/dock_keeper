package alert

import (
	"os"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const chaveCooldown = "e12-teste:host-x"

func setupCooldownDB(t *testing.T) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de cooldown persistente")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	resetState()
	t.Cleanup(resetState)
}

func TestClaimSlotNaoCriaEstadoSemRegra(t *testing.T) {
	setupCooldownDB(t)

	if !claimSlot(chaveCooldown) {
		t.Fatal("primeiro disparo foi bloqueado")
	}

	var linhas int64
	database.DB.Model(&database.AlertState{}).Where("key = ?", chaveCooldown).Count(&linhas)
	if linhas != 0 {
		t.Errorf("%d linha(s) em alert_states sem regra dona; a integridade referencial as recusa", linhas)
	}
}

func TestBancoIndisponivelNaoEngoleOAlerta(t *testing.T) {
	resetState()
	original := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = original
		resetState()
	})

	if !claimSlot("e12-teste:sem-banco") {
		t.Error("com o banco indisponível o alerta foi engolido")
	}
}

func TestNotifyDizSeOAvisoSaiu(t *testing.T) {
	setupCooldownDB(t)

	if !Notify(chaveCooldown, "[ALERTA] teste") {
		t.Fatal("primeiro Notify devolveu false")
	}
	if Notify(chaveCooldown, "[ALERTA] teste") {
		t.Error("Notify dentro do cooldown devolveu true; a recuperação seria prometida sem alerta")
	}
}
