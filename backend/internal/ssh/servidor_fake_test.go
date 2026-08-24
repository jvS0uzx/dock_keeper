package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Infra de teste: um servidor SSH real em loopback. É o que permite cobrir
// dial, openSession e os streams sem VPS nenhuma — o cliente do pacote conversa
// com um servidor de verdade, só que respondendo o que o teste mandar.

// respostaExec descreve como o servidor responde a um comando remoto.
type respostaExec struct {
	stdout string
	stderr string
	status uint32
	// consomeStdin drena o stdin da sessão — os streams de coleta sobem o
	// script por `bash -s` e bloqueariam sem um leitor do outro lado.
	consomeStdin bool
}

// servidorSSHFake sobe o servidor e devolve endereço e chave pública de host.
// responder recebe o comando pedido pelo cliente e decide a resposta.
func servidorSSHFake(t *testing.T, responder func(cmd string) respostaExec) (string, ssh.PublicKey) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gerar chave de host: %v", err)
	}
	_ = pub
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer de host: %v", err)
	}

	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go atendeConexao(conn, cfg, responder)
		}
	}()

	return ln.Addr().String(), signer.PublicKey()
}

func atendeConexao(conn net.Conn, cfg *ssh.ServerConfig, responder func(string) respostaExec) {
	sconn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sconn.Close()
	go ssh.DiscardRequests(reqs)

	for novo := range chans {
		if novo.ChannelType() != "session" {
			novo.Reject(ssh.UnknownChannelType, "apenas session")
			continue
		}
		ch, sessReqs, err := novo.Accept()
		if err != nil {
			continue
		}
		go atendeSessao(ch, sessReqs, responder)
	}
}

func atendeSessao(ch ssh.Channel, reqs <-chan *ssh.Request, responder func(string) respostaExec) {
	defer ch.Close()
	for req := range reqs {
		if req.Type != "exec" {
			req.Reply(false, nil)
			continue
		}
		var payload struct{ Command string }
		ssh.Unmarshal(req.Payload, &payload)
		req.Reply(true, nil)

		resp := responder(payload.Command)
		if resp.consomeStdin {
			go io.Copy(io.Discard, ch)
		}
		if resp.stdout != "" {
			io.WriteString(ch, resp.stdout)
		}
		if resp.stderr != "" {
			io.WriteString(ch.Stderr(), resp.stderr)
		}
		// Sem exit-status o session.Wait() do cliente devolve ExitMissingError,
		// e todo stream terminaria em erro mesmo com a saída certa.
		ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{resp.status}))
		return
	}
}

// chaveClienteTemporaria grava uma chave privada OpenSSH descartável.
func chaveClienteTemporaria(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gerar chave de cliente: %v", err)
	}
	bloco, err := ssh.MarshalPrivateKey(priv, "teste")
	if err != nil {
		t.Fatalf("serializar chave: %v", err)
	}
	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, pem.EncodeToMemory(bloco), 0o600); err != nil {
		t.Fatalf("escrever chave: %v", err)
	}
	return path
}

// knownHostsPara grava um known_hosts contendo a chave do endereço dado.
func knownHostsPara(t *testing.T, addr string, pub ssh.PublicKey) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "known_hosts")
	linha := knownhosts.Line([]string{addr}, pub) + "\n"
	if err := os.WriteFile(path, []byte(linha), 0o600); err != nil {
		t.Fatalf("escrever known_hosts: %v", err)
	}
	return path
}

// usaPoliticaHostKey aponta a política global de host key (memoizada por
// sync.Once) para o known_hosts dado e devolve tudo ao estado virgem no fim —
// sem o reset, o primeiro teste que dialasse congelaria a política para o
// binário inteiro.
func usaPoliticaHostKey(t *testing.T, knownHosts string) {
	t.Helper()
	t.Setenv("SSH_KNOWN_HOSTS", knownHosts)
	t.Setenv("SSH_INSECURE_HOST_KEY", "")
	zeraPoliticaHostKey()
	t.Cleanup(zeraPoliticaHostKey)
}

func zeraPoliticaHostKey() {
	hostKeyOnce = sync.Once{}
	hostKeyCB = nil
	hostKeyErr = nil
}

// alvoComServidor sobe servidor fake, chave de cliente e known_hosts casando, e
// devolve o Target pronto para dial.
func alvoComServidor(t *testing.T, responder func(cmd string) respostaExec) Target {
	t.Helper()
	addr, hostPub := servidorSSHFake(t, responder)
	usaPoliticaHostKey(t, knownHostsPara(t, addr, hostPub))

	host, porta, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("endereço do servidor fake: %v", err)
	}
	p, err := strconv.Atoi(porta)
	if err != nil {
		t.Fatalf("porta do servidor fake: %v", err)
	}
	return Target{
		ID:      "srv-teste",
		Name:    "fake",
		Host:    host,
		User:    "root",
		Port:    p,
		KeyPath: chaveClienteTemporaria(t),
	}
}
