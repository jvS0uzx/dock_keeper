package api

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
	"gorm.io/gorm"
)

type ingestPayload struct {
	Hostname  string   `json:"hostname"`
	CPU       *float64 `json:"cpu"`
	MemUsed   int64    `json:"mem_used"`
	MemTotal  int64    `json:"mem_total"`
	Load1     *float64 `json:"load1"`
	DiskUsed  int64    `json:"disk_used"`
	DiskTotal int64    `json:"disk_total"`
	Uptime    float64  `json:"uptime"`

	TemperatureC *float64 `json:"temperature_c"`

	NetRxBps *float64 `json:"net_rx_bps"`
	NetTxBps *float64 `json:"net_tx_bps"`

	OS           string `json:"os"`
	Platform     string `json:"platform"`
	Arch         string `json:"arch"`
	LoggedUser   string `json:"logged_user"`
	SiteCode     string `json:"site_code"`
	AgentVersion string `json:"agent_version"`

	MachineID string `json:"machine_id"`

	ReportIntervalSec int `json:"report_interval_sec"`
}

func temperatureOf(p ingestPayload) *float64 {
	if p.TemperatureC == nil || *p.TemperatureC == 0 {
		return nil
	}
	return p.TemperatureC
}

func rateOf(v *float64) *float64 {
	if v == nil || *v < 0 {
		return nil
	}
	return v
}

func IngestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cred, err := authenticateDevice(r)
	if err != nil {
		refuseDeviceAuth(w, r, err, "ingest.legacy_token_disabled")
		return
	}
	if !cred.allowsKind(kindAgent) {
		refuseDeviceKind(w, r, cred, "ingest.kind_mismatch", kindAgent, "métrica")
		return
	}
	if !limitarTaxa(w, chaveDeIngestao(r, cred), tetoDeIngestao()) {
		return
	}

	var p ingestPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	p.Hostname = strings.TrimSpace(p.Hostname)
	if p.Hostname == "" {
		writeError(w, http.StatusBadRequest, "hostname required")
		return
	}
	hostIP := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		hostIP = h
	}

	declarada := siteOfAgent(p)
	if !cred.siteMatches(declarada) {
		auditIngestSiteMismatch(cred, p, declarada)
		writeError(w, http.StatusConflict, "unidade declarada não confere com a credencial do dispositivo")
		return
	}
	siteID := cred.SiteID
	if siteID == nil {
		siteID = declarada
	}

	server, err := findOrCreateAgentServer(p.Hostname, p.MachineID, hostIP, siteID)
	if err != nil {
		log.Printf("[Ingest] erro no upsert do servidor %s: %v", p.Hostname, err)
		writeError(w, http.StatusInternalServerError, "server upsert failed")
		return
	}

	if err := database.DB.Model(&server).Updates(hostFacts(p, hostIP, siteID)).Error; err != nil {
		log.Printf("[Ingest] erro ao atualizar os dados de %s: %v", p.Hostname, err)
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
		TemperatureC:    temperatureOf(p),
		NetRxBps:        rateOf(p.NetRxBps),
		NetTxBps:        rateOf(p.NetTxBps),
		Timestamp:       time.Now().UTC(),
	}
	if err := database.DB.Create(&metric).Error; err != nil {
		log.Printf("[Ingest] erro ao inserir métrica de %s: %v", p.Hostname, err)
		writeError(w, http.StatusInternalServerError, "metric insert failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func siteOfAgent(p ingestPayload) *uint {
	code := strings.ToLower(strings.TrimSpace(p.SiteCode))
	if code == "" {
		return nil
	}

	var site database.Site
	if err := database.DB.Where("code = ?", code).First(&site).Error; err != nil {
		log.Printf("[Ingest] unidade %q informada por %s não existe", code, p.Hostname)
		return nil
	}
	return &site.ID
}

func findOrCreateAgentServer(hostname, machineID, hostIP string, siteID *uint) (database.Server, error) {
	var server database.Server

	if machineID != "" {
		q := database.DB.Where("machine_id = ?", machineID)
		if siteID != nil {
			q = q.Where("site_id = ?", *siteID)
		} else {
			q = q.Where("site_id IS NULL")
		}
		err := q.First(&server).Error
		if err == nil {
			return server, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return server, err
		}
	}

	q := database.DB.Where("name = ?", hostname)
	if siteID != nil {
		q = q.Where("site_id = ?", *siteID)
	} else {
		q = q.Where("site_id IS NULL")
	}

	err := q.First(&server).Error
	if err == nil {
		return server, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return server, err
	}

	if siteID != nil {
		err = database.DB.Where("name = ? AND site_id IS NULL", hostname).First(&server).Error
		if err == nil {
			return server, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return server, err
		}
	}

	server = database.Server{
		Name: hostname, HostIP: hostIP, Kind: "agent", SiteID: siteID, MachineID: machineID,
	}
	return server, database.DB.Create(&server).Error
}

func hostFacts(p ingestPayload, hostIP string, siteID *uint) map[string]any {
	facts := map[string]any{"host_ip": hostIP, "kind": "agent"}

	if p.ReportIntervalSec > 0 {
		facts["report_interval_sec"] = p.ReportIntervalSec
	}

	for column, value := range map[string]string{
		"os":            p.OS,
		"platform":      p.Platform,
		"arch":          p.Arch,
		"agent_version": p.AgentVersion,
		"last_user":     p.LoggedUser,
	} {
		if strings.TrimSpace(value) != "" {
			facts[column] = value
		}
	}

	if siteID != nil {
		facts["site_id"] = *siteID
	}
	return facts
}
