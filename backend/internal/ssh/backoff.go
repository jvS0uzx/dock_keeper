package ssh

import (
	"math/rand/v2"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	reconnectBase       = 5 * time.Second
	defaultReconnectMax = 5 * time.Minute
	healthyRun          = 60 * time.Second
)

type backoff struct {
	max     time.Duration
	current time.Duration
	jitter  func(time.Duration) time.Duration
}

func newBackoff(max time.Duration, jitter func(time.Duration) time.Duration) *backoff {
	return &backoff{max: max, jitter: jitter}
}

func (b *backoff) next(ranFor time.Duration) time.Duration {
	switch {
	case ranFor >= healthyRun || b.current == 0:
		b.current = reconnectBase
	default:
		b.current = min(b.current*2, b.max)
	}
	return b.jitter(b.current)
}

func jitter(d time.Duration) time.Duration {
	return time.Duration(float64(d) * (0.8 + 0.4*rand.Float64()))
}

func reconnectMax() time.Duration {
	return database.EnvDuration("SSH_RECONNECT_MAX", defaultReconnectMax)
}
