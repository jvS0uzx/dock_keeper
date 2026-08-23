package ssh

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/joaov/vd_stats/internal/alert"
)

// Pausa antes de reabrir a sessão SSH. Sem ela um retorno sem erro vira loop
// apertado de reconexão.
const reconnectDelay = 5 * time.Second

type ServerManager struct {
	mu          sync.Mutex
	cancelFuncs map[string]context.CancelFunc
}

var Manager = &ServerManager{
	cancelFuncs: make(map[string]context.CancelFunc),
}

// Start liga os streams de coleta do alvo, se ainda não estiverem rodando.
func (m *ServerManager) Start(t Target) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.cancelFuncs[t.ID]; exists {
		log.Printf("[RealTime] stream de %s já está rodando", t.Host)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFuncs[t.ID] = cancel

	go supervise(ctx, "metricas", t, StartStream, func(err error) {
		alert.Notify("host_unreachable:"+t.ID,
			fmt.Sprintf("[CRITICO] VPS %s (%s) inalcançável: %v", t.Name, t.Host, err))
	})

	if t.CollectNginx {
		go supervise(ctx, "nginx", t, StartNginxStream, nil)
	}
}

// Stop derruba os streams do servidor e esquece o alvo.
func (m *ServerManager) Stop(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cancel, exists := m.cancelFuncs[id]; exists {
		cancel()
		delete(m.cancelFuncs, id)
	}
}

// StopAll derruba todos os streams — usado no encerramento do processo.
func (m *ServerManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, cancel := range m.cancelFuncs {
		cancel()
		delete(m.cancelFuncs, id)
	}
}

// supervise mantém um stream vivo: reabre a sessão sempre que ela cai, até o
// contexto ser cancelado. onError roda apenas quando a queda foi por erro.
func supervise(ctx context.Context, label string, t Target, run func(context.Context, Target) error, onError func(error)) {
	for {
		if err := run(ctx, t); err != nil && ctx.Err() == nil {
			log.Printf("[RealTime] stream %s de %s caiu: %v. Reconectando em %s...", label, t.Host, err, reconnectDelay)
			if onError != nil {
				onError(err)
			}
		}

		select {
		case <-ctx.Done():
			log.Printf("[RealTime] parando stream %s de %s", label, t.Host)
			return
		case <-time.After(reconnectDelay):
		}
	}
}
