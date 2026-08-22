package ssh

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const dialTimeout = 10 * time.Second

var allowedActions = map[string]bool{
	"start":   true,
	"stop":    true,
	"restart": true,
}

// RunContainerAction executa docker start/stop/restart num container remoto.
// Valida ação (whitelist) e nome (regex) antes de montar o comando — os dois
// vêm do frontend e rodariam como root na VPS.
func RunContainerAction(host, user, keyPath, action, containerName string) (string, error) {
	if !allowedActions[action] {
		return "", fmt.Errorf("ação inválida: %q", action)
	}
	if !validContainerName.MatchString(containerName) {
		return "", fmt.Errorf("nome de container inválido: %q", containerName)
	}

	client, err := dial(host, user, keyPath)
	if err != nil {
		return "", err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	out, err := session.CombinedOutput(fmt.Sprintf("docker %s %s", action, containerName))
	if err != nil {
		return string(out), fmt.Errorf("docker %s falhou: %w", action, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// dial centraliza a abertura de sessão SSH usada pelas ações pontuais.
func dial(host, user, keyPath string) (*ssh.Client, error) {
	keyPath = strings.Replace(keyPath, "~", os.Getenv("HOME"), 1)
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         dialTimeout,
	}
	return ssh.Dial("tcp", fmt.Sprintf("%s:22", host), config)
}
