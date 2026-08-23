package api

import (
	"log"
	"net/http"
	"time"

	"github.com/joaov/vd_stats/internal/database"
)

type ContainerLiveStat struct {
	ServerID string  `json:"server_id"`
	DockerID string  `json:"docker_id"`
	Name     string  `json:"name"`
	Project  string  `json:"project"`
	State    string  `json:"state"`
	Status   string  `json:"status"`
	CPU      float64 `json:"cpu"`
	MemUsed  int64   `json:"mem_used"`
	MemLimit int64   `json:"mem_limit"`
}

type ServerLiveStat struct {
	ID             string   `json:"id"`
	HostIP         string   `json:"host_ip"`
	Name           string   `json:"name"`
	Uptime         float64  `json:"uptime"`
	DiskUsed       int64    `json:"disk_used"`
	DiskTotal      int64    `json:"disk_total"`
	CPU            float64  `json:"cpu"`
	MemUsed        int64    `json:"mem_used"`
	MemTotal       int64    `json:"mem_total"`
	Load1          float64  `json:"load1"`
	Online         bool     `json:"online"`
	SSHHandshakeMs *float64 `json:"ssh_handshake_ms"`

	// Usados pelo painel de suporte para separar estação de servidor e mostrar
	// o que o técnico precisa antes de ir até a máquina.
	Kind         string   `json:"kind"` // "ssh" | "agent"
	SiteID       *uint    `json:"site_id"`
	OS           string   `json:"os"`
	Platform     string   `json:"platform"`
	Arch         string   `json:"arch"`
	LastUser     string   `json:"last_user"`
	AgentVersion string   `json:"agent_version"`
	TemperatureC *float64 `json:"temperature_c"`
	CollectNginx bool     `json:"collect_nginx"`

	// Segundos que este host pode passar sem reportar antes de ser dado como
	// offline. Vai para o painel para a tela explicar por que um host está
	// offline em vez de só mostrar o rótulo.
	LiveWindowSec int `json:"live_window_sec"`
}

type LbStat struct {
	UpstreamAddr  string `json:"upstream_addr"`
	ServerName    string `json:"server_name"`
	Status        string `json:"status"`
	RequestsCount int    `json:"requests_count"`
}

type LiveResponse struct {
	Servers       []ServerLiveStat    `json:"servers"`
	Containers    []ContainerLiveStat `json:"containers"`
	LoadBalancing []LbStat            `json:"load_balancing"`
}

// Janela de busca da última métrica de cada host. É deliberadamente ampla: a
// decisão de "online" deixou de ser feita no SQL e passou a ser feita em Go,
// por host, porque cada fonte reporta num ritmo diferente. Serve só para não
// varrer a tabela inteira.
const metricLookback = "10 minutes"

// Container só é coletado pelo stream SSH, cujo ritmo é fixo (~2 s), então
// aqui a janela constante continua valendo.
const containerLiveWindow = "30 seconds"

// Janela de agregação do access log do Nginx exibida no painel.
const lbWindow = "5 seconds"

