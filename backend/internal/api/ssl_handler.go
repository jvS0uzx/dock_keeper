package api

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/joaov/vd_stats/internal/database"
	"github.com/joaov/vd_stats/internal/network"
)

// Aceita apenas nome de host: rótulos alfanuméricos separados por ponto. O
// valor vai virar destino de conexão TLS, então não pode carregar esquema,
// porta, caminho nem espaço.
var validDomain = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)

// sslDomainsHandler faz o CRUD dos domínios monitorados.
//
// A leitura é recortada pela unidade do servidor a que o domínio está
// amarrado. Domínio sem servidor cai fora da subconsulta e só aparece para quem
// tem concessão global, que não a aplica — mesma regra do log e da regra de
// alerta, porque o certificado de uma filial diz que sistema ela roda.
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
		// Checa na hora para o domínio já aparecer com status, sem esperar o worker.
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

// sslRecheckHandler refaz o handshake de um domínio agora e devolve o registro.
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

// sslRecheckAllHandler dispara a varredura completa em background.
func sslRecheckAllHandler(w http.ResponseWriter, r *http.Request) {
	go network.CheckAllDomains()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "checking"})
}

// optionalUUID converte string vazia em NULL. O painel manda server_id em
// branco quando o domínio não está preso a um servidor, e "" não é uuid válido.
func optionalUUID(raw string) *string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(raw)
	return &trimmed
}
