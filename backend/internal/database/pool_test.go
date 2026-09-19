package database

import (
	"github.com/jvS0uzx/dock_keeper/internal/config"
	"testing"
	"time"
)

func TestEnvIntUsaOPadraoQuandoOValorNaoServe(t *testing.T) {
	casos := []struct {
		nome     string
		valor    string
		definido bool
		esperado int
	}{
		{"ausente", "", false, defaultMaxOpenConns},
		{"vazio", "   ", true, defaultMaxOpenConns},
		{"nao numerico", "muitas", true, defaultMaxOpenConns},
		{"zero desligaria o teto", "0", true, defaultMaxOpenConns},
		{"negativo", "-5", true, defaultMaxOpenConns},
		{"valido", "42", true, 42},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if c.definido {
				t.Setenv("DB_MAX_OPEN_CONNS", c.valor)
			}
			if got := config.Inteiro("DB_MAX_OPEN_CONNS", defaultMaxOpenConns); got != c.esperado {
				t.Errorf("config.Inteiro(%q) = %d, esperado %d", c.valor, got, c.esperado)
			}
		})
	}
}

func TestEnvDurationUsaOPadraoQuandoOValorNaoServe(t *testing.T) {
	casos := []struct {
		nome     string
		valor    string
		definido bool
		esperado time.Duration
	}{
		{"ausente", "", false, defaultConnMaxLifetime},
		{"nao e duracao", "30", true, defaultConnMaxLifetime},
		{"zero seria vida infinita", "0s", true, defaultConnMaxLifetime},
		{"negativo", "-1m", true, defaultConnMaxLifetime},
		{"valido", "90s", true, 90 * time.Second},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if c.definido {
				t.Setenv("DB_CONN_MAX_LIFETIME", c.valor)
			}
			if got := config.Duracao("DB_CONN_MAX_LIFETIME", defaultConnMaxLifetime); got != c.esperado {
				t.Errorf("config.Duracao(%q) = %s, esperado %s", c.valor, got, c.esperado)
			}
		})
	}
}

func TestPadraoDoPoolCabeNumaInstalacaoLimpa(t *testing.T) {
	if defaultMaxOpenConns*2 >= 100 {
		t.Errorf("duas réplicas com %d conexões cada não cabem em max_connections=100", defaultMaxOpenConns)
	}
	if defaultMaxIdleConns > defaultMaxOpenConns {
		t.Errorf("ociosas (%d) acima do teto de abertas (%d)", defaultMaxIdleConns, defaultMaxOpenConns)
	}
}
