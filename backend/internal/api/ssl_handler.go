package api

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/network"
)

var validDomain = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)

func sslDomainsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		scope, status := resolveScope(sessionFrom(r), r)
		if status != 0 {
			writeError(w, status, "site_id inválido ou fora do seu alcance")
			return
		}

		tx := database.DB.Model(&database.Domain{})
		if scope.filter {
			tx = tx.Where("server_id IN (?)",
				scope.apply(database.DB.Model(&database.Server{}).Select("id")))
		}

		var domains []database.Domain
		if err := tx.Order("name ASC").Find(&domains).Error; err != nil {
			log.Printf("[API] erro ao listar domínios: %v", err)
			writeError(w, http.StatusInternalServerError, "falha ao listar domínios")
			return
		}
		if domains == nil {
			domains = []database.Domain{}
		}
		writeJSON(w, http.StatusOK, domains)

	case http.MethodPost:
		var req struct {
			Domain   string `json:"domain"`
			ServerID string `json:"server_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "corpo inválido")
			return
		}
		name := strings.ToLower(strings.TrimSpace(req.Domain))
		if !validDomain.MatchString(name) {
			writeError(w, http.StatusBadRequest, "domínio inválido")
			return
		}

		domain := database.Domain{Name: name, ServerID: optionalUUID(req.ServerID)}
		if err := database.DB.Create(&domain).Error; err != nil {
			log.Printf("[API] erro ao cadastrar domínio %s: %v", name, err)
			writeError(w, http.StatusConflict, "domínio já cadastrado ou inválido")
			return
		}
		go network.CheckAndStore(domain)
		writeJSON(w, http.StatusCreated, domain)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "id é obrigatório")
			return
		}
		if err := database.DB.Where("id = ?", id).Delete(&database.Domain{}).Error; err != nil {
			log.Printf("[API] erro ao remover domínio %s: %v", id, err)
			writeError(w, http.StatusInternalServerError, "falha ao remover domínio")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

func sslRecheckHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id é obrigatório")
		return
	}
	var domain database.Domain
	if err := database.DB.Where("id = ?", id).First(&domain).Error; err != nil {
		writeError(w, http.StatusNotFound, "domínio não encontrado")
		return
	}
	writeJSON(w, http.StatusOK, network.CheckAndStore(domain))
}

func sslRecheckAllHandler(w http.ResponseWriter, r *http.Request) {
	go network.CheckAllDomains()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "checking"})
}

func optionalUUID(raw string) *string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(raw)
	return &trimmed
}
