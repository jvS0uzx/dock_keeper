package api

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/joaov/vd_stats/internal/database"
)

// ingestPayload é o corpo enviado pelo agente de coleta (cmd/agent).
type ingestPayload struct {
	Hostname  string  `json:"hostname"`
	CPU       float64 `json:"cpu"`
	MemUsed   int64   `json:"mem_used"`
	MemTotal  int64   `json:"mem_total"`
	Load1     float64 `json:"load1"`
	DiskUsed  int64   `json:"disk_used"`
	DiskTotal int64   `json:"disk_total"`
	Uptime    float64 `json:"uptime"`
}

// IngestHandler recebe métricas via push de agentes (Kind="agent").
// Autentica pelo header X-Agent-Token contra a env AGENT_INGEST_TOKEN.
// Não deve ser registrado com CORS de escrita público sem cuidado; é uma rota máquina-a-máquina.
func IngestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	expected := os.Getenv("AGENT_INGEST_TOKEN")
	if expected == "" {
		// Sem token configurado a ingestão fica desabilitada por padrão (fail-closed).
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "ingest disabled"})
		return
	}
	if r.Header.Get("X-Agent-Token") != expected {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}

	var p ingestPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid body"})
		return
	}
	if p.Hostname == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "hostname required"})
		return
	}

	hostIP := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		hostIP = h
	}

	// Upsert do servidor por Name=hostname, marcado como agente push.
	server := database.Server{Name: p.Hostname}
	if err := database.DB.Where(database.Server{Name: p.Hostname}).
		Attrs(database.Server{HostIP: hostIP, Kind: "agent"}).
		FirstOrCreate(&server).Error; err != nil {
		log.Printf("[Ingest] erro no upsert do servidor %s: %v", p.Hostname, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "server upsert failed"})
		return
	}

	metric := database.MetricServer{
		ServerID:        server.ID,
		CPUUsagePercent: p.CPU,
		MemUsedBytes:    p.MemUsed,
		MemTotalBytes:   p.MemTotal,
		LoadAvg1:        p.Load1,
		DiskUsedBytes:   p.DiskUsed,
		DiskTotalBytes:  p.DiskTotal,
		UptimeSeconds:   p.Uptime,
		Timestamp:       time.Now().UTC(),
	}
	if err := database.DB.Create(&metric).Error; err != nil {
		log.Printf("[Ingest] erro ao inserir métrica de %s: %v", p.Hostname, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "metric insert failed"})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
