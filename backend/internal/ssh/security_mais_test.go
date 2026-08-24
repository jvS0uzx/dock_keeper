package ssh

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestGetRadarPortsParseiaSaidaRemota(t *testing.T) {
	var mu sync.Mutex
	var comando string
	saida := `tcp   LISTEN 0      4096   0.0.0.0:22    0.0.0.0:*    users:(("sshd",pid=812,fd=3))
linha truncada
udp   UNCONN 0      0      0.0.0.0:53    0.0.0.0:*    users:(("dnsmasq",pid=9,fd=4))
`
	alvo := alvoComServidor(t, func(cmd string) respostaExec {
		mu.Lock()
		comando = cmd
		mu.Unlock()
		return respostaExec{stdout: saida}
	})

	ports, err := GetRadarPorts(alvo)
	if err != nil {
		t.Fatalf("radar falhou: %v", err)
	}

	mu.Lock()
	if comando != "ss -tulnp | grep LISTEN" {
		t.Errorf("comando remoto = %q", comando)
	}
	mu.Unlock()

	// A linha truncada tem menos de cinco campos e cai fora; as outras duas
	// entram, cada uma com o processo extraído. (Ruído com cinco ou mais
	// campos passa: em produção o filtro é o grep LISTEN do lado remoto.)
	if len(ports) != 2 {
		t.Fatalf("portas = %d, esperado 2: %+v", len(ports), ports)
	}
	if ports[0].Port != "22" || ports[0].Process != "sshd" || ports[0].Protocol != "tcp" {
		t.Errorf("primeira porta = %+v", ports[0])
	}
	if ports[1].Port != "53" || ports[1].Process != "dnsmasq" {
		t.Errorf("segunda porta = %+v", ports[1])
	}
}

func TestGetRadarPortsPropagaFalhaDoComando(t *testing.T) {
	alvo := alvoComServidor(t, func(string) respostaExec {
		return respostaExec{status: 1}
	})
	if _, err := GetRadarPorts(alvo); err == nil {
		t.Fatal("exit status 1 virou lista de portas")
	}
}

func TestStreamAuthLogsRepassaLinhasPorSSE(t *testing.T) {
	var mu sync.Mutex
	var comando string
	alvo := alvoComServidor(t, func(cmd string) respostaExec {
		mu.Lock()
		comando = cmd
		mu.Unlock()
		return respostaExec{stdout: "Failed password for root\nAccepted publickey for deploy\n"}
	})

	rec := httptest.NewRecorder()
	if err := StreamAuthLogs(context.Background(), alvo, rec, rec); err != nil {
		t.Fatalf("stream falhou: %v", err)
	}

	mu.Lock()
	if !strings.HasPrefix(comando, "tail -n 20 -f ") {
		t.Errorf("comando remoto = %q, esperado tail com histórico de 20 linhas", comando)
	}
	mu.Unlock()

	corpo := rec.Body.String()
	for _, evento := range []string{"data: Failed password for root\n\n", "data: Accepted publickey for deploy\n\n"} {
		if !strings.Contains(corpo, evento) {
			t.Errorf("SSE sem o evento %q; corpo: %q", evento, corpo)
		}
	}
}

func TestProcessNameFormatoInesperado(t *testing.T) {
	casos := []string{`users:(("sshd`, `sem-parenteses`, `users:(()`, ``}
	for _, campo := range casos {
		if nome, ok := processName(campo); ok {
			t.Errorf("processName(%q) aceitou e devolveu %q", campo, nome)
		}
	}
	if nome, ok := processName(`users:(("nginx",pid=1,fd=6))`); !ok || nome != "nginx" {
		t.Errorf("processName válido = %q, %v", nome, ok)
	}
}
