package ssh

import (
	"os"
	"testing"
	"time"

	"github.com/joaov/vd_stats/internal/database"
)

// Testes que gravam de verdade: seguem o padrão do repositório — pulam sem
// DATABASE_URL e limpam o que criaram, identificado por um nome inequívoco.

const nomeServidorDeTeste = "zz-teste-ssh-cobertura"

func servidorDeTeste(t *testing.T) database.Server {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste com banco")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}

	srv := database.Server{Name: nomeServidorDeTeste, HostIP: "203.0.113.99"}
	if err := database.DB.Create(&srv).Error; err != nil {
		t.Fatalf("criar servidor de teste: %v", err)
	}
	t.Cleanup(func() { limpaServidorDeTeste(t, srv.ID) })
	return srv
}

func limpaServidorDeTeste(t *testing.T, serverID string) {
	t.Helper()
	var containers []database.Container
	database.DB.Where("server_id = ?", serverID).Find(&containers)
	for _, c := range containers {
		database.DB.Where("container_id = ?", c.ID).Delete(&database.MetricContainer{})
	}
	database.DB.Where("server_id = ?", serverID).Delete(&database.Container{})
	database.DB.Where("server_id = ?", serverID).Delete(&database.MetricServer{})
	database.DB.Where("server_id = ?", serverID).Delete(&database.MetricLoadBalancer{})
	database.DB.Unscoped().Where("id = ?", serverID).Delete(&database.Server{})
}

func TestStoreHostMetricPersisteAmostra(t *testing.T) {
	srv := servidorDeTeste(t)
	alvo := Target{ID: srv.ID, Host: srv.HostIP}

	temp := 41.5
	payload := SysPayload{
		Uptime: 3600, HostCPU: 12.5, MemUsed: 100, MemTotal: 200,
		Load1: 0.5, DiskRoot: "10,100", TemperatureC: &temp,
	}
	storeHostMetric(alvo, payload, 1200)

	var m database.MetricServer
	if err := database.DB.Where("server_id = ?", srv.ID).Take(&m).Error; err != nil {
		t.Fatalf("métrica não gravada: %v", err)
	}
	if m.DiskUsedBytes != 10 || m.DiskTotalBytes != 100 {
		t.Errorf("disco = %d/%d, esperado 10/100", m.DiskUsedBytes, m.DiskTotalBytes)
	}
	if m.SSHHandshakeMs == nil || *m.SSHHandshakeMs != 1200 {
		t.Errorf("handshake = %v, esperado 1200", m.SSHHandshakeMs)
	}
	if m.TemperatureC == nil || *m.TemperatureC != 41.5 {
		t.Errorf("temperatura = %v, esperado 41.5", m.TemperatureC)
	}
}

func TestStoreContainerMetricsCriaUmaVezEReusaOCache(t *testing.T) {
	srv := servidorDeTeste(t)
	alvo := Target{ID: srv.ID, Host: srv.HostIP}

	payload := SysPayload{
		PS:    []DockerPSPayload{{DockerID: "zzd1", Name: "zz-cont", Project: "/opt/app", State: "running", Status: "Up 2h"}},
		Stats: []DockerStatsPayload{{DockerID: "zzd1", CPUPercent: "1.5%", MemUsage: "10MiB / 20MiB"}},
	}

	cache := make(map[string]string)
	storeContainerMetrics(alvo, payload, cache)
	storeContainerMetrics(alvo, payload, cache)

	var containers int64
	database.DB.Model(&database.Container{}).Where("server_id = ?", srv.ID).Count(&containers)
	if containers != 1 {
		t.Fatalf("containers = %d, esperado 1 (o cache evita o segundo FirstOrCreate)", containers)
	}

	var c database.Container
	database.DB.Where("server_id = ?", srv.ID).Take(&c)
	var metricas []database.MetricContainer
	database.DB.Where("container_id = ?", c.ID).Find(&metricas)
	if len(metricas) != 2 {
		t.Fatalf("métricas = %d, esperado 2 (uma por ciclo)", len(metricas))
	}
	if metricas[0].MemUsedBytes != 10*(1<<20) || metricas[0].MemLimitBytes != 20*(1<<20) {
		t.Errorf("memória = %d/%d, esperado 10MiB/20MiB em bytes",
			metricas[0].MemUsedBytes, metricas[0].MemLimitBytes)
	}
	if metricas[0].CPUUsagePercent != 1.5 {
		t.Errorf("cpu = %v, esperado 1.5", metricas[0].CPUUsagePercent)
	}
}

func TestLBOriginEFlushRecortamPeloServidor(t *testing.T) {
	srv := servidorDeTeste(t)

	serverID, siteID := lbOrigin(Target{ID: srv.ID, Name: srv.Name})
	if serverID == nil || *serverID != srv.ID {
		t.Fatalf("lbOrigin devolveu %v, esperado o id do servidor", serverID)
	}
	if siteID != nil {
		t.Errorf("servidor sem unidade devolveu site %v", *siteID)
	}

	c := newLBCounter(serverID, siteID)
	c.add(lbKey{Upstream: "10.0.0.2:8080", ServerName: "zz-app-teste", Status: "502"})
	c.flush()

	var linha database.MetricLoadBalancer
	err := database.DB.Where("server_id = ?", srv.ID).Take(&linha).Error
	if err != nil {
		t.Fatalf("flush não gravou a linha do balanceador: %v", err)
	}
	if linha.RequestsCount != 1 || linha.Status != "502" || linha.UpstreamAddr != "10.0.0.2:8080" {
		t.Errorf("linha = %+v", linha)
	}
	if linha.Timestamp.After(time.Now().UTC().Add(time.Minute)) {
		t.Errorf("timestamp no futuro: %v", linha.Timestamp)
	}

	// flush esvazia o acumulador: um segundo flush não pode duplicar a linha.
	c.flush()
	var total int64
	database.DB.Model(&database.MetricLoadBalancer{}).Where("server_id = ?", srv.ID).Count(&total)
	if total != 1 {
		t.Errorf("linhas após segundo flush = %d, esperado 1", total)
	}
}

func TestLBOriginServidorInexistenteFicaSemRecorte(t *testing.T) {
	servidorDeTeste(t) // garante conexão com o banco (ou skip)

	// UUID válido que não existe: a consulta falha e a métrica segue sem
	// unidade em vez de derrubar a coleta.
	id, site := lbOrigin(Target{ID: "00000000-0000-0000-0000-000000000000", Name: "fantasma"})
	if id != nil || site != nil {
		t.Errorf("servidor inexistente devolveu (%v, %v)", id, site)
	}
}
