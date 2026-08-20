package ssh

import (
	"context"
	"log"
	"sync"
	"time"
)

type ServerManager struct {
	mu          sync.Mutex
	cancelFuncs map[string]context.CancelFunc
}

var Manager = &ServerManager{
	cancelFuncs: make(map[string]context.CancelFunc),
}

func (m *ServerManager) Start(id, host, user, keyPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.cancelFuncs[id]; exists {
		log.Printf("Stream para %s já está rodando", host)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFuncs[id] = cancel

	go func() {
		// Goroutine para métricas gerais (Docker/CPU)
		go func() {
			for {
				select {
				case <-ctx.Done():
					log.Printf("[RealTime] Parando stream SSH para a VPS %s...", host)
					return
				default:
					err := StartStream(id, host, user, keyPath)
					if err != nil {
						log.Printf("Erro na VPS %s: %v. Tentando reconectar em 5s...", host, err)
						time.Sleep(5 * time.Second)
					}
				}
			}
		}()

		// Goroutine para o Nginx Access Log
		go func() {
			for {
				select {
				case <-ctx.Done():
					log.Printf("[RealTime] Parando stream NGINX para a VPS %s...", host)
					return
				default:
					err := StartNginxStream(id, host, user, keyPath)
					if err != nil {
						log.Printf("Erro no NGINX Stream %s: %v. Tentando reconectar em 5s...", host, err)
						time.Sleep(5 * time.Second)
					}
				}
			}
		}()
	}()
}

func (m *ServerManager) Stop(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cancel, exists := m.cancelFuncs[id]; exists {
		cancel()
		delete(m.cancelFuncs, id)
	}
}
