package discovery

import (
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"

	"github.com/jvS0uzx/dockkeeper_collector/scan"
)

func contarHosts(t *testing.T, ip string) int64 {
	t.Helper()

	var n int64
	if err := database.DB.Model(&database.NetworkHost{}).
		Where("ip = ?", ip).Count(&n).Error; err != nil {
		t.Fatalf("contar hosts de %s: %v", ip, err)
	}
	return n
}

func TestVarreduraNaoDuplicaHostAoClassificar(t *testing.T) {
	setupDB(t)

	unidade := criarUnidade(t, "qa-adocao")

	persist([]scan.Host{{IP: testIPUnnamed, OpenPorts: []int{22}}}, nil)
	if n := contarHosts(t, testIPUnnamed); n != 1 {
		t.Fatalf("linhas após a primeira varredura = %d, esperada 1", n)
	}

	persist([]scan.Host{{IP: testIPUnnamed, OpenPorts: []int{22}}}, &unidade)

	if n := contarHosts(t, testIPUnnamed); n != 1 {
		t.Errorf("linhas após a classificação = %d, esperada 1: o host foi duplicado", n)
	}
	if got := fetch(t, testIPUnnamed).SiteID; got == nil || *got != unidade {
		t.Errorf("unidade = %v, esperada %d", got, unidade)
	}
}

func TestAdocaoRespeitaATravaDoOperador(t *testing.T) {
	setupDB(t)

	unidade := criarUnidade(t, "qa-adocao-travada")

	persist([]scan.Host{{IP: testIPUnnamed, OpenPorts: []int{22}}}, nil)
	database.DB.Model(&database.NetworkHost{}).Where("ip = ?", testIPUnnamed).
		Update("site_locked", true)

	persist([]scan.Host{{IP: testIPUnnamed, OpenPorts: []int{22}}}, &unidade)

	if got := fetch(t, testIPUnnamed).SiteID; got != nil {
		t.Errorf("unidade = %v, esperada nenhuma: a adoção passou por cima da trava", got)
	}
}
