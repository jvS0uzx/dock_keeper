// Command agent é um coletor cross-platform que roda em qualquer host
// (Linux/macOS/Windows) e faz push das métricas do sistema para o painel
// vd_stats via POST /api/ingest/metrics.
package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

type metricsPayload struct {
	Hostname  string  `json:"hostname"`
	CPU       float64 `json:"cpu"`
	MemUsed   int64   `json:"mem_used"`
	MemTotal  int64   `json:"mem_total"`
	Load1     float64 `json:"load1"`
	DiskUsed  int64   `json:"disk_used"`
	DiskTotal int64   `json:"disk_total"`
	Uptime    float64 `json:"uptime"`
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	serverURL := os.Getenv("AGENT_SERVER_URL")
	if serverURL == "" {
		log.Fatal("[Agent] AGENT_SERVER_URL não definido")
	}
	token := os.Getenv("AGENT_TOKEN")
	if token == "" {
		log.Fatal("[Agent] AGENT_TOKEN não definido")
	}

	defaultHost, _ := os.Hostname()
	hostname := getenv("AGENT_HOSTNAME", defaultHost)

	interval := 5
	if raw := os.Getenv("AGENT_INTERVAL"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			interval = n
		}
	}

	endpoint := serverURL + "/api/ingest/metrics"
	client := &http.Client{Timeout: 10 * time.Second}

	log.Printf("[Agent] iniciando: host=%s destino=%s intervalo=%ds", hostname, endpoint, interval)

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		payload := collect(hostname)
		if err := push(client, endpoint, token, payload); err != nil {
			log.Printf("[Agent] erro no envio: %v", err)
		} else {
			log.Printf("[Agent] métricas enviadas (cpu=%.1f%% mem=%d/%d load=%.2f)", payload.CPU, payload.MemUsed, payload.MemTotal, payload.Load1)
		}
		<-ticker.C
	}
}

func collect(hostname string) metricsPayload {
	p := metricsPayload{Hostname: hostname}

	// CPU% agregado. Percent(0,...) mede desde a última chamada; com intervalo
	// pequeno passamos uma janela curta de amostragem para ter um valor útil.
	if pcts, err := cpu.Percent(500*time.Millisecond, false); err == nil && len(pcts) > 0 {
		p.CPU = pcts[0]
	} else if err != nil {
		log.Printf("[Agent] cpu indisponível: %v", err)
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		p.MemUsed = int64(vm.Used)
		p.MemTotal = int64(vm.Total)
	} else {
		log.Printf("[Agent] mem indisponível: %v", err)
	}

	// Load average não existe no Windows; tratamos o erro e mandamos 0.
	if avg, err := load.Avg(); err == nil {
		p.Load1 = avg.Load1
	} else {
		p.Load1 = 0
	}

	if du, err := disk.Usage("/"); err == nil {
		p.DiskUsed = int64(du.Used)
		p.DiskTotal = int64(du.Total)
	} else {
		log.Printf("[Agent] disco indisponível: %v", err)
	}

	if up, err := host.Uptime(); err == nil {
		p.Uptime = float64(up)
	} else {
		log.Printf("[Agent] uptime indisponível: %v", err)
	}

	return p
}

func push(client *http.Client, endpoint, token string, payload metricsPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", token)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &httpError{status: resp.StatusCode}
	}
	return nil
}

type httpError struct{ status int }

func (e *httpError) Error() string {
	return "resposta HTTP inesperada: " + strconv.Itoa(e.status)
}
