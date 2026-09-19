package ssh

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/safego"
)

func TestPanicoNoSuperviseNaoDerrubaOProcesso(t *testing.T) {
	var chamadas atomic.Int32
	reiniciou := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	alvo := Target{ID: "srv-panico", Name: "servidor de teste", Host: "127.0.0.1"}
	coleta := func(ctx context.Context, _ Target) error {
		switch chamadas.Add(1) {
		case 1:
			panic("coleta em pânico")
		case 2:
			close(reiniciou)
		}
		<-ctx.Done()
		return nil
	}

	safego.Run(ctx, "ssh:teste", func(ctx context.Context) {
		supervise(ctx, "teste", alvo, coleta, nil)
	})

	select {
	case <-reiniciou:
	case <-time.After(5 * time.Second):
		t.Fatalf("o stream não voltou depois do pânico: %d execução(ões)", chamadas.Load())
	}
}
