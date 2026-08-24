package ssh

import (
	"encoding/json"
	"testing"
)

// O script remoto omite temperature_c em host sem sensor (VM, container). O
// ponteiro é o que separa isso de uma leitura real de zero.
func TestSysPayloadTemperaturaAusente(t *testing.T) {
	semSensor := `{"uptime":10,"host_cpu":1.5,"mem_used":1,"mem_total":2,"load1":0.1,"disk_root":"1,2","ps":[],"stats":[]}`
	comSensor := `{"uptime":10,"host_cpu":1.5,"mem_used":1,"mem_total":2,"load1":0.1,"disk_root":"1,2","temperature_c":55.9,"ps":[],"stats":[]}`

	var p SysPayload
	if err := json.Unmarshal([]byte(semSensor), &p); err != nil {
		t.Fatal(err)
	}
	if p.TemperatureC != nil {
		t.Errorf("host sem sensor virou %v, esperado nil", *p.TemperatureC)
	}

	p = SysPayload{}
	if err := json.Unmarshal([]byte(comSensor), &p); err != nil {
		t.Fatal(err)
	}
	if p.TemperatureC == nil || *p.TemperatureC != 55.9 {
		t.Errorf("temperatura lida = %v, esperado 55.9", p.TemperatureC)
	}
}
