package database

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPruneBatchedRepeteAteOLoteVirIncompleto(t *testing.T) {
	respostas := []int64{pruneBatchSize, pruneBatchSize, 137}
	var chamadas int
	var limites []any

	exec := func(_ context.Context, sql string, args ...any) (int64, error) {
		if !strings.Contains(sql, "LIMIT") {
			t.Fatalf("DELETE sem LIMIT trava a tabela inteira: %s", sql)
		}
		limites = append(limites, args[len(args)-1])
		n := respostas[chamadas]
		chamadas++
		return n, nil
	}

	total, err := pruneBatched(context.Background(), exec, "metric_servers", "timestamp", time.Now(), 0)
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

func TestPruneBatchedParaNoPrimeiroLoteIncompleto(t *testing.T) {
	var chamadas int
	exec := func(context.Context, string, ...any) (int64, error) {
		chamadas++
		return pruneBatchSize - 1, nil
	}

	if _, err := pruneBatched(context.Background(), exec, "metric_containers", "timestamp", time.Now(), 0); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if chamadas != 1 {
		t.Errorf("lotes executados = %d, esperado 1", chamadas)
	}
}

func TestPruneBatchedEsvaziaOVencidoAlemDeUmMilhaoDeLinhas(t *testing.T) {
	const lotesCheios = 450
	var chamadas int
	exec := func(context.Context, string, ...any) (int64, error) {
		chamadas++
		if chamadas <= lotesCheios {
			return pruneBatchSize, nil
		}
		return 42, nil
	}

	total, err := pruneBatched(context.Background(), exec, "metric_servers", "timestamp", time.Now(), 0)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if want := int64(lotesCheios)*pruneBatchSize + 42; total != want {
		t.Errorf("linhas apagadas = %d, esperado %d: a poda parou antes de esvaziar o vencido", total, want)
	}
	if chamadas != lotesCheios+1 {
		t.Errorf("lotes executados = %d, esperado %d", chamadas, lotesCheios+1)
	}
}

func TestPruneBatchedParaQuandoOContextoCancela(t *testing.T) {
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()

	var chamadas int
	exec := func(context.Context, string, ...any) (int64, error) {
		chamadas++
		if chamadas == 3 {
			cancelar()
		}
		return pruneBatchSize, nil
	}

	total, err := pruneBatched(ctx, exec, "metric_servers", "timestamp", time.Now(), 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("erro devolvido = %v, esperado context.Canceled", err)
	}
	if chamadas != 3 || total != 3*pruneBatchSize {
		t.Errorf("lotes = %d, total = %d: a poda seguiu depois do desligamento", chamadas, total)
	}
}

func TestPruneBatchedNaoDormeAPausaInteiraNoDesligamento(t *testing.T) {
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()

	exec := func(context.Context, string, ...any) (int64, error) {
		return pruneBatchSize, nil
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancelar()
	}()

	inicio := time.Now()
	_, err := pruneBatched(ctx, exec, "metric_servers", "timestamp", time.Now(), time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("erro devolvido = %v, esperado context.Canceled", err)
	}
	if gasto := time.Since(inicio); gasto > 5*time.Second {
		t.Errorf("a poda levou %s para notar o desligamento durante a pausa", gasto)
	}
}

func TestPruneBatchedDevolveOParcialNoErro(t *testing.T) {
	falha := errors.New("conexão perdida")
	var chamadas int
	exec := func(context.Context, string, ...any) (int64, error) {
		chamadas++
		if chamadas == 2 {
			return 0, falha
		}
		return pruneBatchSize, nil
	}

	total, err := pruneBatched(context.Background(), exec, "metric_servers", "timestamp", time.Now(), 0)
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
