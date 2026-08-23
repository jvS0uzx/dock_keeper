package api

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/joaov/vd_stats/internal/database"
	"gorm.io/gorm"
)

const (
	defaultLogLimit = 200
	maxLogLimit     = 1000
)

// LogSearchHandler busca histórico de logs com filtros opcionais.
// GET /api/logs/search?server_id=&source=&container=&q=&limit=
// Ordena por timestamp desc e devolve um array JSON de LogEntry.
//
// LogEntry guarda server_id e não site_id, então o recorte por unidade não sai
// de scope.apply direto: entra como subconsulta sobre os servidores em escopo.
// Linha órfã — cujo server_id não corresponde a nenhum servidor — fica de fora
// dessa subconsulta e só aparece para quem tem concessão global, que não a
// aplica.
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

	tx := database.DB.Model(&database.LogEntry{})
	if v := q.Get("server_id"); v != "" {
		// Servidor fora do alcance responde 404 pelo mesmo motivo do C2: 403
		// confirmaria a existência do host que o recorte esconde.
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
			scope.apply(database.DB.Model(&database.Server{}).Select("id")))
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
