package database

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// noSleep substitui a pausa entre lotes para o teste não esperar de verdade.
func noSleep(time.Duration) {}

// O achado E6: o DELETE sem LIMIT trava a tabela inteira numa tacada. A poda
// precisa sair em lotes até esgotar.
func TestPruneBatchedRepeteAteOLoteVirIncompleto(t *testing.T) {
	respostas := []int64{pruneBatchSize, pruneBatchSize, 137}
	var chamadas int
	var limites []any

	exec := func(sql string, args ...any) (int64, error) {
		if !strings.Contains(sql, "LIMIT") {
			t.Fatalf("DELETE sem LIMIT trava a tabela inteira: %s", sql)
		}
		limites = append(limites, args[len(args)-1])
		n := respostas[chamadas]
		chamadas++
		return n, nil
	}

	total, err := pruneBatched(exec, "metric_servers", "timestamp", time.Now(), noSleep)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if chamadas != len(respostas) {
		t.Errorf("lotes executados = %d, esperado %d", chamadas, len(respostas))
	}
	if want := int64(2*pruneBatchSize + 137); total != want {
		t.Errorf("linhas apagadas = %d, esperado %d", total, want)
	}
	for i, l := range limites {
		if l != pruneBatchSize {
			t.Errorf("lote %d usou LIMIT %v, esperado %d", i, l, pruneBatchSize)
		}
	}
}

// Lote incompleto significa que acabou o que havia para apagar; insistir seria
// uma varredura a mais por tabela a cada ciclo.
func TestPruneBatchedParaNoPrimeiroLoteIncompleto(t *testing.T) {
	var chamadas int
	exec := func(string, ...any) (int64, error) {
		chamadas++
		return pruneBatchSize - 1, nil
	}

	if _, err := pruneBatched(exec, "metric_containers", "timestamp", time.Now(), noSleep); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if chamadas != 1 {
		t.Errorf("lotes executados = %d, esperado 1", chamadas)
	}
}

// Uma base antiga não pode prender a rotina de retenção por horas na primeira
// passada: o teto devolve o controle e o restante sai no ciclo seguinte.
func TestPruneBatchedRespeitaOTetoDeLotes(t *testing.T) {
	var chamadas int
	exec := func(string, ...any) (int64, error) {
		chamadas++
		return pruneBatchSize, nil // sempre cheio: nunca esgota sozinho
	}

	total, err := pruneBatched(exec, "metric_servers", "timestamp", time.Now(), noSleep)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if chamadas != pruneMaxBatches {
		t.Errorf("lotes executados = %d, esperado o teto de %d", chamadas, pruneMaxBatches)
	}
	if want := int64(pruneMaxBatches) * pruneBatchSize; total != want {
		t.Errorf("linhas apagadas = %d, esperado %d", total, want)
	}
}

// Erro no meio do laço devolve o que já foi apagado: o número entra no log e
// não pode virar zero só porque o último lote falhou.
func TestPruneBatchedDevolveOParcialNoErro(t *testing.T) {
	falha := errors.New("conexão perdida")
	var chamadas int
	exec := func(string, ...any) (int64, error) {
		chamadas++
		if chamadas == 2 {
			return 0, falha
		}
		return pruneBatchSize, nil
	}

	total, err := pruneBatched(exec, "metric_servers", "timestamp", time.Now(), noSleep)
	if !errors.Is(err, falha) {
		t.Fatalf("erro devolvido = %v, esperado %v", err, falha)
	}
	if total != pruneBatchSize {
		t.Errorf("parcial devolvido = %d, esperado %d", total, pruneBatchSize)
	}
}

func TestPruneBatchSQLUsaATabelaPedida(t *testing.T) {
	sql := pruneBatchSQL("metric_load_balancers", "timestamp")
	if !strings.Contains(sql, "DELETE FROM metric_load_balancers ") {
		t.Errorf("SQL não apaga da tabela pedida: %s", sql)
	}
	if strings.Count(sql, "metric_load_balancers") != 2 {
		t.Errorf("subselect deveria ler da mesma tabela: %s", sql)
	}
}
