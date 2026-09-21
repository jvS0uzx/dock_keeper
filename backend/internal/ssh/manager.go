package ssh

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/observabilidade"
	"github.com/jvS0uzx/dock_keeper/internal/safego"
)

type ServerManager struct {
	mu          sync.Mutex
	cancelFuncs map[string]context.CancelFunc
}

var Manager = &ServerManager{
	cancelFuncs: make(map[string]context.CancelFunc),
}

func (m *ServerManager) Start(t Target) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.cancelFuncs[t.ID]; exists {
		log.Printf("[RealTime] stream de %s já está rodando", t.Host)
		return false
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFuncs[t.ID] = cancel

	safego.Run(ctx, "ssh:metricas:"+t.Host, func(ctx context.Context) {
		supervise(ctx, "metricas", t, StartStream, func(err error) {
			notifyAlert(alertaDe(t, "host_unreachable:"+t.ID, "critical",
				fmt.Sprintf("[CRITICO] VPS %s (%s) inalcançável: %v", t.Name, t.Host, err), alvoDoHost(t)))
		})
	})

	safego.Run(ctx, "ssh:nginx:sonda:"+t.Host, func(ctx context.Context) {
		SondarNginxPeriodicamente(ctx, t)
	})
	safego.Run(ctx, "ssh:nginx:"+t.Host, func(ctx context.Context) {
		superviseNginx(ctx, t)
	})
	if authWatchEnabled() {
		safego.Run(ctx, "ssh:authlog:"+t.Host, func(ctx context.Context) {
			supervise(ctx, "authlog", t, StartAuthWatch, nil)
		})
	}
	return true
}

func (m *ServerManager) Stop(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cancel, exists := m.cancelFuncs[id]; exists {
		cancel()
		delete(m.cancelFuncs, id)
	}
	forgetRTT(id)
}

func (m *ServerManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, cancel := range m.cancelFuncs {
		cancel()
		delete(m.cancelFuncs, id)
		forgetRTT(id)
	}
}

func superviseNginx(ctx context.Context, t Target) {
	wait := newBackoff(reconnectMax(), jitter)
	onError := nginxDownAlert(t)

	for {
		if !coletaDeNginxLiberada(t) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(intervaloDoPortao):
			}
			continue
		}

		started := time.Now()
		err := StartNginxStream(ctx, t)
		delay := wait.next(time.Since(started))
		if err != nil && ctx.Err() == nil {
			observabilidade.ReconexoesSSH.Add(1)
			log.Printf("[RealTime] stream nginx de %s caiu: %v. Reconectando em %s...", t.Host, err, delay.Round(time.Second))
			onError(err)
		}

		select {
		case <-ctx.Done():
			log.Printf("[RealTime] parando stream nginx de %s", t.Host)
			return
		case <-time.After(delay):
		}
	}
}

func supervise(ctx context.Context, label string, t Target, run func(context.Context, Target) error, onError func(error)) {
	wait := newBackoff(reconnectMax(), jitter)
	for {
		started := time.Now()
		err := run(ctx, t)
		delay := wait.next(time.Since(started))
		if err != nil && ctx.Err() == nil {
			observabilidade.ReconexoesSSH.Add(1)
			log.Printf("[RealTime] stream %s de %s caiu: %v. Reconectando em %s...", label, t.Host, err, delay.Round(time.Second))
			if onError != nil {
				onError(err)
			}
		}

		select {
		case <-ctx.Done():
			log.Printf("[RealTime] parando stream %s de %s", label, t.Host)
			return
		case <-time.After(delay):
		}
	}
}
