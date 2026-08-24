package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/joaov/vd_stats/internal/auth"
	"github.com/joaov/vd_stats/internal/database"
	"github.com/joaov/vd_stats/internal/discovery"
)

// sitesHandler faz o CRUD das unidades monitoradas.
func sitesHandler(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)

	switch r.Method {
	case http.MethodGet:
		tx := database.DB.Order("name ASC")
		// Usuário restrito só lista as unidades que alcança.
		if !auth.HasGlobal(sess.Accesses) {
			ids := auth.SiteIDs(sess.Accesses)
			if len(ids) == 0 {
				writeJSON(w, http.StatusOK, []database.Site{})
				return
			}
			tx = tx.Where("id IN ?", ids)
		}
		var sites []database.Site
		if err := tx.Find(&sites).Error; err != nil {
			log.Printf("[API] erro ao listar unidades: %v", err)
			writeError(w, http.StatusInternalServerError, "falha ao listar unidades")
			return
		}
		if sites == nil {
			sites = []database.Site{}
		}
		writeJSON(w, http.StatusOK, sites)

	case http.MethodPost:
		// Criar/remover unidade muda o cadastro global: exige operador global,
		// não apenas operador de uma unidade.
		if !auth.Allows(auth.GlobalRole(sess.Accesses), auth.RoleOperator) {
			writeError(w, http.StatusForbidden, "criar unidades exige acesso global de Suporte TI")
			return
		}
		var req struct {
			Name      string  `json:"name"`
			Code      string  `json:"code"`
			Address   string  `json:"address"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "corpo inválido")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.Code = strings.ToLower(strings.TrimSpace(req.Code))
		if req.Name == "" || req.Code == "" {
			writeError(w, http.StatusBadRequest, "name e code são obrigatórios")
			return
		}

		site := database.Site{
			Name: req.Name, Code: req.Code, Address: strings.TrimSpace(req.Address),
			Latitude: req.Latitude, Longitude: req.Longitude,
		}
		if err := database.DB.Create(&site).Error; err != nil {
			log.Printf("[API] erro ao cadastrar unidade %q: %v", req.Code, err)
			writeError(w, http.StatusConflict, "unidade já existe ou dados inválidos")
			return
		}
		auditTarget(r, "site", strconv.FormatUint(uint64(site.ID), 10), site.Name, &site.ID)
		writeJSON(w, http.StatusCreated, site)

	case http.MethodDelete:
		if !auth.Allows(auth.GlobalRole(sess.Accesses), auth.RoleOperator) {
			writeError(w, http.StatusForbidden, "remover unidades exige acesso global de Suporte TI")
			return
		}
		id := r.URL.Query().Get("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "id é obrigatório")
			return
		}
		// O nome é lido antes da exclusão: remover a unidade 4 é exatamente o
		// tipo de linha que ninguém consegue interpretar depois.
		var doomed database.Site
		found := database.DB.Where("id = ?", id).First(&doomed).Error == nil

		// Hosts e plantas ficam sem unidade em vez de sumirem junto: o
		// inventário é o registro de campo, a unidade é só o agrupamento.
		database.DB.Model(&database.NetworkHost{}).Where("site_id = ?", id).Update("site_id", nil)
		database.DB.Model(&database.FloorPlan{}).Where("site_id = ?", id).Update("site_id", nil)

		if err := database.DB.Where("id = ?", id).Delete(&database.Site{}).Error; err != nil {
			log.Printf("[API] erro ao remover unidade %s: %v", id, err)
			writeError(w, http.StatusInternalServerError, "falha ao remover a unidade")
			return
		}
		if found {
			auditTarget(r, "site", strconv.FormatUint(uint64(doomed.ID), 10), doomed.Name, &doomed.ID)
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

// networkHostUpdateHandler grava os campos cadastrais de um host.
// PATCH /api/network/host?ip=...
//
// Só toca no que o operador informa: hostname, MAC, portas e datas continuam
// sendo território exclusivo da varredura.
func networkHostUpdateHandler(w http.ResponseWriter, r *http.Request) {
	ip := strings.TrimSpace(r.URL.Query().Get("ip"))
	if ip == "" {
		writeError(w, http.StatusBadRequest, "ip é obrigatório")
		return
	}

	sess := sessionFrom(r)

	var current database.NetworkHost
	if err := database.DB.Where("ip = ?", ip).First(&current).Error; err != nil {
		writeError(w, http.StatusNotFound, "host não encontrado no inventário")
		return
	}
	// O cadastro é editável por quem é Suporte TI na unidade atual do host;
	// host sem unidade exige acesso global.
	if !auth.Allows(auth.RoleForSite(sess.Accesses, current.SiteID), auth.RoleOperator) {
		writeError(w, http.StatusForbidden, "este host está fora do seu alcance")
		return
	}

	var req struct {
		// SiteID é json.RawMessage porque *uint não distingue "campo ausente"
		// de "campo enviado como null", e os dois significam coisas opostas
		// aqui: ausente não mexe na unidade, null devolve o host ao controle
		// automático do coletor.
		SiteID     json.RawMessage `json:"site_id"`
		Floor      *string         `json:"floor"`
		Sector     *string         `json:"sector"`
		Room       *string         `json:"room"`
		Rack       *string         `json:"rack"`
		AssetTag   *string         `json:"asset_tag"`
		Owner      *string         `json:"owner"`
		Notes      *string         `json:"notes"`
		DeviceType *string         `json:"device_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	updates := map[string]any{}

	// json.RawMessage SEM ponteiro é o que distingue "campo ausente" de
	// "campo enviado como null": com ponteiro, o encoding/json zera o ponteiro
	// nos dois casos e o destravar por null nunca chegava aqui.
	if len(req.SiteID) > 0 {
		siteID, ok := parseOptionalUint(req.SiteID)
		if !ok {
			writeError(w, http.StatusBadRequest, "site_id inválido: informe um número ou null")
			return
		}
		// Mover o host exige alcance também na unidade de destino; devolver ao
		// automático (null) é avaliado contra o escopo sem unidade.
		if !auth.Allows(auth.RoleForSite(sess.Accesses, siteID), auth.RoleOperator) {
			writeError(w, http.StatusForbidden, "a unidade de destino está fora do seu alcance")
			return
		}
		if siteID == nil {
			// Destravar: a unidade volta a ser definida pelo coletor.
			updates["site_id"] = nil
			updates["site_locked"] = false
		} else {
			updates["site_id"] = *siteID
			updates["site_locked"] = true
		}
	}

	// device_type vazio destrava e reinfere na hora pelas portas já gravadas;
	// esperar a próxima varredura deixaria o campo desatualizado por um ciclo.
	if req.DeviceType != nil {
		if chosen := strings.TrimSpace(*req.DeviceType); chosen == "" {
			updates["device_type"] = discovery.DeviceType(discovery.ParsePorts(current.OpenPorts))
			updates["device_type_locked"] = false
		} else {
			updates["device_type"] = chosen
			updates["device_type_locked"] = true
		}
	}

	for field, value := range map[string]*string{
		"floor": req.Floor, "sector": req.Sector, "room": req.Room, "rack": req.Rack,
		"asset_tag": req.AssetTag, "owner": req.Owner, "notes": req.Notes,
	} {
		if value != nil {
			updates[field] = strings.TrimSpace(*value)
		}
	}
	if len(updates) == 0 {
		writeError(w, http.StatusBadRequest, "nenhum campo para atualizar")
		return
	}

	res := database.DB.Model(&database.NetworkHost{}).Where("ip = ?", ip).Updates(updates)
	if res.Error != nil {
		log.Printf("[API] erro ao atualizar o host %s: %v", ip, res.Error)
		writeError(w, http.StatusInternalServerError, "falha ao atualizar o host")
		return
	}
	if res.RowsAffected == 0 {
		writeError(w, http.StatusNotFound, "host não encontrado no inventário")
		return
	}

	var host database.NetworkHost
	if err := database.DB.Where("ip = ?", ip).First(&host).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "falha ao reler o host")
		return
	}
	// A unidade registrada é a de DEPOIS da edição: é o estado que esta ação
	// produziu. Quem audita a unidade de origem de um host movido não o
	// encontra por aqui — ver a limitação anotada no relatório do item N5.
	auditTarget(r, "network-host", host.IP, host.Hostname, host.SiteID)
	writeJSON(w, http.StatusOK, host)
}

// parseOptionalUint interpreta o conteúdo bruto de um campo JSON que aceita
// número ou null. Devolve (nil, true) para null e (nil, false) para lixo.
func parseOptionalUint(raw json.RawMessage) (*uint, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "null" || trimmed == "" {
		return nil, true
	}
	var n uint
	if err := json.Unmarshal(raw, &n); err != nil || n == 0 {
		return nil, false
	}
	return &n, true
}
