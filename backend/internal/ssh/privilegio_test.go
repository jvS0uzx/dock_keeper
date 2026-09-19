package ssh

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/jvS0uzx/dock_keeper/scripts"
)

func linhasDoSudoers(t *testing.T) map[string]string {
	t.Helper()

	conteudo, err := os.ReadFile(filepath.Join("..", "..", "deploy", "sudoers-dockkeeper-monitor.exemplo"))
	if err != nil {
		t.Fatalf("ler sudoers de exemplo: %v", err)
	}
	linhas := map[string]string{}
	for _, l := range strings.Split(string(conteudo), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "#") {
			continue
		}
		_, cmd, ok := strings.Cut(l, "NOPASSWD: ")
		if !ok {
			continue
		}
		switch {
		case strings.Contains(cmd, "auth.log") && strings.Contains(cmd, "-n 0 -F"):
			linhas["authwatch"] = cmd
		case strings.Contains(cmd, "auth.log"):
			linhas["auth"] = cmd
		case strings.Contains(cmd, "nginx"):
			linhas["nginx"] = cmd
		case strings.Contains(cmd, "/ss "):
			linhas["ss"] = cmd
		}
	}
	if len(linhas) != 4 {
		t.Fatalf("esperadas 4 linhas ativas no sudoers de exemplo, achei %v", linhas)
	}
	return linhas
}

func TestComandosComSudoBatemComOSudoers(t *testing.T) {
	linhas := linhasDoSudoers(t)
	t.Setenv("SSH_USE_SUDO", "true")
	t.Setenv("SSH_AUTH_LOG_PATH", "")
	alvo := Target{User: "dockkeeper-monitor"}

	if got, want := authLogCommand(alvo), "sudo -n "+linhas["auth"]; got != want {
		t.Errorf("auth.log com sudo:\n got %q\nwant %q", got, want)
	}
	if got, want := authWatchCommand(alvo), "sudo -n "+linhas["authwatch"]; got != want {
		t.Errorf("vigia do auth.log com sudo:\n got %q\nwant %q", got, want)
	}
	if got, want := radarCommand(alvo), "sudo -n "+linhas["ss"]+" | grep LISTEN"; got != want {
		t.Errorf("radar com sudo:\n got %q\nwant %q", got, want)
	}
}

func TestComandosSemSudoFicamIdenticosAosDeHoje(t *testing.T) {
	t.Setenv("SSH_AUTH_LOG_PATH", "")
	casos := []struct {
		sudo string
		user string
	}{
		{"", "dockkeeper-monitor"},
		{"false", "dockkeeper-monitor"},
		{"true", "root"},
		{"true", ""},
	}
	for _, c := range casos {
		t.Setenv("SSH_USE_SUDO", c.sudo)
		alvo := Target{User: c.user}
		if got := authLogCommand(alvo); got != "tail -n 20 -f /var/log/auth.log" {
			t.Errorf("SSH_USE_SUDO=%q user=%q: auth.log virou %q", c.sudo, c.user, got)
		}
		if got := authWatchCommand(alvo); got != "tail -n 0 -F /var/log/auth.log" {
			t.Errorf("SSH_USE_SUDO=%q user=%q: vigia virou %q", c.sudo, c.user, got)
		}
		if got := radarCommand(alvo); got != "ss -tulnp | grep LISTEN" {
			t.Errorf("SSH_USE_SUDO=%q user=%q: radar virou %q", c.sudo, c.user, got)
		}
	}
}

func executarScriptNginx(t *testing.T, alvo Target) (sudo string, tail string) {
	t.Helper()

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash indisponível")
	}
	dir := t.TempDir()
	saidaSudo := filepath.Join(dir, "sudo.args")
	saidaTail := filepath.Join(dir, "tail.args")
	falsos := map[string]string{
		"sudo": "#!/bin/sh\nprintf '%s' \"$*\" > \"" + saidaSudo + "\"\n",
		"tail": "#!/bin/sh\nprintf '%s' \"$*\" > \"" + saidaTail + "\"\n",
	}
	for nome, corpo := range falsos {
		if err := os.WriteFile(filepath.Join(dir, nome), []byte(corpo), 0o755); err != nil {
			t.Fatalf("criar %s falso: %v", nome, err)
		}
	}

	cmd := exec.Command(bash, "-s")
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Stdin = strings.NewReader(scriptPrelude(alvo) + scripts.StreamNginx)
	feito := make(chan error, 1)
	go func() { feito <- cmd.Run() }()
	select {
	case err := <-feito:
		if err != nil {
			t.Fatalf("script do nginx: %v", err)
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("script do nginx não terminou em 10 s")
	}

	b, _ := os.ReadFile(saidaSudo)
	c, _ := os.ReadFile(saidaTail)
	return string(b), string(c)
}

