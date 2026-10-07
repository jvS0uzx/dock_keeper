package ssh

import "testing"

func TestContainerCriadoSoAlertaDepoisDeEstabilizar(t *testing.T) {
	avisos := capturarAvisos(t)
	capturarRecuperacoes(t)
	v := newVigiaDeContainers(alvoDeGatilho())
	temporario := DockerPSPayload{DockerID: "t1", Name: "3f9a0c1b2d4e_web", State: "created"}
	web := DockerPSPayload{DockerID: "aaa", Name: "web", State: "running"}

	for range amostrasParaEstabilizar - 1 {
		v.observe([]DockerPSPayload{temporario, web}, nil)
	}
	if got := avisos(); len(got) != 0 {
		t.Fatalf("container recém-criado pelo compose alertou antes de %d amostras: %+v", amostrasParaEstabilizar, got)
	}

	v.observe([]DockerPSPayload{temporario, web}, nil)
	got := avisos()
	if len(got) != 1 || got[0].chave != "container_down:"+srvGatilho+":3f9a0c1b2d4e_web" {
		t.Errorf("avisos = %+v, esperado um container_down na %dª amostra seguida em created", got, amostrasParaEstabilizar)
	}
}

func TestContainerQueSaiDeCriadoZeraAContagem(t *testing.T) {
	avisos := capturarAvisos(t)
	capturarRecuperacoes(t)
	v := newVigiaDeContainers(alvoDeGatilho())
	criado := DockerPSPayload{DockerID: "c1", Name: "job", State: "created"}
	rodando := DockerPSPayload{DockerID: "c1", Name: "job", State: "running"}
	web := DockerPSPayload{DockerID: "aaa", Name: "web", State: "running"}

	sequencia := [][]DockerPSPayload{
		{criado, web}, {criado, web}, {rodando, web},
		{criado, web}, {criado, web}, {web},
		{criado, web}, {criado, web},
	}
	for _, ps := range sequencia {
		v.observe(ps, nil)
	}

	if got := avisos(); len(got) != 0 {
		t.Errorf("avisos = %+v: sair de created ou sumir do ps deveria zerar a contagem", got)
	}
}
