package api

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/metricas"
)

type historyPoint struct {
	Ts    time.Time `json:"ts"`
	Value float64   `json:"value"`
}

var historyRanges = map[string]time.Duration{
	"1h":  1 * time.Hour,
	"6h":  6 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
	"90d": 90 * 24 * time.Hour,
}

const maxCustomSpan = 400 * 24 * time.Hour

func historyWindow(q url.Values, now time.Time) (time.Time, time.Time, string) {
	rangeKey, from, to := q.Get("range"), q.Get("from"), q.Get("to")
	if from != "" && rangeKey != "" {
		return time.Time{}, time.Time{}, "use range ou from/to, não os dois"
	}
	if from == "" {
		if to != "" {
			return time.Time{}, time.Time{}, "to exige from"
		}
		if rangeKey == "" {
			rangeKey = "1h"
		}
		dur, ok := historyRanges[rangeKey]
		if !ok {
			return time.Time{}, time.Time{}, "range inválido"
		}
		return now.Add(-dur), now, ""
	}

	start, err := time.Parse(time.RFC3339, from)
	if err != nil {
		return time.Time{}, time.Time{}, "from inválido: use data RFC3339"
	}
	end := now
	if to != "" {
		if end, err = time.Parse(time.RFC3339, to); err != nil {
			return time.Time{}, time.Time{}, "to inválido: use data RFC3339"
		}
	}
	if !start.Before(end) {
		return time.Time{}, time.Time{}, "from precisa ser anterior a to"
	}
	if end.Sub(start) > maxCustomSpan {
		return time.Time{}, time.Time{}, "o período passa de 400 dias"
	}
	return start, end, ""
}

func serverMetricExpr(metric string) (string, bool) {
	m, ok := metricas.Buscar(metric)
	return m.SerieDoServidor, ok && m.SerieDoServidor != ""
}

func containerMetricExpr(metric string) (string, bool) {
	m, ok := metricas.Buscar(metric)
	return m.SerieDoContainer, ok && m.SerieDoContainer != ""
}

func bucketExpr(d time.Duration) string {
	switch {
	case d <= time.Hour:
		return "date_trunc('minute', timestamp)"
	case d <= 24*time.Hour:
		return "to_timestamp(floor(extract(epoch from timestamp) / 300) * 300)"
	default:
		return "date_trunc('hour', timestamp)"
	}
}

func HistoryHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	serverID := q.Get("server_id")
	if serverID == "" {
		writeError(w, http.StatusBadRequest, "server_id é obrigatório")
		return
	}

	start, end, invalid := historyWindow(q, time.Now())
	if invalid != "" {
		writeError(w, http.StatusBadRequest, invalid)
		return
	}
	dur := end.Sub(start)

	metric := q.Get("metric")
	containerID := q.Get("container_id")

	sess := sessionFrom(r)
	if !auth.HasGlobal(sess.Accesses) {
		var server database.Server
		if err := database.From(r.Context()).Where("id = ?", serverID).First(&server).Error; err != nil {
			writeError(w, http.StatusNotFound, "servidor não encontrado")
			return
		}
		if !auth.CanSeeSite(sess.Accesses, server.SiteID) {
			writeError(w, http.StatusForbidden, "este servidor está fora do seu alcance")
			return
		}
	}

	var table, valueExpr, filterCol, filterVal string
	if containerID != "" {
		expr, valid := containerMetricExpr(metric)
		if !valid {
			writeError(w, http.StatusBadRequest, "métrica inválida para container")
			return
		}
		table, valueExpr, filterCol, filterVal = "metric_containers", expr, "container_id", containerID
	} else {
		expr, valid := serverMetricExpr(metric)
		if !valid {
			writeError(w, http.StatusBadRequest, "métrica inválida")
			return
		}
		table, valueExpr, filterCol, filterVal = "metric_servers", expr, "server_id", serverID
	}

	_, temTendencia := trendMetricExpr(metric)
	bruta := database.RetentionDays("METRIC_RETENTION_DAYS", database.DefaultMetricRetentionDays)
	if bruta > 0 && dur > bruta && (containerID != "" || !temTendencia) {
		dias := int(bruta / (24 * time.Hour))
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"esta métrica só tem %d dias de histórico: não existe tendência de longo prazo para ela; escolha um período de até %d dias", dias, dias))
		return
	}

	if containerID == "" && dur > trendThreshold {
		if expr, ok := trendMetricExpr(metric); ok {
			serveFromTrend(w, r, serverID, expr, start, end)
			return
		}
	}

	sql := fmt.Sprintf(`
		SELECT %s AS ts, AVG(%s) AS value
		FROM %s
		WHERE %s = ? AND timestamp >= ? AND timestamp <= ? AND (%s) IS NOT NULL
		GROUP BY ts
		ORDER BY ts ASC
	`, bucketExpr(dur), valueExpr, table, filterCol, valueExpr)

	var points []historyPoint
	if err := database.From(r.Context()).Raw(sql, filterVal, start, end).Scan(&points).Error; err != nil {
		log.Printf("[History] erro na consulta: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao consultar o histórico")
		return
	}
	if points == nil {
		points = []historyPoint{}
	}

	writeJSON(w, http.StatusOK, points)
}

const trendThreshold = 24 * time.Hour

func trendMetricExpr(metric string) (string, bool) {
	m, ok := metricas.Buscar(metric)
	return m.TendenciaMedia, ok && m.TemTendencia()
}

func trendBucketExpr(d time.Duration) string {
	switch {
	case d <= 30*24*time.Hour:
		return "bucket"
	case d <= 90*24*time.Hour:
		return "to_timestamp(floor(extract(epoch from bucket) / 21600) * 21600)"
	default:
		return "to_timestamp(floor(extract(epoch from bucket) / 86400) * 86400)"
	}
}

func serveFromTrend(w http.ResponseWriter, r *http.Request, serverID, column string, start, end time.Time) {
	var points []historyPoint
	sql := fmt.Sprintf(`
		SELECT %s AS ts, AVG(%s) AS value
		FROM metric_server_trends
		WHERE server_id = ? AND bucket >= ? AND bucket <= ? AND %s IS NOT NULL
		GROUP BY ts
		ORDER BY ts ASC
	`, trendBucketExpr(end.Sub(start)), column, column)

	if err := database.From(r.Context()).Raw(sql, serverID, start, end).Scan(&points).Error; err != nil {
		log.Printf("[History] erro na consulta de trend: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao consultar o histórico")
		return
	}
	if points == nil {
		points = []historyPoint{}
	}
	writeJSON(w, http.StatusOK, points)
}
