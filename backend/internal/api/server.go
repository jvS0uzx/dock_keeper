package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/joaov/vd_stats/internal/database"
	"github.com/joaov/vd_stats/internal/network"
	"github.com/joaov/vd_stats/internal/ssh"
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
	ID            string  `json:"id"`
	HostIP        string  `json:"host_ip"`
	Name          string  `json:"name"`
	Uptime        float64 `json:"uptime"`
	DiskUsed      int64   `json:"disk_used"`
	DiskTotal     int64   `json:"disk_total"`
	CPU           float64 `json:"cpu"`
	MemUsed       int64   `json:"mem_used"`
	MemTotal      int64   `json:"mem_total"`
	Load1         float64 `json:"load1"`
	Online        bool    `json:"online"`
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
			if ok {
				res.Servers = append(res.Servers, ServerLiveStat{
					ID:            s.ID,
					HostIP:        s.HostIP,
					Name:          s.Name,
					Uptime:        lastSrv.UptimeSeconds,
					DiskUsed:      lastSrv.DiskUsedBytes,
					DiskTotal:     lastSrv.DiskTotalBytes,
					CPU:           lastSrv.CPUUsagePercent,
					MemUsed:       lastSrv.MemUsedBytes,
					MemTotal:      lastSrv.MemTotalBytes,
					Load1:         lastSrv.LoadAvg1,
					Online:        true,
					PingLatencyMs: lastSrv.PingLatencyMs,
				})
			} else {
				// Sem métrica recente: aparece na UI, mas marcado offline.
				res.Servers = append(res.Servers, ServerLiveStat{
					ID:     s.ID,
					HostIP: s.HostIP,
					Name:   s.Name,
					Online: false,
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
					Project:  c.ProjectDir,
					State:    lastMetric.State,
					Status:   lastMetric.Status,
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

	// Fase 4 — ações de gestão em containers (start/stop/restart) via SSH.
	mux.HandleFunc("/api/containers/action", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ServerID      string `json:"server_id"`
			ContainerName string `json:"container_name"`
			Action        string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var server database.Server
		if err := database.DB.Where("id = ?", req.ServerID).First(&server).Error; err != nil {
			http.Error(w, "Servidor nao encontrado", http.StatusNotFound)
			return
		}

		out, err := ssh.RunContainerAction(server.HostIP, server.User, os.Getenv("SSH_KEY_PATH"), req.Action, req.ContainerName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "output": out})
	}))

	mux.HandleFunc("/api/security/radar", withCORS(func(w http.ResponseWriter, r *http.Request) {
		serverID := r.URL.Query().Get("server_id")
		if serverID == "" {
			http.Error(w, "server_id is required", http.StatusBadRequest)
			return
		}
		var server database.Server
		if err := database.DB.Where("id = ?", serverID).First(&server).Error; err != nil {
			http.Error(w, "Servidor nao encontrado", http.StatusNotFound)
			return
		}

		ports, err := ssh.GetRadarPorts(server.HostIP, server.User, os.Getenv("SSH_KEY_PATH"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(ports)
	}))

	mux.HandleFunc("/api/security/authlog/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.WriteHeader(http.StatusOK)
			return
		}

		serverID := r.URL.Query().Get("server_id")
		if serverID == "" {
			http.Error(w, "server_id is required", http.StatusBadRequest)
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
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		err := ssh.StreamAuthLogs(r.Context(), server.HostIP, server.User, os.Getenv("SSH_KEY_PATH"), w, flusher)
		if err != nil {
			log.Printf("Erro no stream de auth logs: %v", err)
		}
	})

	mux.HandleFunc("/api/ssl/domains", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			var domains []database.Domain
			database.DB.Find(&domains)
			json.NewEncoder(w).Encode(domains)
			return
		}
		if r.Method == "POST" {
			var req struct {
				Domain   string `json:"domain"`
				ServerID string `json:"server_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				domain := database.Domain{Name: req.Domain, ServerID: req.ServerID}
				database.DB.Create(&domain)
				// Checa na hora para o domínio já aparecer com status, sem esperar o worker.
				go network.CheckAndStore(domain)
				json.NewEncoder(w).Encode(domain)
			}
			return
		}
		if r.Method == "DELETE" {
			id := r.URL.Query().Get("id")
			database.DB.Where("id = ?", id).Delete(&database.Domain{})
			w.WriteHeader(http.StatusOK)
			return
		}
	}))

	mux.HandleFunc("/api/ssl/check", withCORS(func(w http.ResponseWriter, r *http.Request) {
		domain := r.URL.Query().Get("domain")
		if domain == "" {
			http.Error(w, "domain is required", http.StatusBadRequest)
			return
		}

		info := network.CheckSSL(domain)
		json.NewEncoder(w).Encode(info)
	}))

	// Recheca 1 domínio cadastrado agora, persiste e devolve o registro atualizado.
	mux.HandleFunc("/api/ssl/recheck", withCORS(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		var domain database.Domain
		if err := database.DB.Where("id = ?", id).First(&domain).Error; err != nil {
			http.Error(w, "Dominio nao encontrado", http.StatusNotFound)
			return
		}
		updated := network.CheckAndStore(domain)
		json.NewEncoder(w).Encode(updated)
	}))

	// Dispara a varredura de todos os domínios em background.
	mux.HandleFunc("/api/ssl/recheck-all", withCORS(func(w http.ResponseWriter, r *http.Request) {
		go network.CheckAllDomains()
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "checking"})
	}))

	log.Printf("[RealTime] Servidor da API rodando em http://localhost%s", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Erro crítico na API HTTP: %v", err)
	}
}
