package ssh

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNotifyStoppedContainersSoAlertaContainerParado(t *testing.T) {
	avisos := capturarAvisos(t)
	alvo := Target{ID: "zz-notify-teste-" + strconv.FormatInt(time.Now().UnixNano(), 10), Host: "203.0.113.7"}

	newVigiaDeContainers(alvo).observe([]DockerPSPayload{
		{Name: "web", State: "running"},
		{Name: "worker", State: "exited"},
		{Name: "sem-estado", State: ""},
	}, nil)

	got := avisos()
	if len(got) != 1 || !strings.Contains(got[0].texto, "worker") || !strings.Contains(got[0].texto, "exited") {
		t.Errorf("avisos = %+v, esperado só o do container worker parado", got)
	}
}
