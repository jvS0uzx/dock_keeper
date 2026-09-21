package database

import (
	"context"
	"log"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/config"

	"github.com/jvS0uzx/dock_keeper/internal/safego"
)

func StartRetentionWorker(maxAge, interval time.Duration, trendsReady <-chan struct{}) {
	auditMaxAge := time.Duration(config.Inteiro("AUDIT_RETENTION_DAYS", defaultAuditRetentionDays)) * 24 * time.Hour

	safego.Run(context.Background(), "database:retencao", func(context.Context) {
		if trendsReady != nil {
			<-trendsReady
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			prune(maxAge, auditMaxAge)
			<-ticker.C
		}
	})
}

const defaultAuditRetentionDays = 365

const defaultAlertRetentionDays = 90

const defaultAddressRetentionDays = 30

func pruneAlerts(maxAge time.Duration) {
	cutoff := time.Now().UTC().Add(-maxAge)
	sql := `DELETE FROM alerts WHERE id IN (
		SELECT id FROM alerts
		WHERE created_at < ?
		  AND status <> 'open'
		  AND (status = 'resolved' OR delivery = 'enviado')
		ORDER BY id LIMIT ?)`

	var total int64
	for range pruneMaxBatches {
		n, err := dbExec(sql, cutoff, pruneBatchSize)
		if err != nil {
			log.Printf("[Retention] erro ao podar alerts: %v", err)
			return
		}
		total += n
		if n < pruneBatchSize {
			break
		}
		time.Sleep(pruneBatchPause)
	}
	if total > 0 {
		log.Printf("[Retention] alerts: %d alertas antigos removidos", total)
	}
}

const pruneBatchSize = 5000

const pruneBatchPause = 100 * time.Millisecond

const pruneMaxBatches = 200

type execFunc func(sql string, args ...any) (int64, error)

func dbExec(sql string, args ...any) (int64, error) {
	res := DB.Exec(sql, args...)
	return res.RowsAffected, res.Error
}

func prune(maxAge, auditMaxAge time.Duration) {
	cutoff := time.Now().UTC().Add(-maxAge)
	for _, table := range []string{"metric_servers", "metric_containers", "metric_load_balancers"} {
		n, err := pruneBatched(dbExec, table, "timestamp", cutoff, time.Sleep)
		if err != nil {
			log.Printf("[Retention] erro ao podar %s: %v", table, err)
			continue
		}
		if n > 0 {
			log.Printf("[Retention] %s: %d linhas antigas removidas", table, n)
		}
	}
	pruneContainers(cutoff)
	pruneAuditLog(auditMaxAge)
	pruneAlerts(config.Dias("ALERT_RETENTION_DAYS", defaultAlertRetentionDays))
	PodarEnderecos(config.Dias("ADDRESS_RETENTION_DAYS", defaultAddressRetentionDays))
}

func pruneAuditLog(maxAge time.Duration) {
	cutoff := time.Now().UTC().Add(-maxAge)
	n, err := pruneBatched(dbExec, "audit_logs", "at", cutoff, time.Sleep)
	if err != nil {
		log.Printf("[Retention] erro ao podar audit_logs: %v", err)
		return
	}
	if n > 0 {
		log.Printf("[Retention] audit_logs: %d linhas antigas removidas", n)
	}
}

func PruneOlderThan(table, timeColumn string, cutoff time.Time) (int64, error) {
	return pruneBatched(dbExec, table, timeColumn, cutoff, time.Sleep)
}

func pruneBatchSQL(table, timeColumn string) string {
	return "DELETE FROM " + table +
		" WHERE id IN (SELECT id FROM " + table +
		" WHERE " + timeColumn + " < ? ORDER BY id LIMIT ?)"
}

func pruneBatched(exec execFunc, table, timeColumn string, cutoff time.Time, sleep func(time.Duration)) (int64, error) {
	sql := pruneBatchSQL(table, timeColumn)

	var total int64
	for i := 0; i < pruneMaxBatches; i++ {
		n, err := exec(sql, cutoff, pruneBatchSize)
		if err != nil {
			return total, err
		}
		total += n

		if n < pruneBatchSize {
			return total, nil
		}
		sleep(pruneBatchPause)
	}

	log.Printf("[Retention] %s: teto de %d lotes atingido, o restante sai no próximo ciclo",
		table, pruneMaxBatches)
	return total, nil
}

func pruneContainers(cutoff time.Time) {
	res := DB.Exec(`
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

	res = DB.Exec(`
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
