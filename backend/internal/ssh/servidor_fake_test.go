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

type respostaExec struct {
	stdout       string
	stderr       string
	status       uint32
	consomeStdin bool
}

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
		ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{resp.status}))
		return
	}
}

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

func knownHostsPara(t *testing.T, addr string, pub ssh.PublicKey) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "known_hosts")
	linha := knownhosts.Line([]string{addr}, pub) + "\n"
	if err := os.WriteFile(path, []byte(linha), 0o600); err != nil {
		t.Fatalf("escrever known_hosts: %v", err)
	}
	return path
}

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
