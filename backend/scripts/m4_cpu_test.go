package scripts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func escreverStat(t *testing.T, caminho, conteudo string) {
	t.Helper()
	if err := os.WriteFile(caminho, []byte(conteudo), 0o644); err != nil {
		t.Fatalf("escrever %s: %v", caminho, err)
	}
}

func TestSemDeltaDeCPUNaoSaiAmostra(t *testing.T) {
	stat := filepath.Join(t.TempDir(), "stat")
	escreverStat(t, stat, "cpu  100 0 100 1000 0 0 0 0 0 0\n")

	saida := iniciarScript(t, "DOCKKEEPER_INTERVAL=1\nDOCKKEEPER_PROC_STAT="+stat+"\n")

	select {
	case linha, ok := <-saida.linhas:
		if ok {
			t.Fatalf("o script emitiu amostra sem delta de CPU: %s", linha)
		}
		t.Fatal("o script terminou sozinho")
	case <-time.After(3 * time.Second):
	}

	escreverStat(t, stat, "cpu  200 0 200 1400 0 0 0 0 0 0\n")

	var payload struct {
		HostCPU *float64 `json:"host_cpu"`
	}
	linha := saida.proxima()
	if err := json.Unmarshal([]byte(linha), &payload); err != nil {
		t.Fatalf("linha inválida: %v\n%s", err, linha)
	}
	if payload.HostCPU == nil {
		t.Fatal("a amostra saiu sem host_cpu")
	}
	if *payload.HostCPU < 33.0 || *payload.HostCPU > 33.6 {
		t.Errorf("host_cpu = %.1f, esperado ~33.3 (delta de 600 jiffies com 400 ociosos)", *payload.HostCPU)
	}
}
