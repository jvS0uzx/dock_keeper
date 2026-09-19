package ssh

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/jvS0uzx/dock_keeper/internal/config"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	defaultRTTInterval        = 30 * time.Second
	defaultKeepaliveMaxMisses = 3
	defaultKeepaliveTimeout   = 3 * time.Second
)

var keepaliveTimeoutNs atomic.Int64

func init() {
	keepaliveTimeoutNs.Store(int64(defaultKeepaliveTimeout))
}

func keepaliveTimeout() time.Duration {
	return time.Duration(keepaliveTimeoutNs.Load())
}

var rttCache = struct {
	mu   sync.Mutex
	last map[string]*float64
}{last: map[string]*float64{}}

func keepaliveRTT(client *ssh.Client, timeout time.Duration) *float64 {
	start := time.Now()
	done := make(chan error, 1)
	go func() {
		_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			return nil
		}
		ms := float64(time.Since(start).Microseconds()) / 1000
		return &ms
	case <-time.After(timeout):
		return nil
	}
}

func runKeepalive(ctx context.Context, t Target, client *ssh.Client, interval, timeout time.Duration, maxMisses int, record bool) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	misses := 0
	for {
		rtt := keepaliveRTT(client, timeout)
		if ctx.Err() != nil {
			return
		}
		if record {
			setRTT(t.ID, rtt)
		}
		if rtt != nil {
			misses = 0
		} else if misses++; misses >= maxMisses {
			log.Printf("[RealTime] %s não respondeu a %d keepalives seguidos: conexão tratada como morta, fechando para reconectar",
				t.Host, misses)
			client.Close()
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func latestRTT(id string) *float64 {
	rttCache.mu.Lock()
	defer rttCache.mu.Unlock()
	return rttCache.last[id]
}

func setRTT(id string, v *float64) {
	rttCache.mu.Lock()
	defer rttCache.mu.Unlock()
	rttCache.last[id] = v
}

func forgetRTT(id string) {
	rttCache.mu.Lock()
	defer rttCache.mu.Unlock()
	delete(rttCache.last, id)
}

func rttProbeEnabled() bool {
	raw := strings.TrimSpace(os.Getenv("RTT_PROBE"))
	if raw == "" {
		return true
	}
	on, err := strconv.ParseBool(raw)
	if err != nil {
		log.Printf("[SSH] RTT_PROBE=%q inválido, gravação do RTT continua ligada", raw)
		return true
	}
	return on
}

func rttInterval() time.Duration {
	return database.EnvDuration("RTT_PROBE_INTERVAL", defaultRTTInterval)
}

func keepaliveMaxMisses() int {
	return config.Inteiro("SSH_KEEPALIVE_MAX_MISSES", defaultKeepaliveMaxMisses)
}
