package ssh

import (
	"strings"
	"sync"
	"testing"
)

func TestRunContainerActionMontaComandoComSeparador(t *testing.T) {
	var mu sync.Mutex
	var comando string
	alvo := alvoComServidor(t, func(cmd string) respostaExec {
		mu.Lock()
		comando = cmd
		mu.Unlock()
		return respostaExec{stdout: "web\n"}
	})

	out, err := RunContainerAction(alvo, "restart", "web")
	if err != nil {
		t.Fatalf("ação válida falhou: %v", err)
	}
	if out != "web" {
		t.Errorf("saída = %q, esperado o eco do docker sem espaços", out)
	}

	mu.Lock()
	defer mu.Unlock()
	if comando != "docker restart -- web" {
		t.Errorf("comando remoto = %q, esperado %q", comando, "docker restart -- web")
	}
}

func TestRunContainerActionPropagaFalhaRemota(t *testing.T) {
	alvo := alvoComServidor(t, func(string) respostaExec {
		return respostaExec{stdout: "Error response from daemon\n", status: 1}
	})

	out, err := RunContainerAction(alvo, "stop", "web")
	if err == nil {
		t.Fatal("exit status 1 do docker virou sucesso")
	}
	if !strings.Contains(err.Error(), "docker stop falhou") {
		t.Errorf("erro sem contexto da ação: %v", err)
	}
	if !strings.Contains(out, "Error response") {
		t.Errorf("saída do erro perdida: %q", out)
	}
}
