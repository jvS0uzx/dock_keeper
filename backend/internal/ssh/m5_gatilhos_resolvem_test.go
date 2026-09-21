package ssh

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/alert"
)

const srvGatilho = "00000000-0000-0000-0000-0000000000f5"

func alvoDeGatilho() Target {
	unidade := uint(7)
	return Target{ID: srvGatilho, Name: "vps-filial", Host: "203.0.113.5", SiteID: &unidade}
}

func capturarResolvidos(t *testing.T, abertas ...string) func() []string {
	t.Helper()

	var mu sync.Mutex
	var chaves []string
	resolver, listar := resolveAlert, openAlertKeys
	resolveAlert = func(e alert.Entrada) bool {
		mu.Lock()
		defer mu.Unlock()
		chaves = append(chaves, e.Key)
		return true
	}
	openAlertKeys = func(prefixo string) []string {
		var saida []string
		for _, chave := range abertas {
			if len(chave) >= len(prefixo) && chave[:len(prefixo)] == prefixo {
				saida = append(saida, chave)
			}
		}
		return saida
	}
	t.Cleanup(func() { resolveAlert, openAlertKeys = resolver, listar })
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), chaves...)
	}
}

func TestGatilhosCarregamOrigemESeveridade(t *testing.T) {
	avisos := capturarAvisos(t)
	capturarResolvidos(t)
	alvo := alvoDeGatilho()

	d := newBruteForceDetector(alvo)
	base := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	for i := range 10 {
		d.observe(falha("198.51.100.7"), base.Add(time.Duration(i)*time.Second))
	}
	h := newLBHealth(alvo)
	for i := range 20 {
		h.observe("10.0.0.3:8080", 502, base.Add(time.Duration(i)*time.Second))
	}
	nginxDownAlert(alvo)(errors.New("EOF"))
	newVigiaDeContainers(alvo).observe([]DockerPSPayload{{Name: "worker", State: "exited"}}, nil)

	severidades := map[string]string{
		"bruteforce:" + srvGatilho + ":198.51.100.7":       "high",
		"lb_upstream_5xx:" + srvGatilho + ":10.0.0.3:8080": "high",
		"nginx_down:" + srvGatilho:                         "critical",
		"container_down:" + srvGatilho + ":worker":         "high",
	}
	vistos := map[string]bool{}
	for _, a := range avisos() {
		quer, ok := severidades[a.chave]
		if !ok {
			t.Errorf("aviso inesperado: %q", a.chave)
			continue
		}
		vistos[a.chave] = true
		if a.origem.ServerID == nil || *a.origem.ServerID != srvGatilho || a.origem.SiteID == nil || *a.origem.SiteID != 7 {
			t.Errorf("%s sem origem: servidor=%v unidade=%v; o operador da filial nunca vê este alerta",
				a.chave, a.origem.ServerID, a.origem.SiteID)
		}
		if a.origem.Severity != quer {
			t.Errorf("%s com severidade %q, esperado %q", a.chave, a.origem.Severity, quer)
		}
	}
	for chave := range severidades {
		if !vistos[chave] {
			t.Errorf("gatilho %q não disparou", chave)
		}
	}
}

func TestForcaBrutaResolveQuandoAJanelaEsvazia(t *testing.T) {
	capturarAvisos(t)
	resolvidos := capturarResolvidos(t)
	t.Setenv("BRUTEFORCE_THRESHOLD", "10")
	t.Setenv("BRUTEFORCE_WINDOW", "5m")
	d := newBruteForceDetector(alvoDeGatilho())

	base := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	for i := range 10 {
		d.observe(falha("198.51.100.7"), base.Add(time.Duration(i)*time.Second))
	}

	d.revisar(base.Add(time.Minute))
	if n := len(resolvidos()); n != 0 {
		t.Fatalf("janela ainda com falhas e %d alerta(s) resolvido(s)", n)
	}

	d.revisar(base.Add(6 * time.Minute))
	d.revisar(base.Add(7 * time.Minute))
	got := resolvidos()
	if len(got) != 1 || got[0] != "bruteforce:"+srvGatilho+":198.51.100.7" {
		t.Errorf("resolvidos = %v, esperado uma vez bruteforce:%s:198.51.100.7", got, srvGatilho)
	}
}

