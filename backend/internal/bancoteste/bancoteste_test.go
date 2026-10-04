package bancoteste

import (
	"fmt"
	"testing"
)

func TestTrocarBancoCobreOsDoisFormatos(t *testing.T) {
	casos := []struct {
		nome     string
		dsn      string
		esperado string
	}{
		{"url", "postgres://u:s@localhost:5433/dockkeeper?sslmode=disable", "postgres://u:s@localhost:5433/alvo?sslmode=disable"},
		{"chave-valor", "host=localhost user=u password=s dbname=dockkeeper port=5433 sslmode=disable", "host=localhost user=u password=s dbname=alvo port=5433 sslmode=disable"},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			if got := TrocarBanco(caso.dsn, "alvo"); got != caso.esperado {
				t.Errorf("TrocarBanco = %q, esperado %q", got, caso.esperado)
			}
		})
	}
}

type registro struct {
	testing.TB
	fatal, pulou string
}

func (r *registro) Helper() {}

func (r *registro) Fatalf(formato string, args ...any) { r.fatal = fmt.Sprintf(formato, args...) }

func (r *registro) Skipf(formato string, args ...any) { r.pulou = fmt.Sprintf(formato, args...) }

func TestPularViraFalhaQuandoOBancoEExigido(t *testing.T) {
	casos := []struct {
		valor        string
		esperaFalhar bool
	}{
		{"", false},
		{"0", false},
		{"1", true},
	}
	for _, caso := range casos {
		t.Run("valor="+caso.valor, func(t *testing.T) {
			t.Setenv(VariavelExige, caso.valor)
			r := &registro{}
			Pular(r, "banco indisponível: %v", "recusado")
			if caso.esperaFalhar && (r.fatal == "" || r.pulou != "") {
				t.Errorf("com %s=%q o teste pulou (%q) em vez de falhar", VariavelExige, caso.valor, r.pulou)
			}
			if !caso.esperaFalhar && (r.pulou != "banco indisponível: recusado" || r.fatal != "") {
				t.Errorf("com %s=%q esperado pular, falhou com %q", VariavelExige, caso.valor, r.fatal)
			}
		})
	}
}
