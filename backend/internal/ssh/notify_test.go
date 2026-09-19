package ssh

import (
	"bytes"
	"log"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func TestNotifyStoppedContainersSoAlertaContainerParado(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	alvo := Target{ID: "zz-notify-teste-" + strconv.FormatInt(time.Now().UnixNano(), 10), Host: "203.0.113.7"}
	t.Cleanup(func() {
		if database.DB != nil {
			database.DB.Where("key LIKE ?", "container_down:"+alvo.ID+":%").Delete(&database.AlertState{})
		}
	})
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
