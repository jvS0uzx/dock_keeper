package ssh

import (
	"strings"
	"sync"
	"testing"
)

// O "--" no comando é a segunda camada da defesa do S9: mesmo que a regex de
// nome afrouxe um dia, o docker deixa de ler o nome como flag.
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
	// A saída volta mesmo no erro: é o que o operador lê para entender a falha.
	if !strings.Contains(out, "Error response") {
		t.Errorf("saída do erro perdida: %q", out)
	}
}
