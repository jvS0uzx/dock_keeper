package ssh

import (
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func TestContainerParadoSaiComAlvoDeContainer(t *testing.T) {
	avisos := capturarAvisos(t)
	capturarResolvidos(t)
	v := newVigiaDeContainers(alvoDeGatilho())

	v.observe(
		[]DockerPSPayload{{DockerID: "abc123def456", Name: "api", State: "exited"}},
		nil,
	)

	got := avisos()
	if len(got) != 1 {
		t.Fatalf("avisos = %+v, esperado exatamente um", got)
	}
	e := got[0].origem
	if e.AlvoTipo != database.AlvoTipoContainer {
		t.Errorf("AlvoTipo = %q, esperado %q", e.AlvoTipo, database.AlvoTipoContainer)
	}
	if e.AlvoID != "abc123def456" {
		t.Errorf("AlvoID = %q, esperado o docker_id", e.AlvoID)
	}
	if e.AlvoNome != "api" {
		t.Errorf("AlvoNome = %q, esperado o nome do container", e.AlvoNome)
	}
	if e.Metrica != "estado" {
		t.Errorf("Metrica = %q, esperado estado", e.Metrica)
	}
	if e.Valor != nil || e.Limiar != nil {
		t.Errorf("Valor = %v e Limiar = %v, estado não é medida numérica", e.Valor, e.Limiar)
	}
}

func TestHostInalcancavelSaiComAlvoDeHost(t *testing.T) {
	capturarAvisos(t)
	resolvidos := capturarResolvidos(t)

	entrada := alertaDe(alvoDeGatilho(), "host_unreachable:"+srvGatilho, "critical", "texto", alvoDoHost(alvoDeGatilho()))

	if entrada.AlvoTipo != database.AlvoTipoHost {
		t.Errorf("AlvoTipo = %q, esperado %q", entrada.AlvoTipo, database.AlvoTipoHost)
	}
	if entrada.AlvoID != srvGatilho || entrada.AlvoNome != "vps-filial" {
		t.Errorf("alvo = (%q, %q), esperado o id e o nome da VPS", entrada.AlvoID, entrada.AlvoNome)
	}
	if entrada.Valor != nil {
		t.Errorf("Valor = %v, esperado nulo para host inalcançável", entrada.Valor)
	}
	_ = resolvidos
}

func TestForcaBrutaSaiComOHostAtacadoEAContagem(t *testing.T) {
	avisos := capturarAvisos(t)
	capturarResolvidos(t)
	d := newBruteForceDetector(alvoDeGatilho())

	agora := time.Now()
	for range d.threshold {
		d.observe("Sep 18 10:00:01 vps sshd[811]: Failed password for root from 198.51.100.7 port 51422 ssh2", agora)
	}

	got := avisos()
	if len(got) == 0 {
		t.Fatalf("nenhum aviso: esperado alerta de força bruta")
	}
	e := got[len(got)-1].origem
	if e.AlvoTipo != database.AlvoTipoHost || e.AlvoID != srvGatilho {
		t.Errorf("alvo = (%q, %q), o alvo é o host atacado e não o IP atacante", e.AlvoTipo, e.AlvoID)
	}
	if e.Metrica != "falhas_de_login" {
		t.Errorf("Metrica = %q, esperado falhas_de_login", e.Metrica)
	}
	if e.Valor == nil || *e.Valor < float64(d.threshold) {
		t.Errorf("Valor = %v, esperado a contagem observada", e.Valor)
	}
	if e.Limiar == nil || *e.Limiar != float64(d.threshold) {
		t.Errorf("Limiar = %v, esperado %d", e.Limiar, d.threshold)
	}
}

func TestUpstreamComErroSaiComAlvoDeServicoETaxa(t *testing.T) {
	avisos := capturarAvisos(t)
	capturarResolvidos(t)
	h := newLBHealth(alvoDeGatilho())

	agora := time.Now()
	for range h.minRequests {
		h.observe("api-interna", 500, agora)
	}

	got := avisos()
	if len(got) == 0 {
		t.Fatalf("nenhum aviso: esperado alerta de upstream doente")
	}
	e := got[len(got)-1].origem
	if e.AlvoTipo != database.AlvoTipoServico || e.AlvoNome != "api-interna" {
		t.Errorf("alvo = (%q, %q), esperado o upstream como serviço", e.AlvoTipo, e.AlvoNome)
	}
	if e.Metrica != "taxa_de_erro" {
		t.Errorf("Metrica = %q, esperado taxa_de_erro", e.Metrica)
	}
	if e.Valor == nil || *e.Valor <= 0 {
		t.Errorf("Valor = %v, esperado a taxa observada", e.Valor)
	}
	if e.Limiar == nil || *e.Limiar != h.ratio {
		t.Errorf("Limiar = %v, esperado %v", e.Limiar, h.ratio)
	}
}
