package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/discovery"
)

func sitesHandler(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)

	switch r.Method {
	case http.MethodGet:
		tx := database.DB.Order("name ASC")
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
		var doomed database.Site
		found := database.DB.Where("id = ?", id).First(&doomed).Error == nil

		presos, err := dispositivosVivos(id)
		if err != nil {
			log.Printf("[API] erro ao conferir dispositivos da unidade %s: %v", id, err)
			writeError(w, http.StatusInternalServerError, "falha ao remover a unidade")
			return
		}
		if presos != "" {
			writeError(w, http.StatusConflict,
				"a unidade ainda tem "+presos+"; revogue antes de remover, senão o dispositivo continuaria enviando dados para uma unidade que não existe mais")
			return
		}

		if err := removerUnidadeEReferencias(id); err != nil {
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

func dispositivosVivos(id string) (string, error) {
	var credenciais, convites int64
	if err := database.DB.Model(&database.DeviceCredential{}).
		Where("site_id = ? AND revoked_at IS NULL", id).Count(&credenciais).Error; err != nil {
		return "", err
	}
	if err := database.DB.Model(&database.EnrollmentToken{}).
		Where("site_id = ? AND used_at IS NULL AND expires_at > ?", id, time.Now().UTC()).
		Count(&convites).Error; err != nil {
		return "", err
	}

	partes := make([]string, 0, 2)
	if credenciais > 0 {
		partes = append(partes, strconv.FormatInt(credenciais, 10)+" dispositivo(s) com credencial ativa")
	}
	if convites > 0 {
		partes = append(partes, strconv.FormatInt(convites, 10)+" convite(s) válido(s)")
	}
	return strings.Join(partes, " e "), nil
}

func removerUnidadeEReferencias(id string) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&database.Server{}).Where("site_id = ?", id).Update("site_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Model(&database.AlertRule{}).Where("target_site_id = ?", id).Update("target_site_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Model(&database.NetworkHost{}).Where("site_id = ?", id).Update("site_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Model(&database.FloorPlan{}).Where("site_id = ?", id).Update("site_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Where("site_id = ?", id).Delete(&database.UserSiteAccess{}).Error; err != nil {
			return err
		}
		if err := tx.Where("site_id = ?", id).Delete(&database.DeviceCredential{}).Error; err != nil {
			return err
		}
		if err := tx.Where("site_id = ?", id).Delete(&database.EnrollmentToken{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&database.Site{}).Error
	})
}

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
	if !auth.Allows(auth.RoleForSite(sess.Accesses, current.SiteID), auth.RoleOperator) {
		writeError(w, http.StatusForbidden, "este host está fora do seu alcance")
		return
	}

	var req struct {
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

	if len(req.SiteID) > 0 {
		siteID, ok := parseOptionalUint(req.SiteID)
		if !ok {
			writeError(w, http.StatusBadRequest, "site_id inválido: informe um número ou null")
			return
		}
		if !auth.Allows(auth.RoleForSite(sess.Accesses, siteID), auth.RoleOperator) {
			writeError(w, http.StatusForbidden, "a unidade de destino está fora do seu alcance")
			return
		}
		if siteID == nil {
			updates["site_id"] = nil
			updates["site_locked"] = false
		} else {
			updates["site_id"] = *siteID
			updates["site_locked"] = true
		}
	}

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
	auditTarget(r, "network-host", host.IP, host.Hostname, host.SiteID)
	writeJSON(w, http.StatusOK, host)
}

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
