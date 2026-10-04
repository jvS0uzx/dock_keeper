package api

import "testing"

func TestURLDeEscutaRespeitaOHostDoAPIAddr(t *testing.T) {
	casos := map[string]string{
		":8080":           "http://localhost:8080",
		"127.0.0.1:18080": "http://127.0.0.1:18080",
		"0.0.0.0:8080":    "http://localhost:8080",
		"[::]:8080":       "http://localhost:8080",
		"[::1]:9000":      "http://[::1]:9000",
		"painel.lan:80":   "http://painel.lan:80",
	}
	for addr, esperado := range casos {
		if obtido := urlDeEscuta(addr); obtido != esperado {
			t.Errorf("urlDeEscuta(%q) = %q, esperado %q", addr, obtido, esperado)
		}
	}
}
