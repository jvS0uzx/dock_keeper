package database

import (
	"log"
	"time"
)

// StartRetentionWorker apaga métricas mais antigas que maxAge em intervalos fixos.
// Sem isso as tabelas metric_* crescem sem limite (inserção a cada 1-2s por servidor)
// e o Postgres incha até degradar as queries do painel.
func StartRetentionWorker(maxAge, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			prune(maxAge)
			<-ticker.C
		}
	}()
}

func prune(maxAge time.Duration) {
	cutoff := time.Now().UTC().Add(-maxAge)
	for _, table := range []string{"metric_servers", "metric_containers", "metric_load_balancers"} {
		res := DB.Exec("DELETE FROM "+table+" WHERE timestamp < ?", cutoff)
		if res.Error != nil {
			log.Printf("[Retention] erro ao podar %s: %v", table, res.Error)
			continue
		}
		if res.RowsAffected > 0 {
			log.Printf("[Retention] %s: %d linhas antigas removidas", table, res.RowsAffected)
		}
	}
}
