package ssh

import (
	"fmt"
	"strings"
)

// Critério de auditoria do pacote: registra-se o comando remoto que MUDA ESTADO
// no host por decisão de uma pessoa. Hoje isso é exatamente RunContainerAction,
// e o registro fica no handler, que é quem conhece o ator.
//
// Ficam deliberadamente fora, e a ausência de linha aqui não é esquecimento:
//
//   - StartStream e StartNginxStream (client.go) rodam em laço pelo supervisor,
//     sem pessoa por trás. Auditá-los afogaria o sinal em ruído de máquina,
//     pelo mesmo motivo que a rota de ingestão não é auditada.
//   - StreamDockerLogs (client.go), StreamAuthLogs e GetRadarPorts (security.go)
//     nascem de requisição, mas apenas leem: nenhum altera o host.

var allowedActions = map[string]bool{
	"start":   true,
	"stop":    true,
	"restart": true,
}

// IsAllowedAction expõe a allowlist para o chamador HTTP.
//
// O nome da ação vira coluna na linha de auditoria, gravada antes da execução.
// Validar aqui, contra o mesmo mapa que RunContainerAction consulta, evita que
// o corpo da requisição escolha o que fica escrito na tabela — e evita a cópia
// da lista no pacote HTTP, que divergiria no primeiro verbo novo.
func IsAllowedAction(action string) bool {
	return allowedActions[action]
}

// RunContainerAction executa docker start/stop/restart num container remoto.
// Valida ação (whitelist) e nome (regex) antes de montar o comando — os dois
// vêm do frontend e rodariam como root na VPS.
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
