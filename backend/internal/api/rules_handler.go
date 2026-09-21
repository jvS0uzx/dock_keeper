package api

import (
	"gorm.io/gorm"

	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/metricas"
	"github.com/jvS0uzx/dock_keeper/internal/rules"
)

type alertRuleRequest struct {
	Name              string  `json:"name"`
	Target            string  `json:"target"`
	Metric            string  `json:"metric"`
	Operator          string  `json:"operator"`
	Threshold         float64 `json:"threshold"`
	Enabled           *bool   `json:"enabled"`
	Severity          string  `json:"severity"`
	DependsOnServerID *string `json:"depends_on_server_id"`
	TargetSiteID      *uint   `json:"target_site_id"`

	ForDurationSec int `json:"for_duration_sec"`
}

const maxRuleDurationSec = 24 * 60 * 60

var validRuleOperators = map[string]bool{">": true, "<": true}

func ruleSiteID(rule database.AlertRule, siteByServer map[string]*uint) (*uint, bool) {
	if rule.TargetSiteID != nil {
		return rule.TargetSiteID, true
	}
	if rule.Target != "" && rule.Target != "*" {
		site, ok := siteByServer[rule.Target]
		return site, ok
	}
	return nil, false
}

func visibleRules(all []database.AlertRule, scope siteScope, siteByServer map[string]*uint, hasGlobal bool) []database.AlertRule {
	out := make([]database.AlertRule, 0, len(all))
	for _, rule := range all {
		site, bound := ruleSiteID(rule, siteByServer)
		if !bound {
			if hasGlobal {
				out = append(out, rule)
			}
			continue
		}
		if scope.matches(site) {
			out = append(out, rule)
		}
	}
	return out
}

func serverSites() (map[string]*uint, error) {
	var servers []database.Server
	if err := database.DB.Model(&database.Server{}).Select("id", "site_id").Find(&servers).Error; err != nil {
		return nil, err
	}
	out := make(map[string]*uint, len(servers))
	for _, s := range servers {
		out[s.ID] = s.SiteID
	}
	return out, nil
}

func AlertRulesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sess := sessionFrom(r)
		scope, status := resolveScope(sess, r)
		if status != 0 {
			writeError(w, status, "site_id inválido ou fora do seu alcance")
			return
		}

		var all []database.AlertRule
		if err := database.DB.Order("id ASC").Find(&all).Error; err != nil {
			log.Printf("[Rules] erro ao listar regras: %v", err)
			writeError(w, http.StatusInternalServerError, "falha ao listar regras")
			return
		}
		sites, err := serverSites()
		if err != nil {
			log.Printf("[Rules] erro ao resolver a unidade dos alvos: %v", err)
			writeError(w, http.StatusInternalServerError, "falha ao listar regras")
			return
		}
		writeJSON(w, http.StatusOK, visibleRules(all, scope, sites, auth.HasGlobal(sess.Accesses)))

	case http.MethodPost:
		var req alertRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "corpo inválido")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name é obrigatório")
			return
		}
		if !metricas.Avaliavel(req.Metric) {
			writeError(w, http.StatusBadRequest, "métrica inválida")
			return
		}
		if !validRuleOperators[req.Operator] {
			writeError(w, http.StatusBadRequest, "operador inválido")
			return
		}
		if req.Severity == "" {
			req.Severity = rules.SeverityWarning
		}
		if !rules.ValidSeverity(req.Severity) {
			writeError(w, http.StatusBadRequest, "severidade inválida: use info, warning, high ou critical")
			return
		}
		if req.DependsOnServerID != nil && *req.DependsOnServerID == req.Target {
			writeError(w, http.StatusBadRequest, "a regra não pode depender do próprio alvo")
			return
		}
		if req.Target == "" {
			req.Target = "*"
		}
		if req.TargetSiteID != nil {
			if req.Target != "*" {
				writeError(w, http.StatusBadRequest, "informe alvo por unidade OU por servidor, não os dois")
				return
			}
			var count int64
			if err := database.DB.Model(&database.Site{}).
				Where("id = ?", *req.TargetSiteID).Count(&count).Error; err != nil {
				log.Printf("[Rules] erro ao validar a unidade %d: %v", *req.TargetSiteID, err)
				writeError(w, http.StatusInternalServerError, "falha ao validar a unidade")
				return
			}
			if count == 0 {
				writeError(w, http.StatusBadRequest, "unidade inexistente")
				return
			}
			req.Target = "*"
		}
		if req.ForDurationSec < 0 {
			writeError(w, http.StatusBadRequest, "a duração exigida não pode ser negativa")
			return
		}
		if req.ForDurationSec > maxRuleDurationSec {
			writeError(w, http.StatusBadRequest, "a duração exigida não pode passar de 24 horas")
			return
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		rule := database.AlertRule{
			Name:              req.Name,
			Target:            req.Target,
			Metric:            req.Metric,
			Operator:          req.Operator,
			Threshold:         req.Threshold,
			Enabled:           enabled,
			Severity:          req.Severity,
			DependsOnServerID: req.DependsOnServerID,
			TargetSiteID:      req.TargetSiteID,
			ForDurationSec:    req.ForDurationSec,
		}
		if err := database.DB.Create(&rule).Error; err != nil {
			log.Printf("[Rules] erro ao criar regra %q: %v", rule.Name, err)
			writeError(w, http.StatusInternalServerError, "falha ao criar regra")
			return
		}
		auditRuleTarget(r, rule)
		writeJSON(w, http.StatusCreated, rule)

	case http.MethodPut, http.MethodPatch:
		id := r.URL.Query().Get("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "id é obrigatório")
			return
		}
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "corpo inválido")
			return
		}
		if body.Enabled == nil {
			writeError(w, http.StatusBadRequest, "enabled é obrigatório")
			return
		}
		if err := database.DB.Model(&database.AlertRule{}).
			Where("id = ?", id).
			Update("enabled", *body.Enabled).Error; err != nil {
			log.Printf("[Rules] erro ao alternar regra %s: %v", id, err)
			writeError(w, http.StatusInternalServerError, "falha ao atualizar regra")
			return
		}
		var updated database.AlertRule
		if database.DB.Where("id = ?", id).First(&updated).Error == nil {
			auditRuleTarget(r, updated)
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "enabled": *body.Enabled})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "id é obrigatório")
			return
		}
		var doomed database.AlertRule
		found := database.DB.Where("id = ?", id).First(&doomed).Error == nil

		if err := removerRegra(id); err != nil {
			log.Printf("[Rules] erro ao remover regra %s: %v", id, err)
			writeError(w, http.StatusInternalServerError, "falha ao remover regra")
			return
		}

		if found {
			auditRuleTarget(r, doomed)
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

func removerRegra(id string) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Delete(&database.AlertRule{}).Error; err != nil {
			return err
		}
		return tx.Where("rule_id = ?", id).Delete(&database.AlertState{}).Error
	})
}

func auditRuleTarget(r *http.Request, rule database.AlertRule) {
	auditTarget(r, "alert-rule",
		strconv.FormatUint(uint64(rule.ID), 10), rule.Name, ruleAuditSite(rule))
}

func ruleAuditSite(rule database.AlertRule) *uint {
	if rule.TargetSiteID != nil {
		return rule.TargetSiteID
	}
	if rule.Target == "" || rule.Target == "*" {
		return nil
	}
	var server database.Server
	if database.DB.Select("site_id").Where("id = ?", rule.Target).First(&server).Error != nil {
		return nil
	}
	return server.SiteID
}
