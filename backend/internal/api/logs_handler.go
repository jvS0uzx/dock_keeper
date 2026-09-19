package api

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/jvS0uzx/dock_keeper/internal/database"
	"gorm.io/gorm"
)

const (
	defaultLogLimit = 200
	maxLogLimit     = 1000
)

func LogSearchHandler(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	scope, status := resolveScope(sess, r)
	if status != 0 {
		writeError(w, status, "site_id inválido ou fora do seu alcance")
		return
	}

	q := r.URL.Query()

	limit := defaultLogLimit
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	limit = min(limit, maxLogLimit)

	tx := database.From(r.Context()).Model(&database.LogEntry{})
	if v := q.Get("server_id"); v != "" {
		server, ok := lookupServer(w, sess, v)
		if !ok {
			return
		}
		if !scope.matches(server.SiteID) {
			writeError(w, http.StatusNotFound, "servidor não encontrado")
			return
		}
		tx = tx.Where("server_id = ?", v)
	} else if scope.filter {
		tx = tx.Where("server_id IN (?)",
			scope.apply(database.From(r.Context()).Model(&database.Server{}).Select("id")))
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
	if err := tx.Order("timestamp desc").Limit(limit).Find(&entries).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("[LogSearch] erro na busca: %v", err)
		writeError(w, http.StatusInternalServerError, "falha na busca de logs")
		return
	}

	if entries == nil {
		entries = []database.LogEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}
