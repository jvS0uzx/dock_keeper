package api

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/joaov/vd_stats/internal/auth"
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
	case "temperature":
		// Faltava aqui: trendMetricExpr aceitava "temperature", mas a validação
		// passa antes por esta função, então o gráfico de temperatura devolvia
		// 400 em qualquer janela e nunca chegou a ser exibido. Só passou a
		// valer a pena corrigir agora que o stream SSH também mede temperatura
		// (achado 5) — antes só as estações tinham o dado.
		return "temperature_c", true
	case "latency":
		// A chave da API continua "latency" para não quebrar links e o painel
		// já publicado; a coluna e o rótulo é que mudaram de nome. O número é
		// o handshake SSH, não RTT — ver MetricServer.SSHHandshakeMs.
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
		writeError(w, http.StatusBadRequest, "server_id é obrigatório")
		return
	}

	rangeKey := q.Get("range")
	if rangeKey == "" {
		rangeKey = "1h"
	}
	dur, ok := historyRanges[rangeKey]
	if !ok {
		writeError(w, http.StatusBadRequest, "range inválido")
		return
	}

	metric := q.Get("metric")
	containerID := q.Get("container_id")
	cutoff := time.Now().Add(-dur)

	// Recorte por unidade: a série é endereçada por server_id, então a
	// visibilidade é a do servidor dono dela.
	sess := sessionFrom(r)
	if !auth.HasGlobal(sess.Accesses) {
		var server database.Server
		if err := database.DB.Where("id = ?", serverID).First(&server).Error; err != nil {
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

	// Janelas longas leem a trend horária em vez do histórico bruto: 24 linhas
	// por dia por host contra uma a cada 1-2 segundos.
	if containerID == "" && dur > trendThreshold {
		if expr, ok := trendMetricExpr(metric); ok {
			serveFromTrend(w, serverID, expr, cutoff)
			return
		}
	}

	// bucket e valueExpr vêm de whitelist; filtros usam placeholders.
	//
	// O IS NOT NULL descarta a amostra que a fonte não mediu. Temperatura e
	// handshake SSH passaram a ser nuláveis (achado 5 do QA) e um balde inteiro
	// sem medição faria AVG devolver NULL, que não entra num float64. Omitir o
	// ponto é também o mais honesto: a série fica com um buraco em vez de uma
	// linha no chão fingindo leitura.
	sql := fmt.Sprintf(`
		SELECT %s AS ts, AVG(%s) AS value
		FROM %s
		WHERE %s = ? AND timestamp >= ? AND (%s) IS NOT NULL
		GROUP BY ts
		ORDER BY ts ASC
	`, bucketExpr(dur), valueExpr, table, filterCol, valueExpr)

	var points []historyPoint
	if err := database.DB.Raw(sql, filterVal, cutoff).Scan(&points).Error; err != nil {
		log.Printf("[History] erro na consulta: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao consultar o histórico")
		return
	}
	if points == nil {
		points = []historyPoint{}
	}

	writeJSON(w, http.StatusOK, points)
}

// Acima desta janela a consulta passa a ler a trend agregada.
const trendThreshold = 24 * time.Hour

// trendMetricExpr mapeia a métrica para a coluna já agregada.
func trendMetricExpr(metric string) (string, bool) {
	switch metric {
	case "cpu":
		return "cpu_avg", true
	case "mem":
		return "mem_percent_avg", true
	case "disk":
		return "disk_percent_avg", true
	case "load":
		return "load_avg1_avg", true
	case "temperature":
		return "temperature_avg", true
	}
	// latency não é agregada: só interessa ao vivo.
	return "", false
}

func serveFromTrend(w http.ResponseWriter, serverID, column string, cutoff time.Time) {
	var points []historyPoint
	// Mesma razão do caminho bruto: temperature_avg é nulo na hora em que
	// nenhum host da amostra tinha sensor, e NULL não cabe num float64.
	sql := fmt.Sprintf(`
		SELECT bucket AS ts, %s AS value
		FROM metric_server_trends
		WHERE server_id = ? AND bucket >= ? AND %s IS NOT NULL
		ORDER BY bucket ASC
	`, column, column)

	if err := database.DB.Raw(sql, serverID, cutoff).Scan(&points).Error; err != nil {
		log.Printf("[History] erro na consulta de trend: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao consultar o histórico")
		return
	}
	if points == nil {
		points = []historyPoint{}
	}
	writeJSON(w, http.StatusOK, points)
}
