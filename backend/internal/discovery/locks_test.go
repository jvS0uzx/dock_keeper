package discovery

import (
	"testing"

	"github.com/joaov/vd_stats/internal/database"
)

// Regressão do achado 1 do QA: a varredura reinferia device_type a cada ciclo e
// desfazia a correção do técnico em no máximo 15 minutos.
func TestVarreduraNaoDesfazTipoTravado(t *testing.T) {
	setupDB(t)

	// Primeira varredura: 9100 aberta, o inferidor diz "impressora".
	persist([]Host{{IP: testIPKnown, OpenPorts: []int{80, 9100}}}, nil)
	if got := fetch(t, testIPKnown).DeviceType; got != TypePrinter {
		t.Fatalf("tipo inferido = %q, esperado %q", got, TypePrinter)
	}

	// O operador corrige a mão para NAS e a trava é ligada.
	database.DB.Model(&database.NetworkHost{}).Where("ip = ?", testIPKnown).
		Updates(map[string]any{"device_type": TypeNAS, "device_type_locked": true})

	// Ciclo seguinte com as mesmas portas: sem a trava, voltaria a impressora.
	persist([]Host{{IP: testIPKnown, OpenPorts: []int{80, 9100}}}, nil)

	host := fetch(t, testIPKnown)
	if host.DeviceType != TypeNAS {
		t.Errorf("tipo travado = %q, a varredura desfez a correção", host.DeviceType)
	}
	if !host.DeviceTypeLocked {
		t.Error("a trava foi perdida no upsert")
	}
}

// O contraponto: sem trava, o tipo continua acompanhando as portas.
func TestVarreduraAtualizaTipoNaoTravado(t *testing.T) {
	setupDB(t)

	persist([]Host{{IP: testIPKnown, OpenPorts: []int{22}}}, nil)
	if got := fetch(t, testIPKnown).DeviceType; got != TypeLinux {
		t.Fatalf("tipo inicial = %q, esperado %q", got, TypeLinux)
	}

	// A máquina passou a expor 3389: agora é estação Windows.
	persist([]Host{{IP: testIPKnown, OpenPorts: []int{22, 3389}}}, nil)

	if got := fetch(t, testIPKnown).DeviceType; got != TypeWindows {
		t.Errorf("tipo = %q, esperado %q — sem trava o valor deve seguir as portas", got, TypeWindows)
	}
}

// Regressão do achado 2: o coletor sempre manda a unidade dele, então o
// COALESCE antigo revertia todo host que o operador tivesse movido.
func TestVarreduraNaoRevertUnidadeTravada(t *testing.T) {
	setupDB(t)

	matriz := criarUnidade(t, "qa-matriz")
	filial := criarUnidade(t, "qa-filial")

	// A varredura da matriz descobre o host e o classifica.
	persist([]Host{{IP: testIPKnown, OpenPorts: []int{22}}}, &matriz)
	if got := fetch(t, testIPKnown).SiteID; got == nil || *got != matriz {
		t.Fatalf("unidade inicial = %v, esperada a matriz", got)
	}

	// O operador move o host para a filial; a trava é ligada.
	database.DB.Model(&database.NetworkHost{}).Where("ip = ?", testIPKnown).
		Updates(map[string]any{"site_id": filial, "site_locked": true})

	// A varredura da matriz roda de novo e insiste na unidade dela.
	persist([]Host{{IP: testIPKnown, OpenPorts: []int{22}}}, &matriz)

	host := fetch(t, testIPKnown)
	if host.SiteID == nil || *host.SiteID != filial {
		t.Errorf("unidade = %v, esperada a filial: a varredura reverteu a escolha manual", host.SiteID)
	}
}

// Sem trava, a varredura classifica o host que ainda não tinha unidade — que é
// justamente o achado 3 (varredura local nunca escrevia site_id).
func TestVarreduraClassificaHostSemUnidade(t *testing.T) {
	setupDB(t)

	unidade := criarUnidade(t, "qa-classifica")

	persist([]Host{{IP: testIPUnnamed, OpenPorts: []int{22}}}, nil)
	if got := fetch(t, testIPUnnamed).SiteID; got != nil {
		t.Fatalf("unidade inicial = %v, esperada nenhuma", got)
	}

	persist([]Host{{IP: testIPUnnamed, OpenPorts: []int{22}}}, &unidade)

	host := fetch(t, testIPUnnamed)
	if host.SiteID == nil || *host.SiteID != unidade {
		t.Errorf("unidade = %v, esperada %d", host.SiteID, unidade)
	}
	if host.SiteLocked {
		t.Error("classificação automática não pode ligar a trava do operador")
	}
}

// criarUnidade cria uma unidade descartável e a remove ao fim do teste.
func criarUnidade(t *testing.T, code string) uint {
	t.Helper()

	database.DB.Where("code = ?", code).Delete(&database.Site{})
	site := database.Site{Name: code, Code: code}
	if err := database.DB.Create(&site).Error; err != nil {
		t.Fatalf("criar unidade %s: %v", code, err)
	}
	t.Cleanup(func() {
		database.DB.Where("id = ?", site.ID).Delete(&database.Site{})
	})
	return site.ID
}
