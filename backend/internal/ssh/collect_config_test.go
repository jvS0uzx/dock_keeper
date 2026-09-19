package ssh

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestCaminhoRemotoRecusaMetacaractere(t *testing.T) {
	casos := []struct {
		nome   string
		valor  string
		aceito bool
	}{
		{"caminho absoluto comum", "/var/log/secure", true},
		{"com hífen e ponto", "/var/log/nginx/access.log-1", true},
		{"relativo", "var/log/secure", false},
		{"ponto-e-vírgula", "/var/log/x;id", false},
		{"substituição de comando", "/var/log/$(id)", false},
		{"crase", "/var/log/`id`", false},
		{"pipe", "/var/log/a|b", false},
		{"espaço", "/var/log/meu log", false},
		{"aspas", `/var/log/"x"`, false},
		{"redirecionamento", "/var/log/x>y", false},
		{"e comercial", "/var/log/x&", false},
		{"vazio cai no padrão", "", true},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			t.Setenv("SSH_AUTH_LOG_PATH", c.valor)
			got := AuthLogPath()

			if c.aceito && c.valor != "" && got != c.valor {
				t.Errorf("caminho legítimo %q foi recusado, virou %q", c.valor, got)
			}
			if !c.aceito && got != defaultAuthLogPath {
				t.Errorf("caminho perigoso %q foi aceito: %q", c.valor, got)
			}
		})
	}
}

func TestIntervaloInvalidoCaiNoPadrao(t *testing.T) {
	casos := []struct {
		valor string
		quer  int
	}{
		{"", defaultCollectInterval},
		{"0", defaultCollectInterval},
		{"-1", defaultCollectInterval},
		{"abc", defaultCollectInterval},
		{"5", 5},
		{" 10 ", 10},
	}

	for _, c := range casos {
		t.Run("valor "+c.valor, func(t *testing.T) {
			t.Setenv("SSH_COLLECT_INTERVAL", c.valor)
			if got := CollectIntervalSec(); got != c.quer {
				t.Errorf("SSH_COLLECT_INTERVAL=%q deu %d, esperado %d", c.valor, got, c.quer)
			}
		})
	}
}

type sessaoFalsa struct {
	enviado *strings.Builder
	fechado chan struct{}
	comando string
}

func novaSessaoFalsa() *sessaoFalsa {
	return &sessaoFalsa{enviado: &strings.Builder{}, fechado: make(chan struct{})}
}

type escritorFalso struct {
	destino *strings.Builder
	fechado chan struct{}
}

func (e escritorFalso) Write(p []byte) (int, error) { return e.destino.Write(p) }
func (e escritorFalso) Close() error                { close(e.fechado); return nil }

func (s *sessaoFalsa) StdinPipe() (io.WriteCloser, error) {
	return escritorFalso{destino: s.enviado, fechado: s.fechado}, nil
}

func (s *sessaoFalsa) Start(cmd string) error {
	s.comando = cmd
	return nil
}

func (s *sessaoFalsa) esperarEnvio(t *testing.T) string {
	t.Helper()
	select {
	case <-s.fechado:
		return s.enviado.String()
	case <-time.After(2 * time.Second):
		t.Fatal("a goroutine de escrita não terminou")
		return ""
	}
}

func TestPreludioVaiAntesDoScript(t *testing.T) {
	t.Setenv("SSH_COLLECT_INTERVAL", "7")
	t.Setenv("SSH_NGINX_LOG_PATH", "/var/log/nginx/outro.log")

	s := novaSessaoFalsa()
	if err := runScript(s, Target{}, "#!/bin/bash\necho corpo-do-script\n"); err != nil {
		t.Fatalf("runScript: %v", err)
	}

	enviado := s.esperarEnvio(t)
	posPrelude := strings.Index(enviado, "DOCKKEEPER_INTERVAL=7")
	posCorpo := strings.Index(enviado, "corpo-do-script")

	if posPrelude < 0 {
		t.Fatalf("o intervalo não foi injetado; enviado:\n%s", enviado)
	}
	if posCorpo < 0 {
		t.Fatalf("o corpo do script não foi enviado")
	}
	if posPrelude > posCorpo {
		t.Error("o prelúdio saiu depois do script; as variáveis nunca seriam lidas")
	}
	if !strings.Contains(enviado, "DOCKKEEPER_NGINX_LOG=/var/log/nginx/outro.log") {
		t.Errorf("o caminho do access log não foi injetado; enviado:\n%s", enviado)
	}
	if s.comando != "bash -s" {
		t.Errorf("comando = %q, esperado bash -s", s.comando)
	}
}
