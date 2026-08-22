package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/joaov/vd_stats/internal/database"
	"gorm.io/gorm"
)

// LogSearchHandler busca histórico de logs com filtros opcionais.
// GET /api/logs/search?server_id=&source=&container=&q=&limit=
// Ordena por timestamp desc e devolve um array JSON de LogEntry.
func LogSearchHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	q := r.URL.Query()

	limit := 200
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	tx := database.DB.Model(&database.LogEntry{})
	if v := q.Get("server_id"); v != "" {
		tx = tx.Where("server_id = ?", v)
	}
	if v := q.Get("source"); v != "" {
		tx = tx.Where("source = ?", v)
	}
	if v := q.Get("container"); v != "" {
		tx = tx.Where("container = ?", v)
	}
	if v := q.Get("q"); v != "" {
		tx = tx.Where("line ILIKE ?", "%"+v+"%")
	}

	var entries []database.LogEntry
	if err := tx.Order("timestamp desc").Limit(limit).Find(&entries).Error; err != nil && err != gorm.ErrRecordNotFound {
		log.Printf("[LogSearch] erro na busca: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "search failed"})
		return
	}

	if entries == nil {
		entries = []database.LogEntry{}
	}
	json.NewEncoder(w).Encode(entries)
}
