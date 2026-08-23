package ssh

import (
	"bufio"
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/joaov/vd_stats/internal/logstore"
)

// Quantas linhas de histórico o auth.log entrega antes de passar a acompanhar.
const authLogTailLines = 20

// StreamAuthLogs acompanha o log de autenticação do host e repassa cada linha
// por SSE, persistindo no histórico de logs.
//
// O caminho é configurável (SSH_AUTH_LOG_PATH) porque o padrão de Debian
// não vale em RHEL, onde o arquivo é /var/log/secure — e o caminho cravado
// deixava a tela de Segurança vazia sem nenhum erro aparecer.
func StreamAuthLogs(ctx context.Context, t Target, w http.ResponseWriter, flusher http.Flusher) error {
	client, session, err := openSession(t)
	if err != nil {
		return err
	}
	defer client.Close()
	defer session.Close()

	stopOnCancel(ctx, client, session)

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	if err := session.Start("tail -n " + strconv.Itoa(authLogTailLines) + " -f " + AuthLogPath()); err != nil {
		return err
	}

	stream := newSSEWriter(w, flusher)
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		logstore.Save(t.ID, "auth", "", line)
		stream.send(line)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	return session.Wait()
}

type PortInfo struct {
	Protocol string `json:"protocol"`
	State    string `json:"state"`
	Port     string `json:"port"`
	Process  string `json:"process"`
}

// GetRadarPorts lista as portas em LISTEN do host, com o processo dono.
func GetRadarPorts(t Target) ([]PortInfo, error) {
	client, session, err := openSession(t)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	defer session.Close()

	out, err := session.Output("ss -tulnp | grep LISTEN")
	if err != nil {
		return nil, err
	}

	var ports []PortInfo
	for _, line := range strings.Split(string(out), "\n") {
		if info, ok := parseSSLine(line); ok {
			ports = append(ports, info)
		}
	}
	return ports, nil
}

// parseSSLine extrai uma porta de uma linha do `ss -tulnp`, no formato
// "tcp LISTEN 0 128 0.0.0.0:22 0.0.0.0:* users:((\"sshd\",pid=1,fd=3))".
func parseSSLine(line string) (PortInfo, bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return PortInfo{}, false
	}

	localAddr := fields[4] // 0.0.0.0:22 ou *:80
	port := "unknown"
	if idx := strings.LastIndex(localAddr, ":"); idx != -1 {
		port = localAddr[idx+1:]
	}

	process := "System/Unknown"
	if len(fields) >= 7 {
		if name, ok := processName(fields[6]); ok {
			process = name
		}
	}

	return PortInfo{Protocol: fields[0], State: fields[1], Port: port, Process: process}, true
}

// processName tira "sshd" de `users:(("sshd",pid=123,fd=3))`.
func processName(field string) (string, bool) {
	start := strings.Index(field, `("`)
	end := strings.Index(field, `",`)
	if start == -1 || end == -1 || start+2 >= end {
		return "", false
	}
	return field[start+2 : end], true
}
