package ssh

import (
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func amostraGravada(t *testing.T, serverID string) database.MetricServer {
	t.Helper()

	var m database.MetricServer
	if err := database.DB.Where("server_id = ?", serverID).Order("id desc").Take(&m).Error; err != nil {
		t.Fatalf("métrica não gravada: %v", err)
	}
	return m
}

func TestAmostraSemDeltaDeCPUGravaNulo(t *testing.T) {
	srv := servidorDeTeste(t)
	alvo := Target{ID: srv.ID, Host: srv.HostIP}

	storeHostMetric(alvo, SysPayload{Uptime: 10, MemUsed: 100, MemTotal: 200, DiskRoot: "10,100"}, 900)

	m := amostraGravada(t, srv.ID)
	if m.CPUUsagePercent != nil {
		t.Errorf("cpu gravada = %v, esperado NULL: sem delta não houve medição", *m.CPUUsagePercent)
	}
	if m.LoadAvg1 != nil {
		t.Errorf("load gravado = %v, esperado NULL", *m.LoadAvg1)
	}
	if m.MemTotalBytes != 200 {
		t.Errorf("mem_total = %d, esperado 200: a amostra sem cpu ainda vale pelo resto", m.MemTotalBytes)
	}
}

func TestAmostraComCPUZeroGravaZero(t *testing.T) {
	srv := servidorDeTeste(t)
	alvo := Target{ID: srv.ID, Host: srv.HostIP}

	zero, carga := 0.0, 0.0
	storeHostMetric(alvo, SysPayload{
		Uptime: 10, HostCPU: &zero, Load1: &carga,
		MemUsed: 100, MemTotal: 200, DiskRoot: "10,100",
	}, 900)

	m := amostraGravada(t, srv.ID)
	if m.CPUUsagePercent == nil || *m.CPUUsagePercent != 0 {
		t.Errorf("cpu gravada = %v, esperado 0: máquina ociosa é medição, não ausência", m.CPUUsagePercent)
	}
	if m.LoadAvg1 == nil || *m.LoadAvg1 != 0 {
		t.Errorf("load gravado = %v, esperado 0", m.LoadAvg1)
	}
}
