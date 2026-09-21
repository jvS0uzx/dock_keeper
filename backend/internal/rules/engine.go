package rules

import (
	"context"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm/clause"

	"github.com/jvS0uzx/dock_keeper/internal/alert"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/metricas"
	"github.com/jvS0uzx/dock_keeper/internal/safego"
)

func metricValue(m database.MetricServer, metric string) (float64, bool) {
	definicao, ok := metricas.Buscar(metric)
	if !ok || !definicao.Avaliavel() {
		return 0, false
	}
	return definicao.Valor(m)
}

func resolveTargets(rule database.AlertRule, servers []database.Server) []string {
	if rule.TargetSiteID != nil {
		targets := make([]string, 0)
		for _, s := range servers {
			if s.SiteID != nil && *s.SiteID == *rule.TargetSiteID {
				targets = append(targets, s.ID)
			}
		}
		return targets
	}

	if rule.Target == "*" {
		targets := make([]string, 0, len(servers))
		for _, s := range servers {
			targets = append(targets, s.ID)
		}
		return targets
	}

	return []string{rule.Target}
}

func violates(value float64, operator string, threshold float64) bool {
	switch operator {
	case ">":
		return value > threshold
	case "<":
		return value < threshold
	}
	return false
}

func recentMetrics(latest []database.MetricServer, servers []database.Server, now time.Time) map[string]database.MetricServer {
	byServer := make(map[string]database.MetricServer, len(latest))
	for _, m := range latest {
		byServer[m.ServerID] = m
	}

	recent := make(map[string]database.MetricServer, len(byServer))
	for _, s := range servers {
		m, ok := byServer[s.ID]
		if !ok {
			continue
		}
		if now.Sub(m.Timestamp) <= database.LiveWindowFor(s.ReportIntervalSec) {
			recent[s.ID] = m
		}
	}
	return recent
}

var tickInterval = 30 * time.Second

func breachGap() time.Duration {
	if gap := 2 * tickInterval; gap > 90*time.Second {
		return gap
	}
	return 90 * time.Second
}

func entradaDoAlerta(rule database.AlertRule, serverID string, siteID *uint, key, texto, severidade, alvoNome string, valor float64) alert.Entrada {
	entrada := alert.Entrada{
		Key: key, Text: texto, Severity: severidade,
		SiteID: siteID, RuleID: &rule.ID,
		AlvoTipo: database.AlvoTipoHost,
		AlvoID:   serverID,
		AlvoNome: alvoNome,
		Metrica:  rule.Metric,
		Valor:    &valor,
		Limiar:   &rule.Threshold,
	}
	if definicao, ok := metricas.Buscar(rule.Metric); ok {
		entrada.Unidade = definicao.Unidade
	}
	if serverID != "" {
		entrada.ServerID = &serverID
	}
	if entrada.SiteID == nil {
		entrada.SiteID = rule.TargetSiteID
	}
	return entrada
}

func stateKey(ruleID uint, serverID string) string {
	return fmt.Sprintf("rule:%d:%s", ruleID, serverID)
}

func loadAlertStates(ruleID uint) map[string]database.AlertState {
	states := make(map[string]database.AlertState)
	if database.DB == nil {
		return states
	}

	var rows []database.AlertState
	if err := database.DB.Where("rule_id = ?", ruleID).Find(&rows).Error; err != nil {
		log.Printf("[rules] erro ao carregar estado da regra %d: %v", ruleID, err)
		return states
	}
	for _, r := range rows {
		states[r.Key] = r
	}
	return states
}

func flushBreaches(rows []database.AlertState) {
	if len(rows) == 0 || database.DB == nil {
		return
	}

	err := database.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"rule_id", "server_id", "severity",
			"first_breach_at", "last_breach_at", "active", "updated_at",
		}),
	}).Create(&rows).Error
	if err != nil {
		log.Printf("[rules] erro ao gravar estado de %d alvo(s): %v", len(rows), err)
	}
}

func flushSettled(keys []string, at time.Time) {
	if len(keys) == 0 || database.DB == nil {
		return
	}

	err := database.DB.Model(&database.AlertState{}).
		Where("key IN ?", keys).
		Updates(map[string]any{
			"active":          false,
			"first_breach_at": time.Time{},
			"last_breach_at":  time.Time{},
			"updated_at":      at,
		}).Error
	if err != nil {
		log.Printf("[rules] erro ao encerrar a violação de %d alvo(s): %v", len(keys), err)
	}
}

func breachStart(state database.AlertState, hasState bool, now time.Time) time.Time {
	if !hasState || state.FirstBreachAt.IsZero() {
		return now
	}
	if now.Sub(state.LastBreachAt) > breachGap() {
		return now
	}
	return state.FirstBreachAt
}

