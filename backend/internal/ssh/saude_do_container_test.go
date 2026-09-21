package ssh

import (
	"strings"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func pontosDeReinicio(n int) *int { return &n }

func verdadeiro() *bool { v := true; return &v }

func metricasDoServidor(t *testing.T, serverID string) []database.MetricContainer {
	t.Helper()
	var c database.Container
	if err := database.DB.Where("server_id = ?", serverID).Take(&c).Error; err != nil {
		t.Fatalf("container não gravado: %v", err)
	}
	var metricas []database.MetricContainer
	database.DB.Where("container_id = ?", c.ID).Order("id ASC").Find(&metricas)
	return metricas
}

func TestStoreContainerMetricsSemInspecaoGravaNulo(t *testing.T) {
	srv := servidorDeTeste(t)
	alvo := Target{ID: srv.ID, Host: srv.HostIP}

	payload := SysPayload{
		PS: []DockerPSPayload{{DockerID: "zzs1", Name: "zz-sem-inspecao", State: "running", Status: "Up 1h"}},
	}
	storeContainerMetrics(alvo, payload, map[string]string{})

	metricas := metricasDoServidor(t, srv.ID)
	if len(metricas) != 1 {
		t.Fatalf("métricas = %d, esperado 1", len(metricas))
	}
	m := metricas[0]
	if m.Health != nil || m.RestartCount != nil || m.OOMKilled != nil {
		t.Errorf("saúde = %v/%v/%v, esperado nulo nos três quando o inspect não observou o container",
			m.Health, m.RestartCount, m.OOMKilled)
	}
}

func TestStoreContainerMetricsSemHealthcheckNaoViraNulo(t *testing.T) {
	srv := servidorDeTeste(t)
	alvo := Target{ID: srv.ID, Host: srv.HostIP}

	payload := SysPayload{
		PS: []DockerPSPayload{{DockerID: "zzs2", Name: "zz-sem-healthcheck", State: "running", Status: "Up 1h"}},
		Inspect: []DockerInspectPayload{
			{DockerID: "zzs2", RestartCount: pontosDeReinicio(0), OOMKilled: new(bool), Health: ""},
		},
	}
	storeContainerMetrics(alvo, payload, map[string]string{})

	m := metricasDoServidor(t, srv.ID)[0]
	if m.Health == nil || *m.Health != "" {
		t.Errorf("health = %v, esperado string vazia observada e não nulo", m.Health)
	}
	if m.RestartCount == nil || *m.RestartCount != 0 {
		t.Errorf("restart_count = %v, esperado zero observado e não nulo", m.RestartCount)
	}
	if m.OOMKilled == nil || *m.OOMKilled {
		t.Errorf("oom_killed = %v, esperado false observado e não nulo", m.OOMKilled)
	}
}

func amostraEmLoop(reinicios int) ([]DockerPSPayload, []DockerInspectPayload) {
	return []DockerPSPayload{{DockerID: "zzl1", Name: "worker", State: "running"}},
		[]DockerInspectPayload{{DockerID: "zzl1", RestartCount: pontosDeReinicio(reinicios), OOMKilled: new(bool)}}
}

func TestVigiaNaoResolveContainerEmRestartLoop(t *testing.T) {
	avisos := capturarAvisos(t)
	resolvidos := capturarResolvidos(t)
	v := newVigiaDeContainers(alvoDeGatilho())

	v.observe(amostraEmLoop(5))
	v.observe(amostraEmLoop(6))
	v.observe(amostraEmLoop(7))

	if got := resolvidos(); len(got) != 0 {
		t.Fatalf("resolvidos = %v, esperado nenhum enquanto o container reinicia em ciclo", got)
	}
	got := avisos()
	if len(got) == 0 || !strings.Contains(got[0].texto, "reiniciando em ciclo") {
		t.Errorf("avisos = %+v, esperado alerta de reinício em ciclo", got)
	}
}

func TestVigiaResolveDepoisDeReinicioEstabilizar(t *testing.T) {
	capturarAvisos(t)
	resolvidos := capturarResolvidos(t)
	v := newVigiaDeContainers(alvoDeGatilho())

	v.observe(amostraEmLoop(5))
	v.observe(amostraEmLoop(6))
	v.observe(amostraEmLoop(7))
	for range amostrasParaEstabilizar {
		v.observe(amostraEmLoop(7))
	}

	got := resolvidos()
	if len(got) != 1 || got[0] != "container_down:"+srvGatilho+":worker" {
		t.Errorf("resolvidos = %v, esperado uma resolução após %d amostras estáveis", got, amostrasParaEstabilizar)
	}
}

func TestVigiaSemContagemAnteriorNaoInventaLoop(t *testing.T) {
	capturarAvisos(t)
	resolvidos := capturarResolvidos(t, "container_down:"+srvGatilho+":worker")
	v := newVigiaDeContainers(alvoDeGatilho())

	v.observe(amostraEmLoop(5))

	got := resolvidos()
	if len(got) != 1 {
		t.Errorf("resolvidos = %v, esperado resolver: primeira amostra não prova crescimento", got)
	}
}

func TestVigiaUmReinicioIsoladoNaoViraLoop(t *testing.T) {
	avisos := capturarAvisos(t)
	capturarResolvidos(t)
	v := newVigiaDeContainers(alvoDeGatilho())

	v.observe(amostraEmLoop(5))
	v.observe(amostraEmLoop(6))

	for _, a := range avisos() {
		if strings.Contains(a.texto, "reiniciando em ciclo") {
			t.Fatalf("avisos = %+v, um reinício isolado não é ciclo", a)
		}
	}
}

func TestVigiaOOMAntigoNaoAlertaSemNovoReinicio(t *testing.T) {
	avisos := capturarAvisos(t)
	capturarResolvidos(t)
	v := newVigiaDeContainers(alvoDeGatilho())

	amostra := func() ([]DockerPSPayload, []DockerInspectPayload) {
		return []DockerPSPayload{{DockerID: "zzo9", Name: "fila", State: "running"}},
			[]DockerInspectPayload{{DockerID: "zzo9", RestartCount: pontosDeReinicio(2), OOMKilled: verdadeiro()}}
	}
	v.observe(amostra())
	v.observe(amostra())

	for _, a := range avisos() {
		if strings.Contains(a.texto, "falta de memória") {
			t.Fatalf("avisos = %+v, OOM antigo sem reinício novo não é incidente ativo", a)
		}
	}
}

func TestVigiaAlertaUnhealthyEOOMComEstadoRunning(t *testing.T) {
	avisos := capturarAvisos(t)
	resolvidos := capturarResolvidos(t)
	v := newVigiaDeContainers(alvoDeGatilho())

	v.observe(
		[]DockerPSPayload{
			{DockerID: "zzu1", Name: "web", State: "running"},
			{DockerID: "zzo1", Name: "fila", State: "running"},
		},
		[]DockerInspectPayload{
			{DockerID: "zzu1", Health: "unhealthy"},
			{DockerID: "zzo1", RestartCount: pontosDeReinicio(3), OOMKilled: verdadeiro()},
		},
	)

	v.observe(
		[]DockerPSPayload{
			{DockerID: "zzu1", Name: "web", State: "running"},
			{DockerID: "zzo1", Name: "fila", State: "running"},
		},
		[]DockerInspectPayload{
			{DockerID: "zzu1", Health: "unhealthy"},
			{DockerID: "zzo1", RestartCount: pontosDeReinicio(4), OOMKilled: verdadeiro()},
		},
	)

	if got := resolvidos(); len(got) != 0 {
		t.Fatalf("resolvidos = %v, esperado nenhum: unhealthy e OOM não são recuperação", got)
	}
	textos := ""
	for _, a := range avisos() {
		textos += a.texto + "\n"
	}
	if !strings.Contains(textos, "unhealthy") {
		t.Errorf("avisos = %q, esperado alerta de healthcheck unhealthy", textos)
	}
	if !strings.Contains(textos, "falta de memória") {
		t.Errorf("avisos = %q, esperado alerta de morte por falta de memória", textos)
	}
}
