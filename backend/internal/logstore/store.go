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
	// A guarda existe porque Save roda dentro das goroutines de streaming: um
	// pânico ali derruba o processo inteiro, e perder uma linha de log vale
	// menos que perder o painel. Hoje é inalcançável — main.go aborta se
	// Connect() falhar —, mas o custo da guarda é uma comparação.
	if database.DB == nil {
		log.Printf("[LogStore] banco indisponível: linha de %s descartada", source)
		return
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
			// Em lotes, com pausa entre eles. Um DELETE sem limite trava
			// log_entries, que é a tabela de maior volume do sistema — recebe
			// uma linha por linha de log de container e de auth.log —, e trava
			// exatamente no momento em que mais se escreve nela.
			n, err := database.PruneOlderThan("log_entries", "timestamp", cutoff)
			if err != nil {
				log.Printf("[LogStore] erro ao podar log_entries: %v", err)
			} else if n > 0 {
				log.Printf("[LogStore] %d linhas de log antigas removidas", n)
			}
			<-ticker.C
		}
	}()
}
