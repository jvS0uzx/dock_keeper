package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/network"
)

const vhostWindow = "24 hours"

type discoveredDomain struct {
	Domain     string `json:"domain"`
	Monitored  bool   `json:"monitored"`
	SampleReqs int    `json:"sample_reqs"`
}

func sslDiscoverHandler(w http.ResponseWriter, r *http.Request) {
	scope, status := resolveScope(sessionFrom(r), r)
	if status != 0 {
		writeError(w, status, "site_id inválido ou fora do seu alcance")
		return
	}

	names, err := observedVHosts(scope)
	if err != nil {
		log.Printf("[SSL] erro ao listar vhosts observados: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao consultar os domínios do Nginx")
		return
	}

	monitoredTx := database.DB.Model(&database.Domain{})
	if scope.filter {
		monitoredTx = monitoredTx.Where("server_id IN (?)",
			scope.apply(database.DB.Model(&database.Server{}).Select("id")))
	}

	var existing []database.Domain
	if err := monitoredTx.Find(&existing).Error; err != nil {
		log.Printf("[SSL] erro ao listar domínios cadastrados: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao ler os domínios cadastrados")
		return
	}
	monitored := make(map[string]bool, len(existing))
	for _, d := range existing {
		monitored[strings.ToLower(d.Name)] = true
	}

	out := make([]discoveredDomain, 0, len(names))
	for name, reqs := range names {
		out = append(out, discoveredDomain{
			Domain:     name,
			Monitored:  monitored[name],
			SampleReqs: reqs,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func sslImportHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domains []string `json:"domains"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if len(req.Domains) == 0 {
		writeError(w, http.StatusBadRequest, "informe ao menos um domínio")
		return
	}

	scope, status := resolveScope(sessionFrom(r), r)
	if status != 0 {
		writeError(w, status, "site_id inválido ou fora do seu alcance")
		return
	}

	observed, err := observedVHosts(scope)
	if err != nil {
		log.Printf("[SSL] erro ao validar vhosts: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao consultar os domínios do Nginx")
		return
	}

	imported := make([]database.Domain, 0, len(req.Domains))
	for _, raw := range req.Domains {
		name := strings.ToLower(strings.TrimSpace(raw))
		if !validDomain.MatchString(name) {
			continue
		}
		if _, seen := observed[name]; !seen {
			writeError(w, http.StatusBadRequest, "domínio "+name+" não aparece no log do Nginx")
			return
		}

		domain := database.Domain{Name: name}
		if err := database.DB.Where("name = ?", name).FirstOrCreate(&domain).Error; err != nil {
			log.Printf("[SSL] erro ao importar %s: %v", name, err)
			continue
		}
		imported = append(imported, domain)
	}

	for _, d := range imported {
		go network.CheckAndStore(d)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"imported": len(imported),
	})
}

func observedVHosts(scope siteScope) (map[string]int, error) {
	type row struct {
		ServerName string
		Reqs       int
	}

	tx := scope.apply(database.DB.Model(&database.MetricLoadBalancer{})).
		Select("server_name, SUM(requests_count) AS reqs").
		Where("timestamp >= NOW() - INTERVAL '" + vhostWindow + "'").
		Where("server_name <> ''").
		Group("server_name")

	var rows []row
	if err := tx.Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make(map[string]int, len(rows))
	for _, r := range rows {
		name := strings.ToLower(strings.TrimSpace(r.ServerName))
		if validDomain.MatchString(name) {
			out[name] += r.Reqs
		}
	}
	return out, nil
}
