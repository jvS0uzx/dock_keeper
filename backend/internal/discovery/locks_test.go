package discovery

import (
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func TestVarreduraNaoDesfazTipoTravado(t *testing.T) {
	setupDB(t)

	persist([]Host{{IP: testIPKnown, OpenPorts: []int{80, 9100}}}, nil)
	if got := fetch(t, testIPKnown).DeviceType; got != TypePrinter {
		t.Fatalf("tipo inferido = %q, esperado %q", got, TypePrinter)
	}

	database.DB.Model(&database.NetworkHost{}).Where("ip = ?", testIPKnown).
		Updates(map[string]any{"device_type": TypeNAS, "device_type_locked": true})

	persist([]Host{{IP: testIPKnown, OpenPorts: []int{80, 9100}}}, nil)

	host := fetch(t, testIPKnown)
	if host.DeviceType != TypeNAS {
		t.Errorf("tipo travado = %q, a varredura desfez a correção", host.DeviceType)
	}
	if !host.DeviceTypeLocked {
		t.Error("a trava foi perdida no upsert")
	}
}

func TestVarreduraAtualizaTipoNaoTravado(t *testing.T) {
	setupDB(t)

	persist([]Host{{IP: testIPKnown, OpenPorts: []int{22}}}, nil)
	if got := fetch(t, testIPKnown).DeviceType; got != TypeLinux {
		t.Fatalf("tipo inicial = %q, esperado %q", got, TypeLinux)
	}

	persist([]Host{{IP: testIPKnown, OpenPorts: []int{22, 3389}}}, nil)

	if got := fetch(t, testIPKnown).DeviceType; got != TypeWindows {
		t.Errorf("tipo = %q, esperado %q — sem trava o valor deve seguir as portas", got, TypeWindows)
	}
}

func TestVarreduraNaoRevertUnidadeTravada(t *testing.T) {
	setupDB(t)

	matriz := criarUnidade(t, "qa-matriz")
	filial := criarUnidade(t, "qa-filial")

	persist([]Host{{IP: testIPKnown, OpenPorts: []int{22}}}, &matriz)
	if got := fetch(t, testIPKnown).SiteID; got == nil || *got != matriz {
		t.Fatalf("unidade inicial = %v, esperada a matriz", got)
	}

	database.DB.Model(&database.NetworkHost{}).Where("ip = ?", testIPKnown).
		Updates(map[string]any{"site_id": filial, "site_locked": true})

	persist([]Host{{IP: testIPKnown, OpenPorts: []int{22}}}, &matriz)

	host := fetch(t, testIPKnown)
	if host.SiteID == nil || *host.SiteID != filial {
		t.Errorf("unidade = %v, esperada a filial: a varredura reverteu a escolha manual", host.SiteID)
	}
}

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
