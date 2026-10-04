package database

import (
	"context"
	"log"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/config"

	"github.com/jvS0uzx/dock_keeper/internal/safego"
)

func StartRetentionWorker(ctx context.Context, maxAge, interval time.Duration, trendsReady <-chan struct{}) {
	auditMaxAge := time.Duration(config.Inteiro("AUDIT_RETENTION_DAYS", defaultAuditRetentionDays)) * 24 * time.Hour

	safego.Run(ctx, "database:retencao", func(ctx context.Context) {
		if trendsReady != nil {
			select {
			case <-trendsReady:
			case <-ctx.Done():
				return
			}
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			prune(ctx, maxAge, auditMaxAge)
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	})
}

const defaultAuditRetentionDays = 365

const defaultAlertRetentionDays = 90

const defaultAddressRetentionDays = 30

const defaultNetworkMetricRetention = 72 * time.Hour

func PodarMetricasDeInterface(ctx context.Context, maxAge time.Duration) {
	cutoff := time.Now().UTC().Add(-maxAge)
	n, err := pruneBatched(ctx, dbExec, "metric_network_interfaces", "ts", cutoff, pruneBatchPause)
	if err != nil {
		log.Printf("[Retention] erro ao podar metric_network_interfaces: %v", err)
		return
	}
	if n > 0 {
		log.Printf("[Retention] metric_network_interfaces: %d linhas antigas removidas", n)
	}
}

func podarInterfacesSemSinal(ctx context.Context, maxAge time.Duration) {
	cutoff := time.Now().UTC().Add(-maxAge)
	sql := `DELETE FROM network_interfaces n
		WHERE n.last_seen < ?
		  AND NOT EXISTS (SELECT 1 FROM metric_network_interfaces m WHERE m.interface_id = n.id)
		  AND n.id IN (
			SELECT i.id FROM network_interfaces i
			WHERE i.last_seen < ?
			  AND NOT EXISTS (SELECT 1 FROM metric_network_interfaces m WHERE m.interface_id = i.id)
			ORDER BY i.id LIMIT ?)`

	n, err := podarEmLotes(ctx, dbExec, pruneBatchPause, sql, cutoff, cutoff)
	if err != nil {
		log.Printf("[Retention] erro ao podar network_interfaces: %v", err)
		return
	}
	if n > 0 {
		log.Printf("[Retention] network_interfaces: %d interfaces sem sinal e sem leitura removidas", n)
	}
}

func pruneAlerts(ctx context.Context, maxAge time.Duration) {
	cutoff := time.Now().UTC().Add(-maxAge)
	sql := `DELETE FROM alerts WHERE id IN (
		SELECT id FROM alerts
		WHERE created_at < ?
		  AND status <> 'open'
		  AND (status = 'resolved' OR delivery = 'enviado')
		ORDER BY id LIMIT ?)`

	total, err := podarEmLotes(ctx, dbExec, pruneBatchPause, sql, cutoff)
	if err != nil {
		log.Printf("[Retention] erro ao podar alerts: %v", err)
	}
	if total > 0 {
		log.Printf("[Retention] alerts: %d alertas antigos removidos", total)
	}
}

const pruneBatchSize = 5000

const pruneBatchPause = 100 * time.Millisecond

type execFunc func(ctx context.Context, sql string, args ...any) (int64, error)

func dbExec(ctx context.Context, sql string, args ...any) (int64, error) {
	res := DB.WithContext(ctx).Exec(sql, args...)
	return res.RowsAffected, res.Error
}

func prune(ctx context.Context, maxAge, auditMaxAge time.Duration) {
	cutoff := time.Now().UTC().Add(-maxAge)
	for _, table := range []string{"metric_servers", "metric_containers", "metric_load_balancers"} {
		n, err := pruneBatched(ctx, dbExec, table, "timestamp", cutoff, pruneBatchPause)
		if err != nil {
			log.Printf("[Retention] erro ao podar %s: %v", table, err)
			continue
		}
		if n > 0 {
			log.Printf("[Retention] %s: %d linhas antigas removidas", table, n)
		}
	}
	pruneContainers(ctx, cutoff)
	pruneAuditLog(ctx, auditMaxAge)
	pruneAlerts(ctx, config.Dias("ALERT_RETENTION_DAYS", defaultAlertRetentionDays))
	PodarEnderecos(config.Dias("ADDRESS_RETENTION_DAYS", defaultAddressRetentionDays))
	retencaoDeRede := config.Duracao("NETWORK_METRIC_RETENTION", defaultNetworkMetricRetention)
	PodarMetricasDeInterface(ctx, retencaoDeRede)
	podarInterfacesSemSinal(ctx, retencaoDeRede)
}

func pruneAuditLog(ctx context.Context, maxAge time.Duration) {
	cutoff := time.Now().UTC().Add(-maxAge)
	n, err := pruneBatched(ctx, dbExec, "audit_logs", "at", cutoff, pruneBatchPause)
	if err != nil {
		log.Printf("[Retention] erro ao podar audit_logs: %v", err)
		return
	}
	if n > 0 {
		log.Printf("[Retention] audit_logs: %d linhas antigas removidas", n)
	}
}

func PruneOlderThan(table, timeColumn string, cutoff time.Time) (int64, error) {
	return pruneBatched(context.Background(), dbExec, table, timeColumn, cutoff, pruneBatchPause)
}

func pruneBatchSQL(table, timeColumn string) string {
	return "DELETE FROM " + table +
		" WHERE id IN (SELECT id FROM " + table +
		" WHERE " + timeColumn + " < ? ORDER BY id LIMIT ?)"
}

func pruneBatched(ctx context.Context, exec execFunc, table, timeColumn string, cutoff time.Time, pausa time.Duration) (int64, error) {
	return podarEmLotes(ctx, exec, pausa, pruneBatchSQL(table, timeColumn), cutoff)
}

func podarEmLotes(ctx context.Context, exec execFunc, pausa time.Duration, sql string, args ...any) (int64, error) {
	args = append(args[:len(args):len(args)], pruneBatchSize)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := exec(ctx, sql, args...)
		if err != nil {
			return total, err
		}
		total += n
		if n < pruneBatchSize {
			return total, nil
		}

		espera := time.NewTimer(pausa)
		select {
		case <-ctx.Done():
			espera.Stop()
			return total, ctx.Err()
		case <-espera.C:
		}
	}
}

func pruneContainers(ctx context.Context, cutoff time.Time) {
	db := DB.WithContext(ctx)
	res := db.Exec(`
		DELETE FROM containers c
		WHERE c.created_at < ?
		  AND NOT EXISTS (
		      SELECT 1 FROM metric_containers m
		      WHERE m.container_id = c.id AND m.timestamp >= ?
		  )
	`, cutoff, cutoff)
	if res.Error != nil {
		log.Printf("[Retention] erro ao podar containers: %v", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("[Retention] containers: %d cadastros sem métrica removidos", res.RowsAffected)
	}

	res = db.Exec(`
		DELETE FROM metric_containers m
		WHERE NOT EXISTS (SELECT 1 FROM containers c WHERE c.id = m.container_id)
	`)
	if res.Error != nil {
		log.Printf("[Retention] erro ao podar métricas órfãs: %v", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("[Retention] metric_containers: %d métricas órfãs removidas", res.RowsAffected)
	}
}
