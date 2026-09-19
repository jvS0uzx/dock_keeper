package api

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	defaultAlertLimit = 100
	maxAlertLimit     = 500
)

func alertVisible(sess auth.Session, a database.Alert) bool {
	if a.SiteID == nil {
		return auth.HasGlobal(sess.Accesses)
	}
	return auth.CanSeeSite(sess.Accesses, a.SiteID)
}

func alertStatusFilter(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", database.AlertStatusOpen:
		return database.AlertStatusOpen, true
	case database.AlertStatusAcked:
		return database.AlertStatusAcked, true
	case database.AlertStatusResolved:
		return database.AlertStatusResolved, true
	case "all":
		return "all", true
	default:
		return "", false
	}
}

func alertLimit(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultAlertLimit, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > maxAlertLimit {
		return 0, false
	}
	return n, true
}

func alertsHandler(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	q := r.URL.Query()

	status, ok := alertStatusFilter(q.Get("status"))
	if !ok {
		writeError(w, http.StatusBadRequest, "status inválido: use open, acked, resolved ou all")
		return
	}
	limit, ok := alertLimit(q.Get("limit"))
	if !ok {
		writeError(w, http.StatusBadRequest, "limit inválido: use de 1 a 500")
		return
	}

	tx := database.From(r.Context()).Model(&database.Alert{})
	if status != "all" {
		tx = tx.Where("status = ?", status)
	}
	if v := strings.TrimSpace(q.Get("from")); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from inválido: use data RFC3339")
			return
		}
		tx = tx.Where("created_at >= ?", t)
	}
	if v := strings.TrimSpace(q.Get("to")); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to inválido: use data RFC3339")
			return
		}
		tx = tx.Where("created_at <= ?", t)
	}

	var alertas []database.Alert
	if err := tx.Order("created_at desc, id desc").Limit(maxAlertLimit).Find(&alertas).Error; err != nil {
		log.Printf("[Alertas] erro ao listar: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao listar os alertas")
		return
	}

	out := make([]database.Alert, 0, len(alertas))
	for _, a := range alertas {
		if !alertVisible(sess, a) {
			continue
		}
		out = append(out, a)
		if len(out) == limit {
			break
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func alertsSummaryHandler(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)

	var alertas []database.Alert
	err := database.From(r.Context()).Where("status <> ? OR delivery = ?",
		database.AlertStatusResolved, database.AlertDeliveryFalhou).
		Order("created_at desc").Limit(2000).Find(&alertas).Error
	if err != nil {
		log.Printf("[Alertas] erro ao resumir: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao resumir os alertas")
		return
	}

	resumo := map[string]int{"open": 0, "acked": 0, "falhou": 0}
	for _, a := range alertas {
		if !alertVisible(sess, a) {
			continue
		}
		switch a.Status {
		case database.AlertStatusOpen:
			resumo["open"]++
		case database.AlertStatusAcked:
			resumo["acked"]++
		}
		if a.Delivery == database.AlertDeliveryFalhou && a.Status != database.AlertStatusResolved {
			resumo["falhou"]++
		}
	}
	writeJSON(w, http.StatusOK, resumo)
}

func alertDaRota(w http.ResponseWriter, r *http.Request, sess auth.Session) (database.Alert, bool) {
	var alerta database.Alert

	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "id é obrigatório")
		return alerta, false
	}
	if err := database.From(r.Context()).Where("id = ?", id).Take(&alerta).Error; err != nil {
		writeError(w, http.StatusNotFound, "alerta não encontrado")
		return alerta, false
	}
	if !alertVisible(sess, alerta) {
		writeError(w, http.StatusNotFound, "alerta não encontrado")
		return alerta, false
	}
	if alerta.Status == database.AlertStatusResolved {
		writeError(w, http.StatusConflict, "alerta já resolvido")
		return alerta, false
	}
	return alerta, true
}

func alertAckHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := requireUserSession(w, r, "reconhecer alerta exige sessão de usuário")
	if !ok {
		return
	}
	alerta, ok := alertDaRota(w, r, sess)
	if !ok {
		return
	}

	agora := time.Now().UTC()
	mudanca := map[string]any{
		"status":   database.AlertStatusAcked,
		"acked_at": agora,
		"acked_by": sess.UserID,
	}
	if err := database.From(r.Context()).Model(&database.Alert{}).Where("id = ?", alerta.ID).Updates(mudanca).Error; err != nil {
		log.Printf("[Alertas] erro ao reconhecer %d: %v", alerta.ID, err)
		writeError(w, http.StatusInternalServerError, "falha ao reconhecer o alerta")
		return
	}

	alerta.Status = database.AlertStatusAcked
	alerta.AckedAt, alerta.AckedBy = &agora, &sess.UserID
	auditTarget(r, "alert", strconv.FormatUint(uint64(alerta.ID), 10), alerta.Key, alerta.SiteID)
	writeJSON(w, http.StatusOK, alerta)
}

func alertResolveHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := requireUserSession(w, r, "resolver alerta exige sessão de usuário")
	if !ok {
		return
	}
	alerta, ok := alertDaRota(w, r, sess)
	if !ok {
		return
	}
	if !auth.Allows(auth.RoleForSite(sess.Accesses, alerta.SiteID), auth.RoleOperator) {
		writeError(w, http.StatusForbidden, "resolver alerta exige perfil de operador")
		return
	}

	agora := time.Now().UTC()
	mudanca := map[string]any{
		"status":      database.AlertStatusResolved,
		"resolved_at": agora,
	}
	if err := database.From(r.Context()).Model(&database.Alert{}).Where("id = ?", alerta.ID).Updates(mudanca).Error; err != nil {
		log.Printf("[Alertas] erro ao resolver %d: %v", alerta.ID, err)
		writeError(w, http.StatusInternalServerError, "falha ao resolver o alerta")
		return
	}

	alerta.Status = database.AlertStatusResolved
	alerta.ResolvedAt = &agora
	auditTarget(r, "alert", strconv.FormatUint(uint64(alerta.ID), 10), alerta.Key, alerta.SiteID)
	writeJSON(w, http.StatusOK, alerta)
}
