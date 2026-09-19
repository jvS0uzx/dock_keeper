package api

import (
	"encoding/json"
	"testing"
)

func TestTemperatureOfDescartaZero(t *testing.T) {
	zero := 0.0
	real := 41.5

	if got := temperatureOf(ingestPayload{TemperatureC: nil}); got != nil {
		t.Errorf("campo ausente virou %v, esperado nil", *got)
	}
	if got := temperatureOf(ingestPayload{TemperatureC: &zero}); got != nil {
		t.Errorf("zero virou %v, esperado nil", *got)
	}
	got := temperatureOf(ingestPayload{TemperatureC: &real})
	if got == nil || *got != real {
		t.Errorf("leitura real foi descartada: %v", got)
	}
}

func TestServerLiveStatSerializaNulo(t *testing.T) {
	b, err := json.Marshal(ServerLiveStat{ID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, campo := range []string{"temperature_c", "ssh_handshake_ms"} {
		v, ok := m[campo]
		if !ok {
			t.Errorf("%s sumiu do JSON; o painel espera a chave presente", campo)
			continue
		}
		if v != nil {
			t.Errorf("%s = %v, esperado null", campo, v)
		}
	}
}
