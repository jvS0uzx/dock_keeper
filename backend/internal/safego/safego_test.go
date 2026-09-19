package safego

import (
	"bytes"
	"context"
	"log"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type escritorSeguro struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (e *escritorSeguro) Write(p []byte) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.buf.Write(p)
}

func (e *escritorSeguro) texto() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.buf.String()
}

func pausasCurtas(t *testing.T) {
	t.Helper()

	base, teto, bom := pausaBase, pausaMax, execucaoSaudavel
	pausaBase, pausaMax, execucaoSaudavel = 5*time.Millisecond, 20*time.Millisecond, time.Hour
	t.Cleanup(func() { pausaBase, pausaMax, execucaoSaudavel = base, teto, bom })
}

func esperar(t *testing.T, sinal <-chan struct{}, prazo time.Duration, motivo string) {
	t.Helper()

	select {
	case <-sinal:
	case <-time.After(prazo):
		t.Fatal(motivo)
	}
}

func TestRunReiniciaDepoisDePanico(t *testing.T) {
	pausasCurtas(t)

	var chamadas atomic.Int32
	pronto := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	Run(ctx, "teste:panico", func(context.Context) {
		if chamadas.Add(1) <= 2 {
			panic("falha proposital")
		}
		close(pronto)
	})

	esperar(t, pronto, 2*time.Second, "a rotina não foi reiniciada depois do pânico")
	if n := chamadas.Load(); n != 3 {
		t.Errorf("execuções = %d, esperado 3 (dois pânicos e uma execução boa)", n)
	}
}

func TestRunRegistraNomeEPilhaDoPanico(t *testing.T) {
	pausasCurtas(t)

	saida := &escritorSeguro{}
	original := log.Writer()
	log.SetOutput(saida)
	t.Cleanup(func() { log.SetOutput(original) })

	pronto := make(chan struct{})
	var chamadas atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	Run(ctx, "teste:pilha", func(context.Context) {
		if chamadas.Add(1) == 1 {
			panic("explodiu aqui")
		}
		close(pronto)
	})

	esperar(t, pronto, 2*time.Second, "a rotina não foi reiniciada")

	texto := saida.texto()
	for _, trecho := range []string{"teste:pilha", "explodiu aqui", "safego"} {
		if !strings.Contains(texto, trecho) {
			t.Errorf("o log do pânico não contém %q: %s", trecho, texto)
		}
	}
	if !strings.Contains(texto, "goroutine") {
		t.Error("o log do pânico não traz a pilha")
	}
}

func TestRunParaNoCancelamentoSemVazarGoroutine(t *testing.T) {
	pausasCurtas(t)

	base := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())

	fim := Run(ctx, "teste:cancelamento", func(ctx context.Context) { <-ctx.Done() })

	cancel()
	esperar(t, fim, 2*time.Second, "a rotina não respeitou o cancelamento")

	prazo := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > base && time.Now().Before(prazo) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > base {
		t.Errorf("goroutines = %d, esperado no máximo %d", n, base)
	}
}

func TestRunNaoReiniciaFuncaoQueRetornaNormalmente(t *testing.T) {
	pausasCurtas(t)

	var chamadas atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	Run(ctx, "teste:retorno", func(context.Context) { chamadas.Add(1) })

	time.Sleep(200 * time.Millisecond)
	if n := chamadas.Load(); n != 1 {
		t.Errorf("execuções = %d, esperado 1: retorno normal não é reinício", n)
	}
}
