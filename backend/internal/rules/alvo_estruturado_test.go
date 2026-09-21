package rules

import (
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func TestEntradaDoAlertaLevaOAlvoDoHostEAMedida(t *testing.T) {
	rule := database.AlertRule{ID: 3, Name: "CPU alta", Metric: "cpu", Operator: ">", Threshold: 90, Severity: SeverityHigh}

	e := entradaDoAlerta(rule, "srv-1", nil, "rule:3:srv-1", "texto", SeverityHigh, "vps-teste", 97.5)

	if e.AlvoTipo != database.AlvoTipoHost {
		t.Errorf("AlvoTipo = %q, esperado %q", e.AlvoTipo, database.AlvoTipoHost)
	}
	if e.AlvoID != "srv-1" || e.AlvoNome != "vps-teste" {
		t.Errorf("alvo = (%q, %q), esperado o id e o nome do servidor", e.AlvoID, e.AlvoNome)
	}
	if e.Metrica != "cpu" {
		t.Errorf("Metrica = %q, esperado cpu", e.Metrica)
	}
	if e.Valor == nil || *e.Valor != 97.5 {
		t.Errorf("Valor = %v, esperado o valor observado", e.Valor)
	}
	if e.Limiar == nil || *e.Limiar != 90 {
		t.Errorf("Limiar = %v, esperado o limiar da regra", e.Limiar)
	}
	if e.Unidade != "%" {
		t.Errorf("Unidade = %q, esperada a unidade do catálogo de métricas", e.Unidade)
	}
}

func TestEntradaDoAlertaComMetricaDesconhecidaNaoInventaUnidade(t *testing.T) {
	rule := database.AlertRule{ID: 4, Name: "Métrica futura", Metric: "if_erros", Operator: ">", Threshold: 1, Severity: SeverityHigh}

	e := entradaDoAlerta(rule, "srv-2", nil, "rule:4:srv-2", "texto", SeverityHigh, "vps-teste", 2)

	if e.Unidade != "" {
		t.Errorf("Unidade = %q, esperada vazia para métrica fora do catálogo", e.Unidade)
	}
	if e.Metrica != "if_erros" {
		t.Errorf("Metrica = %q, esperado if_erros", e.Metrica)
	}
}