func TestUpstreamResolveQuandoAProporcaoVolta(t *testing.T) {
	capturarAvisos(t)
	resolvidos := capturarResolvidos(t)
	t.Setenv("LB_MIN_REQUESTS", "20")
	t.Setenv("LB_ERROR_RATIO", "0.5")
	h := newLBHealth(alvoDeGatilho())

	base := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	for i := range 20 {
		h.observe("10.0.0.3:8080", 502, base.Add(time.Duration(i)*time.Second))
	}
	for i := range 20 {
		h.observe("10.0.0.3:8080", 200, base.Add(time.Duration(20+i)*time.Second))
	}
	if n := len(resolvidos()); n != 0 {
		t.Fatalf("20 de 40 com 5xx ainda é 50%% e %d alerta(s) foram resolvidos", n)
	}

	h.observe("10.0.0.3:8080", 200, base.Add(41*time.Second))
	h.observe("10.0.0.3:8080", 200, base.Add(42*time.Second))
	got := resolvidos()
	if len(got) != 1 || got[0] != "lb_upstream_5xx:"+srvGatilho+":10.0.0.3:8080" {
		t.Errorf("resolvidos = %v, esperado uma vez o upstream 10.0.0.3:8080", got)
	}
}

func TestContainerQueVoltaARodarResolve(t *testing.T) {
	capturarAvisos(t)
	resolvidos := capturarResolvidos(t)
	v := newVigiaDeContainers(alvoDeGatilho())

	v.observe([]DockerPSPayload{{Name: "worker", State: "exited"}, {Name: "web", State: "running"}}, nil)
	if n := len(resolvidos()); n != 0 {
		t.Fatalf("container que nunca esteve parado foi resolvido: %v", resolvidos())
	}

	v.observe([]DockerPSPayload{{Name: "worker", State: "running"}, {Name: "web", State: "running"}}, nil)
	v.observe([]DockerPSPayload{{Name: "worker", State: "running"}, {Name: "web", State: "running"}}, nil)
	got := resolvidos()
	if len(got) != 1 || got[0] != "container_down:"+srvGatilho+":worker" {
		t.Errorf("resolvidos = %v, esperado uma vez o container worker", got)
	}
}

func TestVigiaRetomaOAlertaAbertoAntesDoRestart(t *testing.T) {
	capturarAvisos(t)
	resolvidos := capturarResolvidos(t, "container_down:"+srvGatilho+":worker", "container_down:outro-servidor:web")
	v := newVigiaDeContainers(alvoDeGatilho())

	v.observe([]DockerPSPayload{{Name: "worker", State: "running"}, {Name: "web", State: "running"}}, nil)

	got := resolvidos()
	if len(got) != 1 || got[0] != "container_down:"+srvGatilho+":worker" {
		t.Errorf("resolvidos = %v, esperado o alerta que ficou aberto antes do painel reiniciar", got)
	}
}

func TestHostDeVoltaResolveNaPrimeiraAmostra(t *testing.T) {
	srv := servidorDeTeste(t)
	capturarAvisos(t)
	resolvidos := capturarResolvidos(t)
	alvo := alvoComServidor(t, func(string) respostaExec {
		return respostaExec{stdout: `{"uptime":10,"disk_root":"1,2"}` + "\n" + `{"uptime":11,"disk_root":"1,2"}` + "\n", consomeStdin: true}
	})
	alvo.ID = srv.ID

	if err := StartStream(context.Background(), alvo); err != nil {
		t.Fatalf("stream falhou: %v", err)
	}

	got := resolvidos()
	if len(got) != 1 || got[0] != "host_unreachable:"+srv.ID {
		t.Errorf("resolvidos = %v, esperado host_unreachable:%s uma vez, na primeira amostra", got, srv.ID)
	}
}

func TestStreamSoContaComoDePeDepoisDeEstavel(t *testing.T) {
	var avisou atomic.Int32

	parar := aoFicarDePe(50*time.Millisecond, func() { avisou.Add(1) })
	parar()
	time.Sleep(120 * time.Millisecond)
	if n := avisou.Load(); n != 0 {
		t.Fatalf("stream que caiu antes de estabilizar contou como de pé %d vez(es)", n)
	}

	parar = aoFicarDePe(20*time.Millisecond, func() { avisou.Add(1) })
	defer parar()
	time.Sleep(120 * time.Millisecond)
	if n := avisou.Load(); n != 1 {
		t.Errorf("stream estável avisou %d vez(es), esperado 1", n)
	}
}

func TestNginxDePeResolveOAlerta(t *testing.T) {
	resolvidos := capturarResolvidos(t)

	nginxDeVolta(alvoDeGatilho())

	got := resolvidos()
	if len(got) != 1 || got[0] != "nginx_down:"+srvGatilho {
		t.Errorf("resolvidos = %v, esperado nginx_down:%s", got, srvGatilho)
	}
}
