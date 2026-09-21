package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shirou/gopsutil/v3/load"
)

func TestCPUComFalhaDeLeituraNaoSai(t *testing.T) {
	falha := func() ([]float64, error) { return nil, errors.New("sem acesso ao contador") }
	if got := medirCPU(falha); got != nil {
		t.Fatalf("cpu com falha de leitura = %v, esperado nulo", *got)
	}
	vazio := func() ([]float64, error) { return []float64{}, nil }
	if got := medirCPU(vazio); got != nil {
		t.Fatalf("cpu sem amostra = %v, esperado nulo", *got)
	}
}

func TestCPUZeroMedidoContinuaZero(t *testing.T) {
	ocioso := func() ([]float64, error) { return []float64{0}, nil }
	got := medirCPU(ocioso)
	if got == nil || *got != 0 {
		t.Fatalf("cpu ociosa medida = %v, esperado 0 medido", got)
	}
	corpo, err := json.Marshal(metricsPayload{CPU: got})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(corpo), `"cpu":0`) {
		t.Fatalf("zero medido sumiu do envio: %s", corpo)
	}
}

func TestLoadNoWindowsNaoSai(t *testing.T) {
	leitor := func() (*load.AvgStat, error) { return &load.AvgStat{Load1: 0}, nil }
	if got := medirLoad("windows", leitor); got != nil {
		t.Fatalf("load1 no Windows = %v, esperado nulo: a plataforma não tem load average", *got)
	}
}

func TestLoadNoLinuxSaiMesmoZero(t *testing.T) {
	leitor := func() (*load.AvgStat, error) { return &load.AvgStat{Load1: 0}, nil }
	got := medirLoad("linux", leitor)
	if got == nil || *got != 0 {
		t.Fatalf("load1 medido no Linux = %v, esperado 0 medido", got)
	}
	falha := func() (*load.AvgStat, error) { return nil, errors.New("sem /proc/loadavg") }
	if got := medirLoad("linux", falha); got != nil {
		t.Fatalf("load1 com falha de leitura = %v, esperado nulo", *got)
	}
}

func TestEnvioSemMedidaOmiteOsCampos(t *testing.T) {
	corpo, err := json.Marshal(metricsPayload{Hostname: "estacao"})
	if err != nil {
		t.Fatal(err)
	}
	for _, campo := range []string{`"cpu"`, `"load1"`} {
		if strings.Contains(string(corpo), campo) {
			t.Fatalf("campo %s saiu sem medida: %s", campo, corpo)
		}
	}
}

func TestFormatPercentSemMedida(t *testing.T) {
	if got := formatPct(nil); got != "sem medida" {
		t.Fatalf("formatPct(nil) = %q", got)
	}
	v := 12.34
	if got := formatPct(&v); got != "12.3%" {
		t.Fatalf("formatPct(12.34) = %q", got)
	}
}
