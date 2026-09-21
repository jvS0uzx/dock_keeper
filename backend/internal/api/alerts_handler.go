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

type alertaComOrigem struct {
	database.Alert
	ServerName *string `json:"server_name"`
	SiteName   *string `json:"site_name"`
}

func escopoDosAlertas(w http.ResponseWriter, r *http.Request) (siteScope, bool) {
	escopo, status := resolveScope(sessionFrom(r), r)
	switch status {
	case 0:
		return escopo, true
	case http.StatusBadRequest:
		writeError(w, status, "site_id inválido: use o id da unidade, none ou all")
	default:
		writeError(w, status, "unidade fora do seu alcance")
	}
	return siteScope{}, false
}

func comOrigem(r *http.Request, alertas []database.Alert) ([]alertaComOrigem, error) {
	servidores, unidades := []string{}, []uint{}
	for _, a := range alertas {
		if a.ServerID != nil {
			servidores = append(servidores, *a.ServerID)
		}
		if a.SiteID != nil {
			unidades = append(unidades, *a.SiteID)
		}
	}

	nomeDoServidor := map[string]string{}
	if len(servidores) > 0 {
		var linhas []database.Server
		err := database.From(r.Context()).Unscoped().Select("id", "name").Where("id IN ?", servidores).Find(&linhas).Error
		if err != nil {
			return nil, err
		}
		for _, s := range linhas {
			nomeDoServidor[s.ID] = s.Name
		}
	}
	nomeDaUnidade := map[uint]string{}
	if len(unidades) > 0 {
		var linhas []database.Site
		err := database.From(r.Context()).Select("id", "name").Where("id IN ?", unidades).Find(&linhas).Error
		if err != nil {
			return nil, err
		}
		for _, u := range linhas {
			nomeDaUnidade[u.ID] = u.Name
		}
	}

	out := make([]alertaComOrigem, 0, len(alertas))
	for _, a := range alertas {
		linha := alertaComOrigem{Alert: a}
		if a.ServerID != nil {
			if nome, ok := nomeDoServidor[*a.ServerID]; ok {
				linha.ServerName = &nome
			}
		}
		if a.SiteID != nil {
			if nome, ok := nomeDaUnidade[*a.SiteID]; ok {
				linha.SiteName = &nome
			}
		}
		out = append(out, linha)
	}
	return out, nil
}

func alertsHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	escopo, ok := escopoDosAlertas(w, r)
	if !ok {
		return
	}

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

	tx := escopo.apply(database.From(r.Context()).Model(&database.Alert{}))
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
	if err := tx.Order("created_at desc, id desc").Limit(limit).Find(&alertas).Error; err != nil {
		log.Printf("[Alertas] erro ao listar: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao listar os alertas")
		return
	}

	out, err := comOrigem(r, alertas)
	if err != nil {
		log.Printf("[Alertas] erro ao resolver a origem: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao listar os alertas")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func alertsSummaryHandler(w http.ResponseWriter, r *http.Request) {
	escopo, ok := escopoDosAlertas(w, r)
	if !ok {
		return
	}

	var contagem struct {
		Open   int
		Acked  int
		Falhou int
	}
	err := escopo.apply(database.From(r.Context()).Model(&database.Alert{})).
		Select(`count(*) FILTER (WHERE status = ?) AS open,
			count(*) FILTER (WHERE status = ?) AS acked,
			count(*) FILTER (WHERE delivery = ?) AS falhou`,
			database.AlertStatusOpen, database.AlertStatusAcked, database.AlertDeliveryFalhou).
		Where("status <> ?", database.AlertStatusResolved).
		Scan(&contagem).Error
	if err != nil {
		log.Printf("[Alertas] erro ao resumir: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao resumir os alertas")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{
		"open": contagem.Open, "acked": contagem.Acked, "falhou": contagem.Falhou,
	})
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
	if !auth.Allows(auth.RoleForSite(sess.Accesses, alerta.SiteID), auth.RoleOperator) {
		writeError(w, http.StatusForbidden, "reconhecer alerta exige perfil de operador")
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