// liveMetricsHandler devolve o último estado conhecido de hosts, containers e
// balanceador — é o endpoint que o painel consulta em polling.
func liveMetricsHandler(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	scope, status := resolveScope(sess, r)
	if status != 0 {
		writeError(w, status, "site_id inválido ou fora do seu alcance")
		return
	}

	var servers []database.Server
	if err := scope.apply(database.DB.Order("name ASC")).Find(&servers).Error; err != nil {
		log.Printf("[API] erro ao listar servidores: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao ler servidores")
		return
	}

	serverMetrics, err := lastServerMetrics()
	if err != nil {
		log.Printf("[API] erro ao ler métricas de host: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao ler métricas")
		return
	}

	res := LiveResponse{
		Servers:       make([]ServerLiveStat, 0, len(servers)),
		Containers:    []ContainerLiveStat{},
		LoadBalancing: []LbStat{},
	}

	for _, s := range servers {
		stat := ServerLiveStat{
			ID: s.ID, HostIP: s.HostIP, Name: s.Name,
			Kind: s.Kind, SiteID: s.SiteID, OS: s.OS, Platform: s.Platform,
			Arch: s.Arch, LastUser: s.LastUser, AgentVersion: s.AgentVersion,
			CollectNginx: s.CollectNginx,
		}
		window := database.LiveWindowFor(s.ReportIntervalSec)
		stat.LiveWindowSec = int(window / time.Second)

		// Só preenche os valores quando a métrica ainda está dentro da janela
		// deste host: fora dela o número é histórico e exibi-lo faria a tela
		// mostrar CPU de dez minutos atrás ao lado do rótulo "offline".
		if m, ok := serverMetrics[s.ID]; ok && time.Since(m.Timestamp) <= window {
			stat.Uptime = m.UptimeSeconds
			stat.DiskUsed = m.DiskUsedBytes
			stat.DiskTotal = m.DiskTotalBytes
			stat.CPU = m.CPUUsagePercent
			stat.MemUsed = m.MemUsedBytes
			stat.MemTotal = m.MemTotalBytes
			stat.Load1 = m.LoadAvg1
			stat.SSHHandshakeMs = m.SSHHandshakeMs
			stat.TemperatureC = m.TemperatureC
			stat.Online = true
		}
		// Sem métrica recente o host continua aparecendo, mas como offline.
		res.Servers = append(res.Servers, stat)
	}

	// Containers pertencem aos servidores em escopo; sem esse filtro o painel
	// de uma unidade mostraria container de outra.
	inScope := make(map[string]bool, len(res.Servers))
	for _, s := range res.Servers {
		inScope[s.ID] = true
	}

	var containers []database.Container
	if err := database.DB.Order("name ASC").Find(&containers).Error; err != nil {
		log.Printf("[API] erro ao listar containers: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao ler containers")
		return
	}

	containerMetrics, err := lastContainerMetrics()
	if err != nil {
		log.Printf("[API] erro ao ler métricas de container: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao ler métricas")
		return
	}

	for _, c := range containers {
		if !inScope[c.ServerID] {
			continue
		}
		m, ok := containerMetrics[c.ID]
		if !ok {
			continue
		}
		res.Containers = append(res.Containers, ContainerLiveStat{
			ServerID: c.ServerID,
			DockerID: c.DockerID,
			Name:     c.Name,
			Project:  c.ProjectDir,
			State:    m.State,
			Status:   m.Status,
			CPU:      m.CPUUsagePercent,
			MemUsed:  m.MemUsedBytes,
			MemLimit: m.MemLimitBytes,
		})
	}

	// O agregado do balanceador usa o mesmo recorte do resto da resposta, pela
	// coluna site_id que a linha passou a carregar. Enquanto a tabela não tinha
	// unidade, a única saída era negar o agregado inteiro a quem é restrito — as
	// duas telas contavam histórias diferentes sobre o mesmo parque.
	lbTx := scope.apply(database.DB.Model(&database.MetricLoadBalancer{})).
		Select("upstream_addr, server_name, status, SUM(requests_count) AS requests_count").
		Where("timestamp >= NOW() - INTERVAL '" + lbWindow + "'").
		Group("upstream_addr, server_name, status")
	if err := lbTx.Scan(&res.LoadBalancing).Error; err != nil {
		log.Printf("[API] erro ao agregar load balancer: %v", err)
	}
	if res.LoadBalancing == nil {
		res.LoadBalancing = []LbStat{}
	}

	writeJSON(w, http.StatusOK, res)
}

func lastServerMetrics() (map[string]database.MetricServer, error) {
	var rows []database.MetricServer
	err := database.DB.Raw(`
		SELECT DISTINCT ON (server_id) *
		FROM metric_servers
		WHERE timestamp >= NOW() - INTERVAL '` + metricLookback + `'
		ORDER BY server_id, timestamp DESC
	`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	byServer := make(map[string]database.MetricServer, len(rows))
	for _, m := range rows {
		byServer[m.ServerID] = m
	}
	return byServer, nil
}

func lastContainerMetrics() (map[string]database.MetricContainer, error) {
	var rows []database.MetricContainer
	err := database.DB.Raw(`
		SELECT DISTINCT ON (container_id) *
		FROM metric_containers
		WHERE timestamp >= NOW() - INTERVAL '` + containerLiveWindow + `'
		ORDER BY container_id, timestamp DESC
	`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	byContainer := make(map[string]database.MetricContainer, len(rows))
	for _, m := range rows {
		byContainer[m.ContainerID] = m
	}
	return byContainer, nil
}
