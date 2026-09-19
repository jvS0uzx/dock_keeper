package safego

import (
	"context"
	"log"
	"runtime/debug"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/observabilidade"
)

var (
	pausaBase        = time.Second
	pausaMax         = time.Minute
	execucaoSaudavel = time.Minute
)

func Run(ctx context.Context, nome string, fn func(context.Context)) <-chan struct{} {
	fim := make(chan struct{})
	go func() {
		defer close(fim)
		supervisionar(ctx, nome, fn)
	}()
	return fim
}

func supervisionar(ctx context.Context, nome string, fn func(context.Context)) {
	pausa := pausaBase
	for {
		inicio := time.Now()
		if !executar(ctx, nome, fn) {
			return
		}
		if time.Since(inicio) >= execucaoSaudavel {
			pausa = pausaBase
		}

		select {
		case <-ctx.Done():
			log.Printf("[safego] rotina %q encerrada depois do pânico: contexto cancelado", nome)
			return
		case <-time.After(pausa):
		}
		pausa = min(pausa*2, pausaMax)
	}
}

func executar(ctx context.Context, nome string, fn func(context.Context)) (panicou bool) {
	defer func() {
		if r := recover(); r != nil {
			panicou = true
			observabilidade.PanicosRecuperados.Add(1)
			log.Printf("[safego] rotina %q entrou em pânico e será reiniciada: %v\n%s", nome, r, debug.Stack())
		}
	}()

	fn(ctx)
	return false
}
