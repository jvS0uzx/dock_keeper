package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/joaov/vd_stats/internal/database"
	"github.com/joaov/vd_stats/internal/ssh"
)

type ContainerLiveStat struct {
	ServerID string  `json:"server_id"`
	DockerID string  `json:"docker_id"`
	Name     string  `json:"name"`
	CPU      float64 `json:"cpu"`
	MemUsed  int64   `json:"mem_used"`
	MemLimit int64   `json:"mem_limit"`
}

type ServerLiveStat struct {
	ID            string  `json:"id"`
	HostIP        string  `json:"host_ip"`
	Name          string  `json:"name"`
	Uptime        float64 `json:"uptime"`
	DiskUsed      int64   `json:"disk_used"`
	DiskTotal     int64   `json:"disk_total"`
	PingLatencyMs float64 `json:"latency_ms"`
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

type ServerCreateRequest struct {
	HostIP string `json:"host_ip"`
	Name   string `json:"name"`
	User   string `json:"user"`
}

func StartServer(port string) {
	mux := http.NewServeMux()

	withCORS := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			h(w, r)
		}
	}

	mux.HandleFunc("/api/servers", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			var servers []database.Server
			database.DB.Find(&servers)
			json.NewEncoder(w).Encode(servers)
			return
		}

		if r.Method == "POST" {
			var req ServerCreateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req.User == "" {
				req.User = "root"
			}
			var server database.Server
			database.DB.Where("host_ip = ?", req.HostIP).Assign(database.Server{
				Name: req.Name,
				User: req.User,
			}).FirstOrCreate(&server, database.Server{
				HostIP: req.HostIP,
			})
			sshKey := os.Getenv("SSH_KEY_PATH")
			ssh.Manager.Start(server.ID, server.Name, server.HostIP, server.User, sshKey)
			json.NewEncoder(w).Encode(server)
			return
		}

		if r.Method == "DELETE" {
			id := r.URL.Query().Get("id")
			if id == "" {
				http.Error(w, "ID required", http.StatusBadRequest)
				return
			}
			ssh.Manager.Stop(id)
			database.DB.Where("id = ?", id).Delete(&database.Server{})
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}))

	mux.HandleFunc("/api/metrics/live", withCORS(func(w http.ResponseWriter, r *http.Request) {
		var servers []database.Server
		database.DB.Find(&servers)

		var lastServerMetrics []database.MetricServer
		database.DB.Raw(`
			SELECT DISTINCT ON (server_id) *
			FROM metric_servers
			WHERE timestamp >= NOW() - INTERVAL '30 seconds'
			ORDER BY server_id, timestamp DESC
		`).Scan(&lastServerMetrics)

		serverMetricsMap := make(map[string]database.MetricServer)
		for _, m := range lastServerMetrics {
			serverMetricsMap[m.ServerID] = m
		}

		var res LiveResponse
		for _, s := range servers {
			lastSrv, ok := serverMetricsMap[s.ID]
			if ok || s.Name == "Load Balancer" {
				res.Servers = append(res.Servers, ServerLiveStat{
					ID:            s.ID,
					HostIP:        s.HostIP,
					Name:          s.Name,
					Uptime:        lastSrv.UptimeSeconds,
					DiskUsed:      lastSrv.DiskUsedBytes,
					DiskTotal:     lastSrv.DiskTotalBytes,
					PingLatencyMs: lastSrv.PingLatencyMs,
				})
			} else {
				// Show servers even if they don't have recent metrics (so they appear in the UI)
				res.Servers = append(res.Servers, ServerLiveStat{
					ID:            s.ID,
					HostIP:        s.HostIP,
					Name:          s.Name,
					Uptime:        0,
				})
			}
		}

		var containers []database.Container
		database.DB.Order("name ASC").Find(&containers)

		var lastContainerMetrics []database.MetricContainer
		database.DB.Raw(`
			SELECT DISTINCT ON (container_id) *
			FROM metric_containers
			WHERE timestamp >= NOW() - INTERVAL '30 seconds'
			ORDER BY container_id, timestamp DESC
		`).Scan(&lastContainerMetrics)

		containerMetricsMap := make(map[string]database.MetricContainer)
		for _, m := range lastContainerMetrics {
			containerMetricsMap[m.ContainerID] = m
		}

		for _, c := range containers {
			lastMetric, ok := containerMetricsMap[c.ID]
			if ok {
				res.Containers = append(res.Containers, ContainerLiveStat{
					ServerID: c.ServerID,
					DockerID: c.DockerID,
					Name:     c.Name,
					CPU:      lastMetric.CPUUsagePercent,
					MemUsed:  lastMetric.MemUsedBytes,
					MemLimit: lastMetric.MemLimitBytes,
				})
			}
		}

		database.DB.Raw(`
			SELECT upstream_addr, server_name, status, SUM(requests_count) as requests_count
			FROM metric_load_balancers
			WHERE timestamp >= NOW() - INTERVAL '5 seconds'
			GROUP BY upstream_addr, server_name, status
		`).Scan(&res.LoadBalancing)

		json.NewEncoder(w).Encode(res)
	}))

	mux.HandleFunc("/api/containers/logs/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.WriteHeader(http.StatusOK)
			return
		}

		serverID := r.URL.Query().Get("server_id")
		containerName := r.URL.Query().Get("container_name")

		if serverID == "" || containerName == "" {
			http.Error(w, "server_id e container_name sao obrigatorios", http.StatusBadRequest)
			return
		}

		var server database.Server
		if err := database.DB.Where("id = ?", serverID).First(&server).Error; err != nil {
			http.Error(w, "Servidor nao encontrado", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
			return
		}

		sshKey := os.Getenv("SSH_KEY_PATH")
		ctx := r.Context()
		
		err := ssh.StreamDockerLogs(ctx, server.HostIP, server.User, sshKey, containerName, w, flusher)
		if err != nil {
			log.Printf("Erro no stream de logs: %v", err)
		}
	})

	log.Printf("[RealTime] Servidor da API rodando em http://localhost%s", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Erro crítico na API HTTP: %v", err)
	}
}
