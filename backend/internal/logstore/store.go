package logstore

import (
	"log"
	"strings"
	"time"

	"github.com/joaov/vd_stats/internal/database"
)

// Save insere uma linha de log. É barata e síncrona: uma inserção direta no
// Postgres. Linhas vazias são ignoradas. Não abre goroutine por chamada — quem
// precisar de não bloquear o caminho crítico deve chamar dentro da própria
// goroutine de coleta/stream.
func Save(serverID, source, container, line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	entry := database.LogEntry{
		ServerID:  serverID,
		Source:    source,
		Container: container,
		Line:      line,
		Timestamp: time.Now().UTC(),
	}
	if err := database.DB.Create(&entry).Error; err != nil {
		log.Printf("[LogStore] erro ao salvar log (source=%s container=%s): %v", source, container, err)
	}
}

// StartRetention roda uma goroutine que apaga log_entries mais antigos que
// maxAge a cada interval (uso típico: 7 dias / 1h). Sem isso a tabela cresce
// indefinidamente com o volume de linhas de auth.log e containers.
func StartRetention(maxAge, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			cutoff := time.Now().UTC().Add(-maxAge)
			res := database.DB.Exec("DELETE FROM log_entries WHERE timestamp < ?", cutoff)
			if res.Error != nil {
				log.Printf("[LogStore] erro ao podar log_entries: %v", res.Error)
			} else if res.RowsAffected > 0 {
				log.Printf("[LogStore] %d linhas de log antigas removidas", res.RowsAffected)
			}
			<-ticker.C
		}
	}()
}
