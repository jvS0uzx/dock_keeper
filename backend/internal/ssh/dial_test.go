package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// knownHostsTemporario escreve um known_hosts válido com uma chave descartável.
func knownHostsTemporario(t *testing.T) string {
	t.Helper()

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gerar chave: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("converter chave: %v", err)
	}

	path := filepath.Join(t.TempDir(), "known_hosts")
	linha := knownhosts.Line([]string{"10.0.0.1:22"}, sshPub) + "\n"
	if err := os.WriteFile(path, []byte(linha), 0o600); err != nil {
		t.Fatalf("escrever known_hosts: %v", err)
	}
	return path
}

func TestResolveHostKeyCallbackSemKnownHostsRecusa(t *testing.T) {
	cb, err := resolveHostKeyCallback("", false)
	if err == nil {
		t.Fatal("sem SSH_KNOWN_HOSTS o painel subiu aceitando qualquer host key")
	}
	if cb != nil {
		t.Error("callback devolvido junto com erro: o dial passaria mesmo assim")
	}

	// A mensagem tem de ensinar a sair do erro, não só apontá-lo.
	for _, trecho := range []string{"SSH_KNOWN_HOSTS", "ssh-keyscan", "SSH_INSECURE_HOST_KEY"} {
		if !strings.Contains(err.Error(), trecho) {
			t.Errorf("mensagem não cita %q: %v", trecho, err)
		}
	}
}

func TestResolveHostKeyCallbackInseguroExigeOptIn(t *testing.T) {
	cb, err := resolveHostKeyCallback("", true)
	if err != nil {
		t.Fatalf("com SSH_INSECURE_HOST_KEY=true devia subir: %v", err)
	}
	if cb == nil {
		t.Fatal("callback nulo com a desativação explícita")
	}
}

func TestResolveHostKeyCallbackComArquivo(t *testing.T) {
	cb, err := resolveHostKeyCallback(knownHostsTemporario(t), false)
	if err != nil {
		t.Fatalf("known_hosts válido recusado: %v", err)
	}
	if cb == nil {
		t.Fatal("callback nulo com known_hosts válido")
	}
}

func TestResolveHostKeyCallbackArquivoInexistente(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nao-existe")
	if _, err := resolveHostKeyCallback(path, false); err == nil {
		t.Fatal("known_hosts inexistente foi aceito")
	} else if !strings.Contains(err.Error(), "ssh-keyscan") {
		t.Errorf("mensagem não ensina a gerar o arquivo: %v", err)
	}
}