func TestNginxComSudoExecutaALinhaDoSudoers(t *testing.T) {
	linhas := linhasDoSudoers(t)
	t.Setenv("SSH_NGINX_LOG_PATH", "")

	t.Setenv("SSH_USE_SUDO", "true")
	sudo, tail := executarScriptNginx(t, Target{User: "dockkeeper-monitor"})
	if sudo != "-n "+linhas["nginx"] {
		t.Errorf("sudo recebeu %q, esperado %q", sudo, "-n "+linhas["nginx"])
	}
	if tail != "" {
		t.Errorf("com sudo o tail do PATH não deveria rodar, recebeu %q", tail)
	}

	t.Setenv("SSH_USE_SUDO", "")
	sudo, tail = executarScriptNginx(t, Target{User: "dockkeeper-monitor"})
	if sudo != "" {
		t.Errorf("sem SSH_USE_SUDO o sudo rodou com %q", sudo)
	}
	if tail != "-n 0 -F /var/log/nginx/access.log" {
		t.Errorf("sem sudo o tail recebeu %q", tail)
	}
}

func servidorExigindoChave(t *testing.T, aceita ssh.PublicKey) Target {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("chave de host: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer de host: %v", err)
	}
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(k.Marshal(), aceita.Marshal()) {
				return nil, nil
			}
			return nil, errors.New("chave desconhecida")
		},
	}
	cfg.AddHostKey(hostSigner)

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
			go atendeConexao(conn, cfg, func(string) respostaExec { return respostaExec{} })
		}
	}()

	usaPoliticaHostKey(t, knownHostsPara(t, ln.Addr().String(), hostSigner.PublicKey()))
	host, porta, _ := net.SplitHostPort(ln.Addr().String())
	p, _ := strconv.Atoi(porta)
	return Target{Host: host, Port: p, User: "dockkeeper-monitor"}
}

func chaveComPassphrase(t *testing.T, passphrase string) (string, ssh.PublicKey) {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gerar chave: %v", err)
	}
	bloco, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "teste", []byte(passphrase))
	if err != nil {
		t.Fatalf("serializar chave: %v", err)
	}
	path := filepath.Join(t.TempDir(), "id_protegida")
	if err := os.WriteFile(path, pem.EncodeToMemory(bloco), 0o600); err != nil {
		t.Fatalf("escrever chave: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	return path, signer.PublicKey()
}

func TestChaveComPassphrase(t *testing.T) {
	path, pub := chaveComPassphrase(t, "segredo-da-frota")
	alvo := servidorExigindoChave(t, pub)
	alvo.KeyPath = path
	t.Setenv("SSH_USE_AGENT", "")

	t.Setenv("SSH_KEY_PASSPHRASE", "")
	if _, err := dial(alvo); err == nil || !strings.Contains(err.Error(), "chave protegida por passphrase") {
		t.Errorf("sem SSH_KEY_PASSPHRASE: erro %v, esperado mencionar \"chave protegida por passphrase\"", err)
	}

	t.Setenv("SSH_KEY_PASSPHRASE", "errada")
	if _, err := dial(alvo); err == nil {
		t.Error("passphrase errada conectou")
	}

	t.Setenv("SSH_KEY_PASSPHRASE", "segredo-da-frota")
	client, err := dial(alvo)
	if err != nil {
		t.Fatalf("passphrase certa não conectou: %v", err)
	}
	client.Close()
}

func agenteEmMemoria(t *testing.T) (string, ssh.PublicKey) {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gerar chave do agente: %v", err)
	}
	chaveiro := agent.NewKeyring()
	if err := chaveiro.Add(agent.AddedKey{PrivateKey: priv}); err != nil {
		t.Fatalf("carregar chave no agente: %v", err)
	}

	dir, err := os.MkdirTemp("", "dkag")
	if err != nil {
		t.Fatalf("diretório do socket: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("socket do agente: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = agent.ServeAgent(chaveiro, conn)
			}()
		}
	}()

	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer do agente: %v", err)
	}
	return sock, signer.PublicKey()
}

func TestAutenticaPeloSSHAgent(t *testing.T) {
	sock, pub := agenteEmMemoria(t)
	alvo := servidorExigindoChave(t, pub)
	t.Setenv("SSH_AUTH_SOCK", sock)
	t.Setenv("SSH_KEY_PASSPHRASE", "")

	t.Setenv("SSH_USE_AGENT", "")
	if _, err := dial(alvo); err == nil {
		t.Error("sem SSH_USE_AGENT e sem SSH_KEY_PATH o dial não deveria autenticar")
	}

	t.Setenv("SSH_USE_AGENT", "true")
	client, err := dial(alvo)
	if err != nil {
		t.Fatalf("SSH_USE_AGENT=true com a chave no agente não conectou: %v", err)
	}
	client.Close()
}

func TestSSHAgentSemSocketExplica(t *testing.T) {
	t.Setenv("SSH_USE_AGENT", "true")
	t.Setenv("SSH_AUTH_SOCK", "")
	usaPoliticaHostKey(t, knownHostsTemporario(t))

	if _, _, err := clientConfig(Target{User: "dockkeeper-monitor"}); err == nil || !strings.Contains(err.Error(), "SSH_AUTH_SOCK") {
		t.Errorf("SSH_USE_AGENT=true sem SSH_AUTH_SOCK: erro %v, esperado mencionar SSH_AUTH_SOCK", err)
	}
}
