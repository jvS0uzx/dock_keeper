package ssh

import (
	"fmt"
	"strings"
)

var allowedActions = map[string]bool{
	"start":   true,
	"stop":    true,
	"restart": true,
}

func IsAllowedAction(action string) bool {
	return allowedActions[action]
}

func RunContainerAction(t Target, action, containerName string) (string, error) {
	if !allowedActions[action] {
		return "", fmt.Errorf("ação inválida: %q", action)
	}
	if !validContainerName.MatchString(containerName) {
		return "", fmt.Errorf("nome de container inválido: %q", containerName)
	}

	client, session, err := openSession(t)
	if err != nil {
		return "", err
	}
	defer client.Close()
	defer session.Close()

	out, err := session.CombinedOutput(fmt.Sprintf("docker %s -- %s", action, containerName))
	if err != nil {
		return string(out), fmt.Errorf("docker %s falhou: %w", action, err)
	}
	return strings.TrimSpace(string(out)), nil
}
