package ssh

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	DefaultPort = 22
	dialTimeout = 10 * time.Second
)

// Target identifica um host monitorado e como abrir SSH nele. Substitui a
// sequência (id, name, host, user, keyPath) que era repetida em cada função.
type Target struct {
	ID      string
	Name    string
	Host    string
	User    string
	Port    int
	KeyPath string

	// CollectNginx liga o stream do access log do Nginx neste host.
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

// ValidateHostKeyPolicy resolve a política de host key no boot. Sem isso a
// recusa só apareceria no primeiro dial, minutos depois da partida e no meio
// do log de um coletor, em vez de na cara de quem subiu o serviço.
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

// resolveHostKeyCallback decide como o host key é verificado. Verificar deixou
// de ser opcional: o painel abre sessão SSH como root nos hosts, e aceitar
// qualquer chave entrega essa sessão a quem conseguir se pôr no caminho. Só
// desliga com SSH_INSECURE_HOST_KEY explícito, no mesmo espírito do API_TOKEN,
// que também recusa subir vazio.
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

// clientConfig monta a config SSH a partir da chave privada do processo.
func clientConfig(t Target) (*ssh.ClientConfig, error) {
	keyBytes, err := os.ReadFile(expandHome(t.KeyPath))
	if err != nil {
		return nil, fmt.Errorf("erro ao ler a chave SSH: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("chave SSH inválida: %w", err)
	}
	hostKey, err := hostKeyCallback()
	if err != nil {
		return nil, err
	}
	return &ssh.ClientConfig{
		User:            t.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKey,
		Timeout:         dialTimeout,
	}, nil
}

// dial abre a conexão SSH com o alvo. Ponto único de entrada — antes cada
// stream reimplementava leitura de chave, parse e Dial.
func dial(t Target) (*ssh.Client, error) {
	config, err := clientConfig(t)
	if err != nil {
		return nil, err
	}
	return ssh.Dial("tcp", t.addr(), config)
}

// openSession abre conexão e sessão no alvo. O chamador fecha as duas.
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

// stopOnCancel derruba sessão e conexão quando o contexto é cancelado. Sem
// isso o comando remoto (tail -f, docker logs -f) segue rodando na VPS depois
// que o operador fecha a tela.
func stopOnCancel(ctx context.Context, client *ssh.Client, session *ssh.Session) {
	go func() {
		<-ctx.Done()
		session.Close()
		client.Close()
	}()
}
