package ssh

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func streamsAtivos(m *ServerManager) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.cancelFuncs)
}

func TestManagerNaoDuplicaNemVazaStreams(t *testing.T) {
	m := &ServerManager{cancelFuncs: make(map[string]context.CancelFunc)}
	// Chave inexistente: o supervise falha na hora, sem tocar a rede, e fica
	// aguardando a reconexão — o ciclo de vida do manager não depende do dial.
	alvo := Target{ID: "s1", Name: "t1", Host: "127.0.0.1", Port: 1, User: "root",
		KeyPath: filepath.Join(t.TempDir(), "inexistente")}

	m.Start(alvo)
	m.Start(alvo) // repetido: não pode abrir segundo stream
	if got := streamsAtivos(m); got != 1 {
		t.Fatalf("streams ativos = %d, esperado 1", got)
	}

	alvo2 := alvo
	alvo2.ID = "s2"
	m.Start(alvo2)
	if got := streamsAtivos(m); got != 2 {
		t.Fatalf("streams ativos = %d, esperado 2", got)
	}

	m.Stop("s1")
	m.Stop("s1") // idempotente
	if got := streamsAtivos(m); got != 1 {
		t.Fatalf("após Stop, streams = %d, esperado 1", got)
	}

	m.StopAll()
	if got := streamsAtivos(m); got != 0 {
		t.Fatalf("após StopAll, streams = %d, esperado 0", got)
	}
}

func TestSuperviseAcionaOnErrorEEncerraNoCancelamento(t *testing.T) {
	t.Run("erro com contexto vivo notifica", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var execucoes atomic.Int32
		run := func(context.Context, Target) error {
			execucoes.Add(1)
			return errors.New("sessão caiu")
		}
		// onError cancela: é como o teste escapa do delay de reconexão de 5s.
		onError := func(error) { cancel() }

		done := make(chan struct{})
		go func() {
			supervise(ctx, "teste", Target{}, run, onError)
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("supervise não encerrou após o cancelamento")
		}
		if got := execucoes.Load(); got != 1 {
			t.Errorf("execuções = %d, esperado 1", got)
		}
	})

	t.Run("queda por cancelamento não notifica", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		var notificado atomic.Bool
		run := func(context.Context, Target) error {
			// O operador removeu o servidor: a sessão devolve erro, mas o
			// contexto já morreu — alerta aqui seria ruído de desligamento.
			cancel()
			return errors.New("sessão fechada no desligamento")
		}
		onError := func(error) { notificado.Store(true) }

		done := make(chan struct{})
		go func() {
			supervise(ctx, "teste", Target{}, run, onError)
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("supervise não encerrou após o cancelamento")
		}
		if notificado.Load() {
			t.Error("queda causada pelo cancelamento gerou notificação")
		}
	})
}
