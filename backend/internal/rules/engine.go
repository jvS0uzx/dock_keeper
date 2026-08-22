package rules

import (
	"fmt"
	"log"
	"time"

	"github.com/joaov/vd_stats/internal/alert"
	"github.com/joaov/vd_stats/internal/database"
)

// metricValue calcula o valor da métrica a partir da última leitura do host.
// Retorna (valor, ok=false) quando não há dado suficiente (ex: total zerado).
func metricValue(m database.MetricServer, metric string) (float64, bool) {
	switch metric {
	case "cpu":
		return m.CPUUsagePercent, true
	case "load":
		return m.LoadAvg1, true
	case "mem":
		if m.MemTotalBytes > 0 {
			return float64(m.MemUsedBytes) / float64(m.MemTotalBytes) * 100, true
		}
	case "disk":
		if m.DiskTotalBytes > 0 {
			return float64(m.DiskUsedBytes) / float64(m.DiskTotalBytes) * 100, true
		}
	}
	return 0, false
}

func violates(value float64, operator string, threshold float64) bool {
	switch operator {
	case ">":
		return value > threshold
	case "<":
		return value < threshold
	}
	return false
}

// StartEngine sobe uma goroutine que avalia as regras de alerta a cada tick.
func StartEngine(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			evaluate()
		}
	}()
}

func evaluate() {
	var rules []database.AlertRule
	if err := database.DB.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		log.Printf("[rules] erro ao carregar regras: %v", err)
		return
	}
	if len(rules) == 0 {
		return
	}

	// nomes dos servers para mensagem legível.
	var servers []database.Server
	database.DB.Find(&servers)
	nameByID := make(map[string]string, len(servers))
	for _, s := range servers {
		nameByID[s.ID] = s.Name
	}

	// última métrica de cada server nos últimos 60s.
	var latest []database.MetricServer
	database.DB.Raw(`
		SELECT DISTINCT ON (server_id) *
		FROM metric_servers
		WHERE timestamp >= NOW() - INTERVAL '60 seconds'
		ORDER BY server_id, timestamp DESC
	`).Scan(&latest)
	metricByServer := make(map[string]database.MetricServer, len(latest))
	for _, m := range latest {
		metricByServer[m.ServerID] = m
	}

	now := time.Now()
	for _, rule := range rules {
		var targets []string
		if rule.Target == "*" {
			for _, s := range servers {
				targets = append(targets, s.ID)
			}
		} else {
			targets = []string{rule.Target}
		}

		for _, serverID := range targets {
			m, ok := metricByServer[serverID]
			if !ok {
				// sem métrica recente: ignora este server.
				continue
			}
			value, ok := metricValue(m, rule.Metric)
			if !ok {
				continue
			}
			if !violates(value, rule.Operator, rule.Threshold) {
				continue
			}

			serverName := nameByID[serverID]
			if serverName == "" {
				serverName = serverID
			}
			msg := fmt.Sprintf(
				"[ALERTA] Regra %s: %s=%.2f %s %.2f em %s",
				rule.Name, rule.Metric, value, rule.Operator, rule.Threshold, serverName,
			)
			alert.Notify(fmt.Sprintf("rule:%d:%s", rule.ID, serverID), msg)

			fired := now
			database.DB.Model(&database.AlertRule{}).
				Where("id = ?", rule.ID).
				Update("last_fired", &fired)
		}
	}
}
