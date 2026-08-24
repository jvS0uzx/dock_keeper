package ssh

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/joaov/vd_stats/internal/database"
)

func TestIsValidContainerNameExpoeARegra(t *testing.T) {
	if !IsValidContainerName("web-01") {
		t.Error("nome legítimo recusado")
	}
	if IsValidContainerName("-f") {
		t.Error("hífen inicial aceito: o docker o leria como flag")
	}
}

// sessaoSemStdin cobre o único caminho de runScript que a sessaoFalsa do
// collect_config_test não alcança: a falha ao abrir o stdin.
type sessaoSemStdin struct{}

func (sessaoSemStdin) StdinPipe() (io.WriteCloser, error) { return nil, errors.New("sem stdin") }
func (sessaoSemStdin) Start(string) error                 { return nil }

func TestRunScriptPropagaFalhaDoStdin(t *testing.T) {
	if err := runScript(sessaoSemStdin{}, "x"); err == nil {
		t.Fatal("falha do StdinPipe foi engolida")
	}
}

func TestParseNginxLineDescartaFormatosQuebrados(t *testing.T) {
	casos := map[string]string{
		"sem separador do upstream": "10.0.0.9 - app.exemplo.com to: semdoispontos",
		"upstream vazio":            "10.0.0.9 - app.exemplo.com to: : GET / 200 10",
	}
	for nome, linha := range casos {
		if _, ok := parseNginxLine(linha); ok {
			t.Errorf("%s: linha aceita: %q", nome, linha)
		}
	}
}

func TestLBCounterAcumulaPorChave(t *testing.T) {
	c := newLBCounter(nil, nil)
	chave := lbKey{Upstream: "10.0.0.2:8080", ServerName: "app", Status: "200"}
	c.add(chave)
	c.add(chave)
	c.add(lbKey{Upstream: "10.0.0.3:8080", ServerName: "app", Status: "502"})

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.counts) != 2 {
		t.Fatalf("chaves = %d, esperado 2", len(c.counts))
	}
	if c.counts[chave] != 2 {
		t.Errorf("contagem da chave repetida = %d, esperado 2", c.counts[chave])
	}
}

func TestLBCounterFlushVazioNaoTocaOBanco(t *testing.T) {
	// Sem linha pendente o flush não pode ir ao banco — este teste roda com
	// database.DB nulo e um Create aqui seria pânico.
	c := newLBCounter(nil, nil)
	c.flush()
}

func TestLBOriginSemIdentidadeResolveNulo(t *testing.T) {
	if id, site := lbOrigin(Target{}); id != nil || site != nil {
		t.Errorf("alvo sem id devolveu (%v, %v)", id, site)
	}
	if database.DB == nil {
		if id, site := lbOrigin(Target{ID: "qualquer"}); id != nil || site != nil {
			t.Errorf("sem banco devolveu (%v, %v)", id, site)
		}
	}
}

func TestStreamDockerLogsRecusaNomeInvalido(t *testing.T) {
	rec := httptest.NewRecorder()
	err := StreamDockerLogs(context.Background(), Target{}, "-f", rec, rec)
	if err == nil || !strings.Contains(err.Error(), "nome de container inválido") {
		t.Fatalf("nome perigoso não foi recusado: %v", err)
	}
}

func TestStreamDockerLogsTransmiteAsDuasSaidas(t *testing.T) {
	var mu sync.Mutex
	var comando string
	alvo := alvoComServidor(t, func(cmd string) respostaExec {
		mu.Lock()
		comando = cmd
		mu.Unlock()
		// docker logs manda a saída da aplicação também em stderr; o stream
		// tem de repassar as duas pontas.
		return respostaExec{stdout: "app de pé\n", stderr: "warn: cache frio\n"}
	})

	rec := httptest.NewRecorder()
	if err := StreamDockerLogs(context.Background(), alvo, "web", rec, rec); err != nil {
		t.Fatalf("stream falhou: %v", err)
	}

	mu.Lock()
	if comando != "docker logs -f --tail 100 -- web" {
		t.Errorf("comando remoto = %q", comando)
	}
	mu.Unlock()
	corpo := rec.Body.String()
	if !strings.Contains(corpo, "data: app de pé\n\n") {
		t.Errorf("stdout não chegou ao SSE: %q", corpo)
	}
	if !strings.Contains(corpo, "data: warn: cache frio\n\n") {
		t.Errorf("stderr não chegou ao SSE: %q", corpo)
	}
}

func TestStartStreamToleraPayloadInvalido(t *testing.T) {
	alvo := alvoComServidor(t, func(string) respostaExec {
		// Linha que não é JSON tem de ser pulada sem derrubar o stream — é o
		// contrato com o stream_metrics.sh, que tolera linha suja.
		return respostaExec{stdout: "isto não é json\n", consomeStdin: true}
	})

	if err := StartStream(context.Background(), alvo); err != nil {
		t.Fatalf("linha inválida derrubou o stream: %v", err)
	}
}

func TestStartNginxStreamIgnoraLinhaSemUpstream(t *testing.T) {
	alvo := alvoComServidor(t, func(string) respostaExec {
		return respostaExec{stdout: "linha qualquer sem o marcador\n", consomeStdin: true}
	})

	if err := StartNginxStream(context.Background(), alvo); err != nil {
		t.Fatalf("stream do nginx falhou: %v", err)
	}
}
