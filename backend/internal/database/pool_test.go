package database

import (
	"testing"
	"time"
)

// O achado E1: sem teto configurado o database/sql abre conexão sem limite e as
// goroutines de coleta somadas aos handlers estouram o max_connections do
// Postgres. O teto precisa vir do ambiente, e valor inválido não pode desligar
// o limite em silêncio.
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
			if got := envInt("DB_MAX_OPEN_CONNS", defaultMaxOpenConns); got != c.esperado {
				t.Errorf("envInt(%q) = %d, esperado %d", c.valor, got, c.esperado)
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
			if got := envDuration("DB_CONN_MAX_LIFETIME", defaultConnMaxLifetime); got != c.esperado {
				t.Errorf("envDuration(%q) = %s, esperado %s", c.valor, got, c.esperado)
			}
		})
	}
}

// O padrão precisa caber no max_connections=100 de uma instalação limpa, com
// folga para uma segunda réplica do painel.
func TestPadraoDoPoolCabeNumaInstalacaoLimpa(t *testing.T) {
	if defaultMaxOpenConns*2 >= 100 {
		t.Errorf("duas réplicas com %d conexões cada não cabem em max_connections=100", defaultMaxOpenConns)
	}
	if defaultMaxIdleConns > defaultMaxOpenConns {
		t.Errorf("ociosas (%d) acima do teto de abertas (%d)", defaultMaxIdleConns, defaultMaxOpenConns)
	}
}
