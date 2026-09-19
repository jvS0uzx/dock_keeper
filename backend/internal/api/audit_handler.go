package api

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	defaultAuditLimit = 50
	maxAuditLimit     = 200
)

type auditListPage struct {
	Items  []database.AuditLog `json:"items"`
	Total  int64               `json:"total"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
}

func auditListHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit := defaultAuditLimit
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	limit = min(limit, maxAuditLimit)

	offset := 0
	if raw := q.Get("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			offset = n
		}
	}

	tx := database.From(r.Context()).Model(&database.AuditLog{})

	if v := strings.TrimSpace(q.Get("actor")); v != "" {
		tx = tx.Where("actor_username = ?", v)
	}
	if v := strings.TrimSpace(q.Get("action")); v != "" {
		tx = tx.Where("action = ? OR action LIKE ?", v, v+".%")
	}
	if v := strings.TrimSpace(q.Get("result")); v != "" {
		tx = tx.Where("result = ?", v)
	}
	if v := strings.TrimSpace(q.Get("site_id")); v != "" {
		id, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			writeError(w, http.StatusBadRequest, "site_id inválido")
			return
		}
		tx = tx.Where("site_id = ?", uint(id))
	}

	from, ok := auditTime(w, q.Get("from"), "from")
	if !ok {
		return
	}
	if from != nil {
		tx = tx.Where("at >= ?", *from)
	}
	to, ok := auditTime(w, q.Get("to"), "to")
	if !ok {
		return
	}
	if to != nil {
		tx = tx.Where("at <= ?", *to)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		log.Printf("[Auditoria] erro ao contar: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao consultar a auditoria")
		return
	}

	var items []database.AuditLog
	if err := tx.Order("at desc, id desc").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		log.Printf("[Auditoria] erro na consulta: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao consultar a auditoria")
		return
	}
	if items == nil {
		items = []database.AuditLog{}
	}

	writeJSON(w, http.StatusOK, auditListPage{
		Items: items, Total: total, Limit: limit, Offset: offset,
	})
}

func auditTime(w http.ResponseWriter, raw, campo string) (*time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, campo+" precisa estar no formato RFC3339")
		return nil, false
	}
	return &t, true
}
