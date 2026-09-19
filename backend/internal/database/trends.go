package database

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/safego"
)

var trendRetention = RetentionDays("TREND_RETENTION_DAYS", DefaultTrendRetentionDays)

const trendRollupWindow = 3 * time.Hour

func StartTrendWorker(interval time.Duration) <-chan struct{} {
	ready := make(chan struct{})
	var once sync.Once

	safego.Run(context.Background(), "database:trends", func(context.Context) {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		full := true
		for {
			rollupTrends(rollupSince(full))
			full = false

			once.Do(func() { close(ready) })

			pruneTrends()
			<-ticker.C
		}
	})

	return ready
}

func rollupSince(full bool) *time.Time {
	if full {
		return nil
	}
	since := time.Now().UTC().Add(-trendRollupWindow)
	return &since
}

func rollupSQL(since *time.Time) (string, []any) {
	where := "timestamp < date_trunc('hour', NOW())"
	args := []any{}
	if since != nil {
		where += " AND timestamp >= ?"
		args = append(args, *since)
	}

	return fmt.Sprintf(`
		INSERT INTO metric_server_trends (
			server_id, bucket,
			cpu_avg, cpu_max,
			mem_percent_avg, disk_percent_avg,
			load_avg1_avg, load_avg1_max,
			temperature_avg, temperature_max,
			net_rx_avg, net_tx_avg,
			rtt_avg, rtt_max,
			samples
		)
		SELECT
			server_id,
			date_trunc('hour', timestamp) AS bucket,
			AVG(cpu_usage_percent), MAX(cpu_usage_percent),
			AVG(mem_used_bytes::float8  / NULLIF(mem_total_bytes, 0)  * 100),
			AVG(disk_used_bytes::float8 / NULLIF(disk_total_bytes, 0) * 100),
			AVG(load_avg1), MAX(load_avg1),
			AVG(NULLIF(temperature_c, 0)), MAX(temperature_c),
			AVG(net_rx_bps), AVG(net_tx_bps),
			AVG(rtt_ms), MAX(rtt_ms),
			COUNT(*)
		FROM metric_servers
		WHERE %s
		GROUP BY server_id, bucket
		ON CONFLICT (server_id, bucket) DO UPDATE SET
			cpu_avg          = EXCLUDED.cpu_avg,
			cpu_max          = EXCLUDED.cpu_max,
			mem_percent_avg  = EXCLUDED.mem_percent_avg,
			disk_percent_avg = EXCLUDED.disk_percent_avg,
			load_avg1_avg    = EXCLUDED.load_avg1_avg,
			load_avg1_max    = EXCLUDED.load_avg1_max,
			temperature_avg  = EXCLUDED.temperature_avg,
			temperature_max  = EXCLUDED.temperature_max,
			net_rx_avg       = EXCLUDED.net_rx_avg,
			net_tx_avg       = EXCLUDED.net_tx_avg,
			rtt_avg          = EXCLUDED.rtt_avg,
			rtt_max          = EXCLUDED.rtt_max,
			samples          = EXCLUDED.samples
	`, where), args
}

func rollupTrends(since *time.Time) {
	sql, args := rollupSQL(since)

	res := DB.Exec(sql, args...)
	if res.Error != nil {
		log.Printf("[Trends] erro ao agregar: %v", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("[Trends] %d baldes horários consolidados", res.RowsAffected)
	}
}

func pruneTrends() {
	cutoff := time.Now().UTC().Add(-trendRetention)
	res := DB.Where("bucket < ?", cutoff).Delete(&MetricServerTrend{})
	if res.Error != nil {
		log.Printf("[Trends] erro ao podar: %v", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("[Trends] %d baldes antigos removidos", res.RowsAffected)
	}
}
