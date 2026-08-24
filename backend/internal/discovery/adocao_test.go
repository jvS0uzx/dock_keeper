package discovery

import (
	"testing"

	"github.com/joaov/vd_stats/internal/database"
)

// contarHosts conta as linhas do inventário para um endereço. É o observável que
// separa "adotou a linha existente" de "criou uma segunda linha": o teste de
// classificação sozinho não distinguia os dois, porque lia só a primeira.
func contarHosts(t *testing.T, ip string) int64 {
	t.Helper()

	var n int64
	if err := database.DB.Model(&database.NetworkHost{}).
		Where("ip = ?", ip).Count(&n).Error; err != nil {
		t.Fatalf("contar hosts de %s: %v", ip, err)
	}
	return n
}

// Regressão da chave composta do inventário: com o índice único em
// (COALESCE(site_id,0), ip), a linha que a varredura deixou sem unidade tem
// chave (0, ip) e não colide com a da varredura que já conhece a unidade. Sem a
// adoção prévia o mesmo endereço vira duas linhas — uma órfã, que continua
// aparecendo na tela, e uma classificada.
func TestVarreduraNaoDuplicaHostAoClassificar(t *testing.T) {
	setupDB(t)

	unidade := criarUnidade(t, "qa-adocao")

	persist([]Host{{IP: testIPUnnamed, OpenPorts: []int{22}}}, nil)
	if n := contarHosts(t, testIPUnnamed); n != 1 {
		t.Fatalf("linhas após a primeira varredura = %d, esperada 1", n)
	}

	persist([]Host{{IP: testIPUnnamed, OpenPorts: []int{22}}}, &unidade)

	if n := contarHosts(t, testIPUnnamed); n != 1 {
		t.Errorf("linhas após a classificação = %d, esperada 1: o host foi duplicado", n)
	}
	if got := fetch(t, testIPUnnamed).SiteID; got == nil || *got != unidade {
		t.Errorf("unidade = %v, esperada %d", got, unidade)
	}
}

// O contraponto que impede a adoção de virar sequestro: unidade nula escolhida a
// mão pelo operador continua nula depois da varredura.
func TestAdocaoRespeitaATravaDoOperador(t *testing.T) {
	setupDB(t)

	unidade := criarUnidade(t, "qa-adocao-travada")

	persist([]Host{{IP: testIPUnnamed, OpenPorts: []int{22}}}, nil)
	database.DB.Model(&database.NetworkHost{}).Where("ip = ?", testIPUnnamed).
		Update("site_locked", true)

	persist([]Host{{IP: testIPUnnamed, OpenPorts: []int{22}}}, &unidade)

	if got := fetch(t, testIPUnnamed).SiteID; got != nil {
		t.Errorf("unidade = %v, esperada nenhuma: a adoção passou por cima da trava", got)
	}
}
