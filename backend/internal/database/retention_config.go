package database

import (
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/config"
)

const (
	DefaultMetricRetentionDays = 7
	DefaultLogRetentionDays    = 7

	DefaultTrendRetentionDays = 400

	DefaultHostRetentionDays = 30
)

func RetentionDays(key string, def int) time.Duration {
	return config.Dias(key, def)
}

func EnvDuration(key string, def time.Duration) time.Duration {
	return config.Duracao(key, def)
}
