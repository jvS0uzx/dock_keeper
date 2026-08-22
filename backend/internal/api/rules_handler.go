package api

import (
	"encoding/json"
	"net/http"

	"github.com/joaov/vd_stats/internal/database"
)

type alertRuleRequest struct {
	Name      string  `json:"name"`
	Target    string  `json:"target"`
	Metric    string  `json:"metric"`
	Operator  string  `json:"operator"`
	Threshold float64 `json:"threshold"`
	Enabled   *bool   `json:"enabled"`
}

var validRuleMetrics = map[string]bool{"cpu": true, "mem": true, "disk": true, "load": true}
var validRuleOperators = map[string]bool{">": true, "<": true}

// AlertRulesHandler faz o CRUD de AlertRule.
// GET lista, POST cria, DELETE remove (?id=), PUT/PATCH alterna enabled (?id=).
func AlertRulesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		var rules []database.AlertRule
		database.DB.Order("id ASC").Find(&rules)
		if rules == nil {
			rules = []database.AlertRule{}
		}
		json.NewEncoder(w).Encode(rules)

	case http.MethodPost:
		var req alertRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		if !validRuleMetrics[req.Metric] {
			http.Error(w, "invalid metric", http.StatusBadRequest)
			return
		}
		if !validRuleOperators[req.Operator] {
			http.Error(w, "invalid operator", http.StatusBadRequest)
			return
		}
		if req.Target == "" {
			req.Target = "*"
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		rule := database.AlertRule{
			Name:      req.Name,
			Target:    req.Target,
			Metric:    req.Metric,
			Operator:  req.Operator,
			Threshold: req.Threshold,
			Enabled:   enabled,
		}
		if err := database.DB.Create(&rule).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(rule)

	case http.MethodPut, http.MethodPatch:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.Enabled == nil {
			http.Error(w, "enabled required", http.StatusBadRequest)
			return
		}
		if err := database.DB.Model(&database.AlertRule{}).
			Where("id = ?", id).
			Update("enabled", *body.Enabled).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "updated", "enabled": *body.Enabled})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		database.DB.Where("id = ?", id).Delete(&database.AlertRule{})
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
