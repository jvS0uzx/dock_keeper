package ssh

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	DefaultPort = 22
	dialTimeout = 10 * time.Second
)

type Target struct {
	ID      string
	Name    string
	Host    string
	User    string
	Port    int
	KeyPath string
	SiteID  *uint

	CollectNginx bool
}

func (t Target) addr() string {
	port := t.Port
	if port <= 0 {
		port = DefaultPort
	}
	return t.Host + ":" + strconv.Itoa(port)
}

var (
	hostKeyOnce sync.Once
	hostKeyCB   ssh.HostKeyCallback
	hostKeyErr  error
)

func ValidateHostKeyPolicy() error {
	_, err := hostKeyCallback()
	return err
}

func hostKeyCallback() (ssh.HostKeyCallback, error) {
	hostKeyOnce.Do(func() {
		insecure, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("SSH_INSECURE_HOST_KEY")))
		hostKeyCB, hostKeyErr = resolveHostKeyCallback(os.Getenv("SSH_KNOWN_HOSTS"), insecure)
	})
	return hostKeyCB, hostKeyErr
}

func resolveHostKeyCallback(knownHosts string, insecure bool) (ssh.HostKeyCallback, error) {
	if insecure {
		log.Println("[SSH] ATENÇÃO: SSH_INSECURE_HOST_KEY=true — host key não verificado. " +
			"A sessão SSH como root fica exposta a máquina no meio. Use apenas em laboratório.")
		return ssh.InsecureIgnoreHostKey(), nil
	}

	path := expandHome(strings.TrimSpace(knownHosts))
	if path == "" {
		return nil, errors.New("SSH_KNOWN_HOSTS não definido: o painel abre sessão SSH como root e " +
			"recusa subir sem verificar o host key. Popule um arquivo com " +
			"`ssh-keyscan -H host1 host2 >> ~/.ssh/known_hosts` e aponte SSH_KNOWN_HOSTS para ele. " +
			"Para desligar a verificação de propósito, defina SSH_INSECURE_HOST_KEY=true")
	}

	cb, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler SSH_KNOWN_HOSTS (%s): %w. "+
			"Gere o arquivo com `ssh-keyscan -H <host> >> %s`", path, err, path)
	}
	log.Printf("[SSH] verificando host keys contra %s", path)
	return cb, nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~") {
		return filepath.Join(os.Getenv("HOME"), strings.TrimPrefix(path, "~"))
	}
	return path
}

func clientConfig(t Target) (*ssh.ClientConfig, func(), error) {
	auth, release, err := authMethods(t)
	if err != nil {
		return nil, nil, err
	}
	hostKey, err := hostKeyCallback()
	if err != nil {
		release()
		return nil, nil, err
	}
	return &ssh.ClientConfig{
		User:            t.User,
		Auth:            auth,
		HostKeyCallback: hostKey,
		Timeout:         dialTimeout,
	}, release, nil
}

func authMethods(t Target) ([]ssh.AuthMethod, func(), error) {
	withAgent := useAgent()
	var methods []ssh.AuthMethod

	if t.KeyPath != "" || !withAgent {
		signer, err := loadKey(t.KeyPath)
		if err != nil {
			return nil, nil, err
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if !withAgent {
		return methods, func() {}, nil
	}

	sock := strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK"))
	if sock == "" {
		return nil, nil, errors.New("SSH_USE_AGENT=true, mas SSH_AUTH_SOCK não está definido: " +
			"rode o painel com um ssh-agent carregado")
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, nil, fmt.Errorf("erro ao falar com o ssh-agent em SSH_AUTH_SOCK: %w", err)
	}
	methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
	return methods, func() { conn.Close() }, nil
}

func loadKey(path string) (ssh.Signer, error) {
	keyBytes, err := os.ReadFile(expandHome(path))
	if err != nil {
		return nil, fmt.Errorf("erro ao ler a chave SSH: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(keyBytes)
	var missing *ssh.PassphraseMissingError
	if !errors.As(err, &missing) {
		if err != nil {
			return nil, fmt.Errorf("chave SSH inválida: %w", err)
		}
		return signer, nil
	}

	passphrase := os.Getenv("SSH_KEY_PASSPHRASE")
	if passphrase == "" {
		return nil, errors.New("chave protegida por passphrase: defina SSH_KEY_PASSPHRASE ou use SSH_USE_AGENT=true")
	}
	signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
	if err != nil {
		return nil, fmt.Errorf("passphrase da chave SSH recusada: %w", err)
	}
	return signer, nil
}

func useAgent() bool {
	on, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("SSH_USE_AGENT")))
	return on
}

func dial(t Target) (*ssh.Client, error) {
	config, release, err := clientConfig(t)
	if err != nil {
		return nil, err
	}
	defer release()
	return ssh.Dial("tcp", t.addr(), config)
}

func openSession(t Target) (*ssh.Client, *ssh.Session, error) {
	client, err := dial(t)
	if err != nil {
		return nil, nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, nil, err
	}
	return client, session, nil
}

func stopOnCancel(ctx context.Context, client *ssh.Client, session *ssh.Session) {
	go func() {
		<-ctx.Done()
		session.Close()
		client.Close()
	}()
}
