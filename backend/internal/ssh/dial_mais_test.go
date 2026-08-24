package ssh

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestExpandHome(t *testing.T) {
	t.Setenv("HOME", "/home/teste")

	casos := map[string]string{
		"~/.ssh/id":    "/home/teste/.ssh/id",
		"/abs/caminho": "/abs/caminho",
		"relativo/x":   "relativo/x",
		"":             "",
	}
	for entrada, esperado := range casos {
		if got := expandHome(entrada); got != esperado {
			t.Errorf("expandHome(%q) = %q, esperado %q", entrada, got, esperado)
		}
	}
}

// ValidateHostKeyPolicy é o que faz a recusa aparecer no boot, não no primeiro
// dial. Os dois caminhos passam pelo sync.Once, por isso o reset entre casos.
func TestValidateHostKeyPolicyResolveNoBoot(t *testing.T) {
	t.Run("sem configuração recusa subir", func(t *testing.T) {
		usaPoliticaHostKey(t, "")
		if err := ValidateHostKeyPolicy(); err == nil {
			t.Fatal("sem SSH_KNOWN_HOSTS a política foi aceita")
		}
	})

	t.Run("com known_hosts válido sobe", func(t *testing.T) {
		usaPoliticaHostKey(t, knownHostsTemporario(t))
		if err := ValidateHostKeyPolicy(); err != nil {
			t.Fatalf("known_hosts válido recusado: %v", err)
		}
	})

	t.Run("resultado fica memoizado", func(t *testing.T) {
		usaPoliticaHostKey(t, knownHostsTemporario(t))
		if err := ValidateHostKeyPolicy(); err != nil {
			t.Fatalf("primeira resolução: %v", err)
		}
		// Trocar o ambiente depois da primeira resolução não muda a política:
		// ela vale para o processo inteiro.
		os.Setenv("SSH_KNOWN_HOSTS", "")
		if err := ValidateHostKeyPolicy(); err != nil {
			t.Fatalf("política deveria continuar a memoizada: %v", err)
		}
	})
}

func TestClientConfigLeChaveEHostKey(t *testing.T) {
	t.Run("chave ausente", func(t *testing.T) {
		usaPoliticaHostKey(t, knownHostsTemporario(t))
		alvo := Target{User: "root", KeyPath: filepath.Join(t.TempDir(), "nao-existe")}
		if _, err := clientConfig(alvo); err == nil || !strings.Contains(err.Error(), "erro ao ler a chave SSH") {
			t.Fatalf("esperado erro de leitura da chave, veio: %v", err)
		}
	})

	t.Run("chave corrompida", func(t *testing.T) {
		usaPoliticaHostKey(t, knownHostsTemporario(t))
		path := filepath.Join(t.TempDir(), "lixo")
		if err := os.WriteFile(path, []byte("isto não é uma chave"), 0o600); err != nil {
			t.Fatal(err)
		}
		alvo := Target{User: "root", KeyPath: path}
		if _, err := clientConfig(alvo); err == nil || !strings.Contains(err.Error(), "chave SSH inválida") {
			t.Fatalf("esperado erro de chave inválida, veio: %v", err)
		}
	})

	t.Run("política de host key inválida derruba a config", func(t *testing.T) {
		usaPoliticaHostKey(t, "")
		alvo := Target{User: "root", KeyPath: chaveClienteTemporaria(t)}
		if _, err := clientConfig(alvo); err == nil || !strings.Contains(err.Error(), "SSH_KNOWN_HOSTS") {
			t.Fatalf("esperado erro da política de host key, veio: %v", err)
		}
	})

	t.Run("config completa", func(t *testing.T) {
		usaPoliticaHostKey(t, knownHostsTemporario(t))
		alvo := Target{User: "monitor", KeyPath: chaveClienteTemporaria(t)}
		cfg, err := clientConfig(alvo)
		if err != nil {
			t.Fatalf("config válida recusada: %v", err)
		}
		if cfg.User != "monitor" {
			t.Errorf("user = %q", cfg.User)
		}
		if cfg.Timeout != dialTimeout {
			t.Errorf("timeout = %v, esperado %v", cfg.Timeout, dialTimeout)
		}
		if len(cfg.Auth) != 1 {
			t.Errorf("auth methods = %d, esperado 1", len(cfg.Auth))
		}
	})
}

// O ponto do S5: o dial só fecha com quem apresenta a chave registrada.
func TestDialVerificaHostKeyDoServidor(t *testing.T) {
	responder := func(string) respostaExec { return respostaExec{} }

	t.Run("chave registrada conecta", func(t *testing.T) {
		alvo := alvoComServidor(t, responder)
		client, err := dial(alvo)
		if err != nil {
			t.Fatalf("dial com host key certa falhou: %v", err)
		}
		client.Close()
	})

	t.Run("chave divergente é recusada", func(t *testing.T) {
		addr, _ := servidorSSHFake(t, responder)
		// known_hosts de outro host: a chave apresentada não casa com nada.
		usaPoliticaHostKey(t, knownHostsTemporario(t))

		host, porta, err := net.SplitHostPort(addr)
		if err != nil {
			t.Fatal(err)
		}
		p, err := strconv.Atoi(porta)
		if err != nil {
			t.Fatal(err)
		}
		alvo := Target{Host: host, Port: p, User: "root", KeyPath: chaveClienteTemporaria(t)}
		if _, err := dial(alvo); err == nil {
			t.Fatal("dial fechou com host key fora do known_hosts")
		}
	})
}

func TestOpenSessionPropagaFalhaDoDial(t *testing.T) {
	usaPoliticaHostKey(t, knownHostsTemporario(t))
	alvo := Target{Host: "127.0.0.1", Port: 1, User: "root", KeyPath: chaveClienteTemporaria(t)}
	if _, _, err := openSession(alvo); err == nil {
		t.Fatal("openSession fechou sem servidor do outro lado")
	}
}
