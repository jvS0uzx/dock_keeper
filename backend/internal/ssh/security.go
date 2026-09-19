package ssh

import (
	"bufio"
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/jvS0uzx/dock_keeper/internal/logstore"
)

const authLogTailLines = 20

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

	if err := session.Start(authLogCommand(t)); err != nil {
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

func GetRadarPorts(t Target) ([]PortInfo, error) {
	client, session, err := openSession(t)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	defer session.Close()

	out, err := session.Output(radarCommand(t))
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

func parseSSLine(line string) (PortInfo, bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return PortInfo{}, false
	}

	localAddr := fields[4]
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

func processName(field string) (string, bool) {
	start := strings.Index(field, `("`)
	end := strings.Index(field, `",`)
	if start == -1 || end == -1 || start+2 >= end {
		return "", false
	}
	return field[start+2 : end], true
}

func authLogCommand(t Target) string {
	cmd := "tail -n " + strconv.Itoa(authLogTailLines) + " -f " + AuthLogPath()
	if useSudo(t) {
		return sudoPrefix + "/usr/bin/" + cmd
	}
	return cmd
}

func radarCommand(t Target) string {
	ss := "ss -tulnp"
	if useSudo(t) {
		ss = sudoPrefix + "/usr/bin/" + ss
	}
	return ss + " | grep LISTEN"
}
