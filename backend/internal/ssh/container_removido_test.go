package ssh

import (
	"sync"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/alert"
)

func capturarRecuperacoes(t *testing.T, abertas ...string) func() []alert.Entrada {
	t.Helper()

	capturarResolvidos(t, abertas...)
	var mu sync.Mutex
	var entradas []alert.Entrada
	resolveAlert = func(e alert.Entrada) bool {
		mu.Lock()
		defer mu.Unlock()
		entradas = append(entradas, e)
		return true
	}
	return func() []alert.Entrada {
		mu.Lock()
		defer mu.Unlock()
		return append([]alert.Entrada(nil), entradas...)
	}
}

func TestContainerRemovidoFechaOAlertaAbertoAntesDoRestart(t *testing.T) {
	capturarAvisos(t)
	temporario := "container_down:" + srvGatilho + ":3f9a0c1b2d4e_web"
	resolvidos := capturarRecuperacoes(t, temporario)
	v := newVigiaDeContainers(alvoDeGatilho())

	v.observe([]DockerPSPayload{{DockerID: "aaa", Name: "web", State: "running"}}, nil)

	got := resolvidos()
	if len(got) != 1 || got[0].Key != temporario {
		t.Fatalf("resolvidos = %+v, esperado só %s: o container sumiu do docker ps -a", got, temporario)
	}
	if quer := "[INFO] Container 3f9a0c1b2d4e_web foi removido de 203.0.113.5"; got[0].Text != quer {
		t.Errorf("texto = %q, esperado %q", got[0].Text, quer)
	}
	if got[0].ServerID == nil || *got[0].ServerID != srvGatilho {
		t.Errorf("a recuperação perdeu o servidor de origem: %v", got[0].ServerID)
	}
}

func TestContainerParadoQueSomeFechaOAlerta(t *testing.T) {
	avisos := capturarAvisos(t)
	resolvidos := capturarRecuperacoes(t)
	v := newVigiaDeContainers(alvoDeGatilho())

	v.observe([]DockerPSPayload{{DockerID: "w1", Name: "worker", State: "exited"}, {DockerID: "aaa", Name: "web", State: "running"}}, nil)
	if n := len(avisos()); n != 1 {
		t.Fatalf("preparação falhou: %d aviso(s), esperado 1 para o worker parado", n)
	}

	v.observe([]DockerPSPayload{{DockerID: "aaa", Name: "web", State: "running"}}, nil)
	v.observe([]DockerPSPayload{{DockerID: "aaa", Name: "web", State: "running"}}, nil)

	got := resolvidos()
	if len(got) != 1 || got[0].Key != "container_down:"+srvGatilho+":worker" {
		t.Errorf("resolvidos = %+v, esperado uma vez o worker removido", got)
	}
}

func TestRodadaSemContainersNaoFechaNada(t *testing.T) {
	capturarAvisos(t)
	resolvidos := capturarRecuperacoes(t, "container_down:"+srvGatilho+":worker")
	v := newVigiaDeContainers(alvoDeGatilho())

	v.observe(nil, nil)
	v.observe([]DockerPSPayload{}, nil)

	if got := resolvidos(); len(got) != 0 {
		t.Errorf("rodada com ps vazio fechou %+v; docker ps falhando também chega como \"ps\":[] e não prova que o container sumiu", got)
	}
}

func TestContainerRemovidoLimpaOEstadoPorDockerID(t *testing.T) {
	capturarAvisos(t)
	capturarRecuperacoes(t)
	v := newVigiaDeContainers(alvoDeGatilho())

	for _, n := range []int{1, 2, 3} {
		v.observe([]DockerPSPayload{{DockerID: "w1", Name: "worker", State: "running"}, {DockerID: "aaa", Name: "web", State: "running"}},
			[]DockerInspectPayload{{DockerID: "w1", RestartCount: &n}})
	}
	if !v.emLoop["w1"] {
		t.Fatal("preparação falhou: worker deveria estar em ciclo de reinício")
	}

	v.observe([]DockerPSPayload{{DockerID: "aaa", Name: "web", State: "running"}}, nil)

	for nome, estado := range map[string]map[string]int{"reinicios": v.reinicios, "crescimentos": v.crescimentos, "estaveis": v.estaveis} {
		if _, ficou := estado["w1"]; ficou {
			t.Errorf("%s ainda guarda o container removido w1", nome)
		}
	}
	if v.emLoop["w1"] {
		t.Error("emLoop ainda marca o container removido w1")
	}
}