func StartEngine(interval time.Duration) {
	if interval > 0 {
		tickInterval = interval
	}
	safego.Run(context.Background(), "rules:engine", func(context.Context) {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			evaluate()
		}
	})
}

func evaluate() {
	var rules []database.AlertRule
	if err := database.DB.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		log.Printf("[rules] erro ao carregar regras: %v", err)
		return
	}
	if len(rules) == 0 {
		return
	}

	var servers []database.Server
	if err := database.DB.Find(&servers).Error; err != nil {
		log.Printf("[rules] erro ao carregar servidores: %v", err)
		return
	}
	nameByID := make(map[string]string, len(servers))
	siteByID := make(map[string]*uint, len(servers))
	for _, s := range servers {
		nameByID[s.ID] = s.Name
		siteByID[s.ID] = s.SiteID
	}

	var sites []database.Site
	if err := database.DB.Find(&sites).Error; err != nil {
		log.Printf("[rules] erro ao carregar unidades: %v", err)
		return
	}
	siteNameByID := make(map[uint]string, len(sites))
	for _, s := range sites {
		siteNameByID[s.ID] = s.Name
	}

	var latest []database.MetricServer
	if err := database.DB.Raw(database.ConsultaUltimasMetricas).Scan(&latest).Error; err != nil {
		log.Printf("[rules] erro ao carregar métricas recentes: %v", err)
		return
	}

	now := time.Now()
	metricByServer := recentMetrics(latest, servers, now)

	for _, rule := range rules {
		if rule.DependsOnServerID != nil {
			if _, parentUp := metricByServer[*rule.DependsOnServerID]; !parentUp {
				continue
			}
		}

		targets := resolveTargets(rule, servers)
		states := loadAlertStates(rule.ID)
		required := time.Duration(rule.ForDurationSec) * time.Second

		var breaches []database.AlertState
		var settled []string
		fired := false

		for _, serverID := range targets {
			m, ok := metricByServer[serverID]
			if !ok {
				continue
			}
			value, ok := metricValue(m, rule.Metric)
			if !ok {
				continue
			}

			key := stateKey(rule.ID, serverID)
			state, hasState := states[key]
			serverName := displayName(rule, serverID, nameByID, siteNameByID)

			if !violates(value, rule.Operator, rule.Threshold) {
				if hasState && state.Active {
					alert.Recovered(entradaDoAlerta(rule, serverID, siteByID[serverID], key, fmt.Sprintf(
						"%s Recuperado - Regra %s: %s=%.2f voltou ao limite (%s %.2f) em %s",
						Prefix(SeverityInfo), rule.Name, rule.Metric, value,
						rule.Operator, rule.Threshold, serverName,
					), SeverityInfo, serverName, value))
				}
				if hasState && (state.Active || !state.FirstBreachAt.IsZero()) {
					settled = append(settled, key)
				}
				continue
			}

			firstBreach := breachStart(state, hasState, now)

			active := hasState && state.Active
			if now.Sub(firstBreach) >= required {
				msg := fmt.Sprintf(
					"%s Regra %s: %s=%.2f %s %.2f em %s",
					Prefix(rule.Severity), rule.Name, rule.Metric, value, rule.Operator, rule.Threshold, serverName,
				)
				if alert.Enqueue(entradaDoAlerta(rule, serverID, siteByID[serverID], key, msg, rule.Severity, serverName, value)) {
					active = true
				}
				fired = true
			}

			breaches = append(breaches, database.AlertState{
				Key:           key,
				RuleID:        rule.ID,
				ServerID:      serverID,
				Severity:      rule.Severity,
				FirstBreachAt: firstBreach,
				LastBreachAt:  now,
				Active:        active,
				UpdatedAt:     now,
			})
		}

		flushBreaches(breaches)
		flushSettled(settled, now)

		if fired {
			at := now
			if err := database.DB.Model(&database.AlertRule{}).
				Where("id = ?", rule.ID).
				Update("last_fired", &at).Error; err != nil {
				log.Printf("[rules] erro ao marcar disparo da regra %d: %v", rule.ID, err)
			}
		}
	}
}

func displayName(rule database.AlertRule, serverID string, nameByID map[string]string, siteNameByID map[uint]string) string {
	name := nameByID[serverID]
	if name == "" {
		name = serverID
	}
	if rule.TargetSiteID != nil {
		if siteName := siteNameByID[*rule.TargetSiteID]; siteName != "" {
			name = fmt.Sprintf("%s (%s)", name, siteName)
		}
	}
	return name
}
