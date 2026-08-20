package ssh

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

func StreamAuthLogs(ctx context.Context, host, user, keyPath string, w http.ResponseWriter, flusher http.Flusher) error {
	keyPath = strings.Replace(keyPath, "~", os.Getenv("HOME"), 1)
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil { return err }

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil { return err }

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout: 10 * time.Second,
	}

	hostPort := fmt.Sprintf("%s:22", host)
	client, err := ssh.Dial("tcp", hostPort, config)
	if err != nil { return err }
	defer client.Close()

	session, err := client.NewSession()
	if err != nil { return err }

	go func() {
		<-ctx.Done()
		session.Close()
		client.Close()
	}()
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil { return err }

	// Lê as últimas 20 linhas e acompanha
	err = session.Start("tail -n 20 -f /var/log/auth.log")
	if err != nil { return err }

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Fprintf(w, "data: %s\n\n", line)
		flusher.Flush()
	}

	return session.Wait()
}

type PortInfo struct {
	Protocol string `json:"protocol"`
	State    string `json:"state"`
	Port     string `json:"port"`
	Process  string `json:"process"`
}

func GetRadarPorts(host, user, keyPath string) ([]PortInfo, error) {
	keyPath = strings.Replace(keyPath, "~", os.Getenv("HOME"), 1)
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil { return nil, err }

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil { return nil, err }

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout: 10 * time.Second,
	}

	hostPort := fmt.Sprintf("%s:22", host)
	client, err := ssh.Dial("tcp", hostPort, config)
	if err != nil { return nil, err }
	defer client.Close()

	session, err := client.NewSession()
	if err != nil { return nil, err }
	defer session.Close()

	out, err := session.Output("ss -tulnp | grep LISTEN")
	if err != nil { return nil, err }

	var ports []PortInfo
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" { continue }
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			// tcp LISTEN 0 128 0.0.0.0:22 ...
			protocol := fields[0]
			state := fields[1]
			
			// extrai a porta
			localAddr := fields[4] // 0.0.0.0:22 ou *:80
			portIdx := strings.LastIndex(localAddr, ":")
			port := "unknown"
			if portIdx != -1 {
				port = localAddr[portIdx+1:]
			}

			// Process (ex: users:(("sshd",pid=123,fd=3)))
			process := "System/Unknown"
			if len(fields) >= 7 {
				procField := fields[6]
				start := strings.Index(procField, `("`)
				end := strings.Index(procField, `",`)
				if start != -1 && end != -1 && start+2 < end {
					process = procField[start+2 : end]
				}
			}

			ports = append(ports, PortInfo{
				Protocol: protocol,
				State:    state,
				Port:     port,
				Process:  process,
			})
		}
	}
	return ports, nil
}
