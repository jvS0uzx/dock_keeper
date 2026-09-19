package main

import (
	"strings"
	"testing"
)

func ambiente(valores map[string]string) func(string) string {
	return func(chave string) string { return valores[chave] }
}

func TestAgenteRecusaHTTPParaPainelRemoto(t *testing.T) {
	_, err := loadConfig(ambiente(map[string]string{"AGENT_SERVER_URL": "http://painel.exemplo.com"}))
	if err == nil {
		t.Fatal("http:// para painel remoto deveria ser recusado: a credencial do dispositivo iria em claro")
	}
	if !strings.Contains(err.Error(), "https") || !strings.Contains(err.Error(), "ALLOW_INSECURE_HTTP") {
		t.Errorf("mensagem = %q, esperada citando https e ALLOW_INSECURE_HTTP", err.Error())
	}
}

func TestAgenteAceitaHTTPNoLocalhost(t *testing.T) {
	for _, url := range []string{"http://127.0.0.1:8080", "http://localhost:8080", "http://[::1]:8080"} {
		if _, err := loadConfig(ambiente(map[string]string{"AGENT_SERVER_URL": url})); err != nil {
			t.Errorf("%s recusado: %v", url, err)
		}
	}
}

func TestAgenteAceitaHTTPComVariavelExplicita(t *testing.T) {
	cfg, err := loadConfig(ambiente(map[string]string{
		"AGENT_SERVER_URL":    "http://painel.exemplo.com",
		"ALLOW_INSECURE_HTTP": "true",
	}))
	if err != nil {
		t.Fatalf("com ALLOW_INSECURE_HTTP=true deveria passar: %v", err)
	}
	if !cfg.inseguro {
		t.Error("a configuração precisa marcar que está em modo inseguro, para o aviso no log")
	}
}

func TestAgenteAceitaHTTPS(t *testing.T) {
	if _, err := loadConfig(ambiente(map[string]string{"AGENT_SERVER_URL": "https://painel.exemplo.com"})); err != nil {
		t.Errorf("https recusado: %v", err)
	}
}
