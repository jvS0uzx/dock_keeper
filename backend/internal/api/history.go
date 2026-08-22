package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/joaov/vd_stats/internal/database"
)

type historyPoint struct {
	Ts    time.Time `json:"ts"`
	Value float64   `json:"value"`
}

// mapa range -> duração. Define quais janelas são aceitas.
var historyRanges = map[string]time.Duration{
	"1h":  1 * time.Hour,
	"6h":  6 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
}

// expressão SQL da métrica para host (metric_servers).
func serverMetricExpr(metric string) (string, bool) {
	switch metric {
	case "cpu":
		return "cpu_usage_percent", true
	case "mem":
		return "mem_used_bytes::float8 / NULLIF(mem_total_bytes, 0) * 100", true
	case "disk":
		return "disk_used_bytes::float8 / NULLIF(disk_total_bytes, 0) * 100", true
	case "load":
		return "load_avg1", true
	case "latency":
		return "ping_latency_ms", true
	}
	return "", false
}

// expressão SQL da métrica para container (metric_containers).
func containerMetricExpr(metric string) (string, bool) {
	switch metric {
	case "cpu":
		return "cpu_usage_percent", true
	case "mem":
		return "mem_used_bytes::float8 / NULLIF(mem_limit_bytes, 0) * 100", true
	}
	return "", false
}

// bucketExpr escolhe o passo de agregação conforme a janela para não
// devolver milhares de pontos: <=1h por minuto, <=24h por 5min, >24h por hora.
func bucketExpr(d time.Duration) string {
	switch {
	case d <= time.Hour:
		return "date_trunc('minute', timestamp)"
	case d <= 24*time.Hour:
		// date_trunc não tem passo de 5min; alinha pelo epoch.
		return "to_timestamp(floor(extract(epoch from timestamp) / 300) * 300)"
	default:
		return "date_trunc('hour', timestamp)"
	}
}

// HistoryHandler devolve a série temporal agregada (downsampled) de uma métrica.
// Params: server_id (obrigatório), metric, range (default 1h), container_id (opcional).
func HistoryHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	serverID := q.Get("server_id")
	if serverID == "" {
		http.Error(w, "server_id required", http.StatusBadRequest)
		return
	}

	rangeKey := q.Get("range")
	if rangeKey == "" {
		rangeKey = "1h"
	}
	dur, ok := historyRanges[rangeKey]
	if !ok {
		http.Error(w, "invalid range", http.StatusBadRequest)
		return
	}

	metric := q.Get("metric")
	containerID := q.Get("container_id")
	cutoff := time.Now().Add(-dur)

	var table, valueExpr, filterCol, filterVal string
	if containerID != "" {
		expr, valid := containerMetricExpr(metric)
		if !valid {
			http.Error(w, "invalid metric for container", http.StatusBadRequest)
			return
		}
		table, valueExpr, filterCol, filterVal = "metric_containers", expr, "container_id", containerID
	} else {
		expr, valid := serverMetricExpr(metric)
		if !valid {
			http.Error(w, "invalid metric", http.StatusBadRequest)
			return
		}
		table, valueExpr, filterCol, filterVal = "metric_servers", expr, "server_id", serverID
	}

	// bucket e valueExpr vêm de whitelist; filtros usam placeholders.
	sql := fmt.Sprintf(`
		SELECT %s AS ts, AVG(%s) AS value
		FROM %s
		WHERE %s = ? AND timestamp >= ?
		GROUP BY ts
		ORDER BY ts ASC
	`, bucketExpr(dur), valueExpr, table, filterCol)

	var points []historyPoint
	if err := database.DB.Raw(sql, filterVal, cutoff).Scan(&points).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if points == nil {
		points = []historyPoint{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(points)
}
