package ssh

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
)

// Sem TELEGRAM_BOT_TOKEN o alerta vira linha de log — que é exatamente o canal
// observável para este teste: parado alerta, rodando fica em silêncio.
func TestNotifyStoppedContainersSoAlertaContainerParado(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	alvo := Target{ID: "zz-notify-teste", Host: "203.0.113.7"}
	notifyStoppedContainers(alvo, []DockerPSPayload{
		{Name: "web", State: "running"},
		{Name: "worker", State: "exited"},
		{Name: "sem-estado", State: ""},
	})

	saida := buf.String()
	if !strings.Contains(saida, "worker") || !strings.Contains(saida, "exited") {
		t.Errorf("container parado não gerou alerta: %q", saida)
	}
	if strings.Contains(saida, "Container web") {
		t.Errorf("container rodando gerou alerta: %q", saida)
	}
}
