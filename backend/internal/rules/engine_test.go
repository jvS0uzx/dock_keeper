package rules

import (
	"sort"
	"testing"

	"github.com/joaov/vd_stats/internal/database"
)

func siteID(id uint) *uint { return &id }

// Parque de exemplo: duas filiais e uma VPS de infraestrutura sem unidade.
func parqueExemplo() []database.Server {
	return []database.Server{
		{ID: "srv-norte-1", Name: "PC-RH", SiteID: siteID(1)},
		{ID: "srv-norte-2", Name: "PC-FIN", SiteID: siteID(1)},
		{ID: "srv-sul-1", Name: "PC-SUL", SiteID: siteID(2)},
		{ID: "vps-1", Name: "VPS-1", SiteID: nil},
	}
}

func assertTargets(t *testing.T, nome string, got, want []string) {
	t.Helper()

	sort.Strings(got)
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("%s: alvos = %v, esperado %v", nome, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: alvos = %v, esperado %v", nome, got, want)
		}
	}
}

func TestResolveTargetsTodos(t *testing.T) {
	rule := database.AlertRule{Target: "*"}

	assertTargets(t, "curinga", resolveTargets(rule, parqueExemplo()),
		[]string{"srv-norte-1", "srv-norte-2", "srv-sul-1", "vps-1"})
}

func TestResolveTargetsServidorEspecifico(t *testing.T) {
	rule := database.AlertRule{Target: "srv-sul-1"}

	assertTargets(t, "servidor", resolveTargets(rule, parqueExemplo()),
		[]string{"srv-sul-1"})
}

// O caso que motivou o campo: uma regra para a filial inteira, sem repetir a
// mesma configuração máquina a máquina.
func TestResolveTargetsPorUnidade(t *testing.T) {
	rule := database.AlertRule{Target: "*", TargetSiteID: siteID(1)}

	assertTargets(t, "unidade 1", resolveTargets(rule, parqueExemplo()),
		[]string{"srv-norte-1", "srv-norte-2"})
}

// A unidade vence o "*": a regra por unidade chega aqui sempre com Target="*",
// porque o handler força isso para o registro não guardar dois alvos.
func TestResolveTargetsUnidadeVenceCuringa(t *testing.T) {
	rule := database.AlertRule{Target: "*", TargetSiteID: siteID(2)}

	assertTargets(t, "precedência", resolveTargets(rule, parqueExemplo()),
		[]string{"srv-sul-1"})
}

// Filial recém-criada, ainda sem máquina: não pode virar alerta do parque
// inteiro nem estourar em pânico.
func TestResolveTargetsUnidadeSemServidores(t *testing.T) {
	rule := database.AlertRule{Target: "*", TargetSiteID: siteID(3)}

	got := resolveTargets(rule, parqueExemplo())
	if len(got) != 0 {
		t.Fatalf("unidade vazia devolveu %v, esperado nenhum alvo", got)
	}
}

func TestResolveTargetsUnidadeInexistente(t *testing.T) {
	rule := database.AlertRule{Target: "*", TargetSiteID: siteID(999)}

	got := resolveTargets(rule, parqueExemplo())
	if len(got) != 0 {
		t.Fatalf("unidade inexistente devolveu %v, esperado nenhum alvo", got)
	}
}

// Servidor sem unidade (VPS de infraestrutura) nunca entra numa regra por
// filial — é o escopo do painel Dev, não do Suporte TI.
func TestResolveTargetsIgnoraServidorSemUnidade(t *testing.T) {
	rule := database.AlertRule{Target: "*", TargetSiteID: siteID(1)}

	for _, id := range resolveTargets(rule, parqueExemplo()) {
		if id == "vps-1" {
			t.Fatal("servidor sem unidade entrou numa regra por filial")
		}
	}
}

func TestResolveTargetsParqueVazio(t *testing.T) {
	for nome, rule := range map[string]database.AlertRule{
		"curinga": {Target: "*"},
		"unidade": {Target: "*", TargetSiteID: siteID(1)},
	} {
		if got := resolveTargets(rule, nil); len(got) != 0 {
			t.Errorf("%s com parque vazio devolveu %v", nome, got)
		}
	}
}
