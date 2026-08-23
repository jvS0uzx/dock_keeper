package alert

import (
	"os"
	"testing"
	"time"

	"github.com/joaov/vd_stats/internal/database"
)

const chaveCooldown = "e12-teste:host-x"

// setupCooldownDB liga no Postgres de desenvolvimento. Sem DATABASE_URL o teste
// é pulado: a suíte precisa passar numa máquina sem banco.
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

// gravarDisparo simula um aviso enviado por uma execução anterior do processo.
func gravarDisparo(t *testing.T, key string, quandoAtras time.Duration) {
	t.Helper()

	at := time.Now().Add(-quandoAtras)
	row := database.AlertState{Key: key, LastNotifiedAt: &at, UpdatedAt: at}
	if err := database.DB.Create(&row).Error; err != nil {
		t.Fatalf("gravar disparo anterior: %v", err)
	}
}

// O item E12: o cooldown vivia num mapa em memória, zerado a cada reinício. Um
// painel que reinicia — deploy, atualização, queda — recomeçava notificando
// tudo de novo, e o operador aprendia a ignorar o canal.
//
// O mapa vazio aqui é justamente o processo recém-subido.
func TestCooldownSobreviveAoReinicioDoProcesso(t *testing.T) {
	setupCooldownDB(t)
	gravarDisparo(t, chaveCooldown, time.Minute)

	if claimSlot(chaveCooldown) {
		t.Error("processo reiniciado reenviou um alerta disparado há 1 minuto")
	}
}

// O contraponto que impede a correção de virar "nunca mais alerta": passado o
// cooldown, o disparo volta a ser liberado.
func TestCooldownVencidoNoBancoLiberaODisparo(t *testing.T) {
	setupCooldownDB(t)
	gravarDisparo(t, chaveCooldown, cooldown+time.Minute)

	if !claimSlot(chaveCooldown) {
		t.Error("disparo de mais de 30 minutos atrás continuou bloqueado")
	}
}

// Sem a gravação, o cooldown volta a morrer com o processo.
func TestClaimSlotGravaODisparoNoBanco(t *testing.T) {
	setupCooldownDB(t)

	if !claimSlot(chaveCooldown) {
		t.Fatal("primeiro disparo foi bloqueado")
	}

	var row database.AlertState
	if err := database.DB.Where("key = ?", chaveCooldown).Take(&row).Error; err != nil {
		t.Fatalf("o disparo não foi persistido: %v", err)
	}
	if row.LastNotifiedAt == nil {
		t.Error("a linha foi criada sem last_notified_at")
	}
}

// Banco fora do ar não pode virar silêncio: alerta duplicado incomoda, alerta
// perdido mata. É a assimetria que a decisão do E12 assume de propósito.
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

// O motor de regras precisa saber se o aviso realmente saiu: só um aviso
// entregue autoriza prometer o "voltou ao normal" depois.
func TestNotifyDizSeOAvisoSaiu(t *testing.T) {
	setupCooldownDB(t)

	if !Notify(chaveCooldown, "[ALERTA] teste") {
		t.Fatal("primeiro Notify devolveu false")
	}
	if Notify(chaveCooldown, "[ALERTA] teste") {
		t.Error("Notify dentro do cooldown devolveu true; a recuperação seria prometida sem alerta")
	}
}
