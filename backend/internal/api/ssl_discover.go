package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/joaov/vd_stats/internal/database"
	"github.com/joaov/vd_stats/internal/network"
)

// Janela de log consultada para descobrir vhosts. Um dia cobre domínio que só
// recebe acesso em horário comercial.
const vhostWindow = "24 hours"

type discoveredDomain struct {
	Domain     string `json:"domain"`
	Monitored  bool   `json:"monitored"`
	SampleReqs int    `json:"sample_reqs"`
}

// sslDiscoverHandler lista os domínios que o Nginx atendeu, marcando quais já
// estão sob monitoramento de certificado.
//
// O cadastro manual era a única entrada, mas o painel já sabe quais vhosts
// existem: o access log do balanceador traz o server_name de cada requisição.
// Pedir para o operador redigitar o que o sistema já observou é trabalho à toa
// e fonte de domínio esquecido — justo o que vence sem ninguém ver.
//
// O recorte é por unidade, pela coluna site_id que a linha do balanceador
// passou a carregar. Antes a tabela não tinha unidade nenhuma e a rota só sabia
// devolver a topologia inteira ou lista vazia: quem tem papel só numa filial
// via a tela de descoberta em branco, que é segurança comprada com uma tela
// quebrada.
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

	// Os já cadastrados saem sob o mesmo recorte: marcar como "monitorado" um
	// domínio de outra unidade contaria ao operador que ele existe.
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

// sslImportHandler cadastra de uma vez os domínios escolhidos.
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

	// O cerco de importação usa o MESMO recorte da descoberta. Validar contra a
	// lista global deixaria o operador de uma filial cadastrar domínio que ele
	// não pode nem enxergar, bastando digitar o nome.
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
		// Só entra o que o próprio Nginx atendeu: a lista vem do cliente e sem
		// esse cerco o endpoint viraria um cadastro de domínio arbitrário.
		if _, seen := observed[name]; !seen {
			writeError(w, http.StatusBadRequest, "domínio "+name+" não aparece no log do Nginx")
			return
		}

		domain := database.Domain{Name: name}
		// Domínio repetido não é erro: o operador pode reimportar a lista.
		if err := database.DB.Where("name = ?", name).FirstOrCreate(&domain).Error; err != nil {
			log.Printf("[SSL] erro ao importar %s: %v", name, err)
			continue
		}
		imported = append(imported, domain)
	}

	// Checa em background para os domínios já aparecerem com status, sem o
	// operador esperar o handshake de cada um.
	for _, d := range imported {
		go network.CheckAndStore(d)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"imported": len(imported),
	})
}

// observedVHosts devolve os server_name vistos no access log, com o total de
// requisições de cada um, restritos ao alcance da sessão.
//
// Linha antiga, gravada antes de a coluna site_id existir, tem unidade nula e
// some para quem é restrito. Não há migração de dado: a retenção de métrica é
// de 7 dias, então o histórico sem unidade se resolve sozinho, e inventar uma
// unidade para linha cuja origem o sistema não registrou seria adivinhação
// gravada como fato.
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
		// O Nginx registra "_" ou "-" para requisição sem Host reconhecido.
		if validDomain.MatchString(name) {
			out[name] += r.Reqs
		}
	}
	return out, nil
}
