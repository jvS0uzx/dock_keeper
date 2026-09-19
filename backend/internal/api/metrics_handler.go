package api

import (
	"log"
	"net/http"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
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
	CPU            *float64 `json:"cpu"`
	MemUsed        int64    `json:"mem_used"`
	MemTotal       int64    `json:"mem_total"`
	Load1          *float64 `json:"load1"`
	Online         bool     `json:"online"`
	SSHHandshakeMs *float64 `json:"ssh_handshake_ms"`

	Kind         string   `json:"kind"`
	SiteID       *uint    `json:"site_id"`
	OS           string   `json:"os"`
	Platform     string   `json:"platform"`
	Arch         string   `json:"arch"`
	LastUser     string   `json:"last_user"`
	AgentVersion string   `json:"agent_version"`
	TemperatureC *float64 `json:"temperature_c"`
	NetRxBps     *float64 `json:"net_rx_bps"`
	NetTxBps     *float64 `json:"net_tx_bps"`
	RTTMs        *float64 `json:"rtt_ms"`
	CollectNginx bool     `json:"collect_nginx"`

	LiveWindowSec int `json:"live_window_sec"`
}

type LbStat struct {
	UpstreamAddr  string `json:"upstream_addr"`
	ServerName    string `json:"server_name"`
	Status        string `json:"status"`
	RequestsCount int    `json:"requests_count"`
	ServerID      string `json:"server_id"`
}

type LiveResponse struct {
	Servers       []ServerLiveStat    `json:"servers"`
	Containers    []ContainerLiveStat `json:"containers"`
	LoadBalancing []LbStat            `json:"load_balancing"`
}

const metricLookback = "10 minutes"

const containerLiveWindow = "30 seconds"

const lbWindow = "5 seconds"

func liveMetricsHandler(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	scope, status := resolveScope(sess, r)
	if status != 0 {
		writeError(w, status, "site_id inválido ou fora do seu alcance")
		return
	}

	var servers []database.Server
	if err := scope.apply(database.From(r.Context()).Order("name ASC")).Find(&servers).Error; err != nil {
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
			stat.NetRxBps = m.NetRxBps
			stat.NetTxBps = m.NetTxBps
			stat.RTTMs = m.RTTMs
			stat.Online = true
		}
		res.Servers = append(res.Servers, stat)
	}

	inScope := make(map[string]bool, len(res.Servers))
	for _, s := range res.Servers {
		inScope[s.ID] = true
	}

	var containers []database.Container
	if err := database.From(r.Context()).Order("name ASC").Find(&containers).Error; err != nil {
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

	lbTx := scope.apply(database.From(r.Context()).Model(&database.MetricLoadBalancer{})).
		Select("upstream_addr, server_name, status, SUM(requests_count) AS requests_count, COALESCE(server_id::text, '') AS server_id").
		Where("timestamp >= NOW() - INTERVAL '" + lbWindow + "'").
		Group("upstream_addr, server_name, status, COALESCE(server_id::text, '')")
	if err := lbTx.Scan(&res.LoadBalancing).Error; err != nil {
		log.Printf("[API] erro ao agregar load balancer: %v", err)
	}
	if res.LoadBalancing == nil {
		res.LoadBalancing = []LbStat{}
	}

	writeJSON(w, http.StatusOK, res)
}

func consultaUltimasMetricas() string {
	return `
		SELECT m.*
		FROM servers s
		JOIN LATERAL (
			SELECT *
			FROM metric_servers
			WHERE server_id = s.id
			  AND timestamp >= NOW() - INTERVAL '` + metricLookback + `'
			ORDER BY timestamp DESC
			LIMIT 1
		) m ON TRUE
		WHERE s.deleted_at IS NULL
	`
}

func lastServerMetrics() (map[string]database.MetricServer, error) {
	var rows []database.MetricServer
	err := database.DB.Raw(consultaUltimasMetricas()).Scan(&rows).Error
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
