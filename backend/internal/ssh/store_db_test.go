package ssh

import (
	"os"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

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
	cpu125, carga05 := 12.5, 0.5
	payload := SysPayload{
		Uptime: 3600, HostCPU: &cpu125, MemUsed: 100, MemTotal: 200,
		Load1: &carga05, DiskRoot: "10,100", TemperatureC: &temp,
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
		t.Errorf("cpu de container = %v, esperado 1.5", metricas[0].CPUUsagePercent)
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

	c.flush()
	var total int64
	database.DB.Model(&database.MetricLoadBalancer{}).Where("server_id = ?", srv.ID).Count(&total)
	if total != 1 {
		t.Errorf("linhas após segundo flush = %d, esperado 1", total)
	}
}

func TestLBOriginServidorInexistenteFicaSemRecorte(t *testing.T) {
	servidorDeTeste(t)

	id, site := lbOrigin(Target{ID: "00000000-0000-0000-0000-000000000000", Name: "fantasma"})
	if id != nil || site != nil {
		t.Errorf("servidor inexistente devolveu (%v, %v)", id, site)
	}
}

func TestAmostraComCincoContainersFazUmInsert(t *testing.T) {
	srv := servidorDeTeste(t)
	alvo := Target{ID: srv.ID, Host: srv.HostIP}

	var payload SysPayload
	for i := range 5 {
		id := "d10cont" + string(rune('a'+i))
		payload.PS = append(payload.PS, DockerPSPayload{DockerID: id, Name: id, State: "running", Status: "Up"})
		payload.Stats = append(payload.Stats, DockerStatsPayload{DockerID: id, CPUPercent: "1.00%", MemUsage: "1MiB / 1GiB"})
	}
	storeContainerMetrics(alvo, payload, map[string]string{})

	var linhas, transacoes int64
	err := database.DB.Raw(`
		SELECT COUNT(*), COUNT(DISTINCT m.xmin::text)
		FROM metric_containers m JOIN containers c ON c.id = m.container_id
		WHERE c.server_id = ?`, srv.ID).Row().Scan(&linhas, &transacoes)
	if err != nil {
		t.Fatalf("contar gravações: %v", err)
	}
	if linhas != 5 {
		t.Fatalf("linhas de métrica = %d, esperado 5", linhas)
	}
	if transacoes != 1 {
		t.Errorf("amostra com 5 containers gerou %d INSERTs em metric_containers, esperado 1", transacoes)
	}
}

func TestStoreHostMetricGravaRede(t *testing.T) {
	srv := servidorDeTeste(t)
	alvo := Target{ID: srv.ID, Host: srv.HostIP}

	rx, tx := 4096.0, 1024.0
	storeHostMetric(alvo, SysPayload{NetRxBps: &rx, NetTxBps: &tx}, 10)
	storeHostMetric(alvo, SysPayload{}, 10)

	var linhas []database.MetricServer
	database.DB.Where("server_id = ?", srv.ID).Order("id asc").Find(&linhas)
	if len(linhas) != 2 {
		t.Fatalf("amostras = %d, esperado 2", len(linhas))
	}
	if linhas[0].NetRxBps == nil || *linhas[0].NetRxBps != 4096 || linhas[0].NetTxBps == nil || *linhas[0].NetTxBps != 1024 {
		t.Errorf("rede gravada = (%v, %v), esperado (4096, 1024)", linhas[0].NetRxBps, linhas[0].NetTxBps)
	}
	if linhas[1].NetRxBps != nil || linhas[1].NetTxBps != nil {
		t.Errorf("amostra sem rede gravou (%v, %v); esperado NULL", linhas[1].NetRxBps, linhas[1].NetTxBps)
	}
}
