package ssh

import (
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/jvS0uzx/dock_keeper/internal/observabilidade"
)

const defaultMaxSessionsPerHost = 6

var ErrSessionLimit = errors.New("limite de sessões SSH simultâneas atingido para este servidor")

var sessions = struct {
	mu    sync.Mutex
	inUse map[string]int
}{inUse: map[string]int{}}

func AcquireSession(t Target) (func(), error) {
	key := t.ID
	if key == "" {
		key = t.addr()
	}
	limit := maxSessionsPerHost()

	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	if sessions.inUse[key] >= limit {
		return nil, ErrSessionLimit
	}
	sessions.inUse[key]++
	observabilidade.SessoesSSH.Add(1)

	var once sync.Once
	return func() {
		once.Do(func() {
			sessions.mu.Lock()
			defer sessions.mu.Unlock()
			observabilidade.SessoesSSH.Add(-1)
			if sessions.inUse[key]--; sessions.inUse[key] <= 0 {
				delete(sessions.inUse, key)
			}
		})
	}, nil
}

func maxSessionsPerHost() int {
	raw := strings.TrimSpace(os.Getenv("SSH_MAX_SESSIONS_PER_HOST"))
	if raw == "" {
		return defaultMaxSessionsPerHost
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		log.Printf("[SSH] SSH_MAX_SESSIONS_PER_HOST=%q inválido, usando %d", raw, defaultMaxSessionsPerHost)
		return defaultMaxSessionsPerHost
	}
	return n
}
