package api

import (
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/versao"
)

func TestReadyzSoMostraAVersaoParaQuemTemCredencial(t *testing.T) {
	setupAuditAPI(t)
	original := versao.Versao
	versao.Versao = "9.9.9-teste"
	t.Cleanup(func() { versao.Versao = original })

	for _, rota := range []string{"/readyz", "/api/readyz"} {
		_, anonimo := readyz(t, rota, false)
		if _, vazou := anonimo["versao"]; vazou {
			t.Errorf("%s sem credencial publicou a versão: %v", rota, anonimo["versao"])
		}
		_, completo := readyz(t, rota, true)
		if completo["versao"] != "9.9.9-teste" {
			t.Errorf("%s com credencial: versao = %v, esperado 9.9.9-teste", rota, completo["versao"])
		}
	}
}
