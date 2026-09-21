package ssh

import (
	"encoding/json"
	"testing"
)

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

func TestSysPayloadInspecaoDeContainer(t *testing.T) {
	bruto := `{"uptime":10,"host_cpu":1.5,"mem_used":1,"mem_total":2,"load1":0.1,"disk_root":"1,2","ps":[],"stats":[],
		"inspect":[
			{"docker_id":"abc123456789","restart_count":7,"oom_killed":true,"health":"unhealthy"},
			{"docker_id":"def123456789","restart_count":0,"oom_killed":false,"health":""}
		]}`

	var p SysPayload
	if err := json.Unmarshal([]byte(bruto), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Inspect) != 2 {
		t.Fatalf("inspect lido com %d itens, esperado 2", len(p.Inspect))
	}

	loop := p.Inspect[0]
	if loop.RestartCount == nil || *loop.RestartCount != 7 {
		t.Errorf("restart_count = %v, esperado 7", loop.RestartCount)
	}
	if loop.OOMKilled == nil || !*loop.OOMKilled {
		t.Errorf("oom_killed = %v, esperado true", loop.OOMKilled)
	}
	if loop.Health != "unhealthy" {
		t.Errorf("health = %q, esperado \"unhealthy\"", loop.Health)
	}

	saudavel := p.Inspect[1]
	if saudavel.RestartCount == nil || *saudavel.RestartCount != 0 {
		t.Errorf("restart_count observado como zero virou %v", saudavel.RestartCount)
	}
	if saudavel.Health != "" {
		t.Errorf("container sem healthcheck virou %q, esperado vazio", saudavel.Health)
	}
}

func TestSysPayloadInspecaoAusenteNaoViraZero(t *testing.T) {
	semInspecao := `{"uptime":10,"host_cpu":1.5,"mem_used":1,"mem_total":2,"load1":0.1,"disk_root":"1,2","ps":[],"stats":[]}`

	var p SysPayload
	if err := json.Unmarshal([]byte(semInspecao), &p); err != nil {
		t.Fatal(err)
	}
	if p.Inspect != nil {
		t.Errorf("host sem inspeção virou %v, esperado nil", p.Inspect)
	}

	campoAusente := `{"uptime":10,"host_cpu":1.5,"mem_used":1,"mem_total":2,"load1":0.1,"disk_root":"1,2","ps":[],"stats":[],
		"inspect":[{"docker_id":"abc123456789","health":""}]}`

	p = SysPayload{}
	if err := json.Unmarshal([]byte(campoAusente), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Inspect) != 1 {
		t.Fatalf("inspect lido com %d itens, esperado 1", len(p.Inspect))
	}
	if p.Inspect[0].RestartCount != nil {
		t.Errorf("restart_count ausente virou %v, esperado nil", *p.Inspect[0].RestartCount)
	}
	if p.Inspect[0].OOMKilled != nil {
		t.Errorf("oom_killed ausente virou %v, esperado nil", *p.Inspect[0].OOMKilled)
	}
}
