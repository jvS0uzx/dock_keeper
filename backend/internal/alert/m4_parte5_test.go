package alert

import (
	"testing"

	"gorm.io/gorm"

	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/observabilidade"
)

func TestFalhaAoEnfileirarDescartaSemChamarOTelegram(t *testing.T) {
	falso := setupFila(t)

	original := database.DB
	quebrado := original.Session(&gorm.Session{NewDB: true}).Table("tabela_que_nao_existe_alerts")
	database.DB = quebrado
	t.Cleanup(func() { database.DB = original })

	antes := observabilidade.AlertasDescartados.Value()
	if Notify(prefixoFila+"insert-quebrado", "[CRITICO] banco fora") {
		t.Error("Notify devolveu true com o INSERT falhando")
	}

	if n := observabilidade.AlertasDescartados.Value(); n != antes+1 {
		t.Errorf("contador de descartados = %d, esperado %d", n, antes+1)
	}
	if n := falso.enviosCom("[CRITICO] banco fora"); n != 0 {
		t.Errorf("o Telegram foi chamado %d vez(es) no caminho de quem alerta", n)
	}
}
