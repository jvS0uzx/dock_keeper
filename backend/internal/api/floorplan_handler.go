package api

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"gorm.io/gorm"
)

const maxPlanUploadBytes = maxPlanBytes + (1 << 20)

type PinView struct {
	ID           uint    `json:"id"`
	HostIP       string  `json:"host_ip"`
	Label        string  `json:"label"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	TargetPlanID *uint   `json:"target_plan_id"`

	Hostname   string `json:"hostname"`
	DeviceType string `json:"device_type"`
	Online     bool   `json:"online"`
	Monitored  bool   `json:"monitored"`
	Known      bool   `json:"known"`
	ServerID   string `json:"server_id"`
}

type FloorPlanView struct {
	ID          uint      `json:"id"`
	SiteID      *uint     `json:"site_id"`
	Name        string    `json:"name"`
	ContentType string    `json:"content_type"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	CreatedAt   time.Time `json:"created_at"`
	Pins        []PinView `json:"pins"`
}

func floorPlansHandler(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)

	switch r.Method {
	case http.MethodGet:
		tx := database.DB.Order("name ASC")
		if !auth.HasGlobal(sess.Accesses) {
			ids := auth.SiteIDs(sess.Accesses)
			if len(ids) == 0 {
				writeJSON(w, http.StatusOK, []FloorPlanView{})
				return
			}
			tx = tx.Where("site_id IN ?", ids)
		}
		var plans []database.FloorPlan
		if err := tx.Find(&plans).Error; err != nil {
			log.Printf("[API] erro ao listar plantas: %v", err)
			writeError(w, http.StatusInternalServerError, "falha ao listar plantas")
			return
		}
		views := make([]FloorPlanView, 0, len(plans))
		for _, p := range plans {
			views = append(views, FloorPlanView{
				ID: p.ID, SiteID: p.SiteID, Name: p.Name, ContentType: p.ContentType,
				Width: p.Width, Height: p.Height, CreatedAt: p.CreatedAt, Pins: []PinView{},
			})
		}
		writeJSON(w, http.StatusOK, views)

	case http.MethodPost:
		createFloorPlan(w, r)
	}
}

func createFloorPlan(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	r.Body = http.MaxBytesReader(w, r.Body, maxPlanUploadBytes)
	if err := r.ParseMultipartForm(maxPlanUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "envio inválido ou grande demais")
		return
	}
	defer r.MultipartForm.RemoveAll()

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "name é obrigatório")
		return
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "arquivo 'image' é obrigatório")
		return
	}
	defer file.Close()

	stored, err := storePlanImage(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	siteID := optionalUint(r.FormValue("site_id"))
	if siteID == nil {
		removePlanImage(stored.Path)
		writeError(w, http.StatusBadRequest, "site_id é obrigatório: a planta pertence a uma unidade")
		return
	}
	var site database.Site
	if err := database.DB.First(&site, *siteID).Error; err != nil {
		removePlanImage(stored.Path)
		writeError(w, http.StatusBadRequest, "unidade inexistente")
		return
	}
	if !auth.Allows(auth.RoleForSite(sess.Accesses, siteID), auth.RoleOperator) {
		removePlanImage(stored.Path)
		writeError(w, http.StatusForbidden, "esta unidade está fora do seu alcance")
		return
	}

	plan := database.FloorPlan{
		Name:        name,
		SiteID:      siteID,
		ImagePath:   stored.Path,
		ContentType: stored.ContentType,
		Width:       stored.Width,
		Height:      stored.Height,
	}
	if err := database.DB.Create(&plan).Error; err != nil {
		removePlanImage(stored.Path)
		log.Printf("[API] erro ao cadastrar planta %q: %v", name, err)
		writeError(w, http.StatusInternalServerError, "falha ao cadastrar a planta")
		return
	}

	writeJSON(w, http.StatusCreated, FloorPlanView{
		ID: plan.ID, SiteID: plan.SiteID, Name: plan.Name, ContentType: plan.ContentType,
		Width: plan.Width, Height: plan.Height, CreatedAt: plan.CreatedAt, Pins: []PinView{},
	})
}

func floorPlanHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := planIDFromPath(w, r, "")
	if !ok {
		return
	}

	sess := sessionFrom(r)
	var plan database.FloorPlan
	if err := database.DB.First(&plan, id).Error; err != nil {
		writeError(w, http.StatusNotFound, "planta não encontrada")
		return
	}
	if !auth.CanSeeSite(sess.Accesses, plan.SiteID) {
		writeError(w, http.StatusNotFound, "planta não encontrada")
		return
	}

	switch r.Method {
	case http.MethodGet:
		pins, err := pinsWithState(plan)
		if err != nil {
			log.Printf("[API] erro ao montar pins da planta %d: %v", plan.ID, err)
			writeError(w, http.StatusInternalServerError, "falha ao ler os marcadores")
			return
		}
		writeJSON(w, http.StatusOK, FloorPlanView{
			ID: plan.ID, SiteID: plan.SiteID, Name: plan.Name, ContentType: plan.ContentType,
			Width: plan.Width, Height: plan.Height, CreatedAt: plan.CreatedAt, Pins: pins,
		})

	case http.MethodDelete:
		if !auth.Allows(auth.RoleForSite(sess.Accesses, plan.SiteID), auth.RoleOperator) {
			writeError(w, http.StatusForbidden, "esta planta está fora do seu alcance")
			return
		}
		if err := database.DB.Where("plan_id = ?", plan.ID).Delete(&database.FloorPlanPin{}).Error; err != nil {
			log.Printf("[API] erro ao remover pins da planta %d: %v", plan.ID, err)
			writeError(w, http.StatusInternalServerError, "falha ao remover a planta")
			return
		}
		if err := database.DB.Delete(&plan).Error; err != nil {
			log.Printf("[API] erro ao remover a planta %d: %v", plan.ID, err)
			writeError(w, http.StatusInternalServerError, "falha ao remover a planta")
			return
		}
		removePlanImage(plan.ImagePath)
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

func floorPlanImageHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := planIDFromPath(w, r, "/image")
	if !ok {
		return
	}

	sess := sessionFrom(r)
	var plan database.FloorPlan
	if err := database.DB.First(&plan, id).Error; err != nil {
		writeError(w, http.StatusNotFound, "planta não encontrada")
		return
	}
	if !auth.CanSeeSite(sess.Accesses, plan.SiteID) {
		writeError(w, http.StatusNotFound, "planta não encontrada")
		return
	}

	data, err := os.ReadFile(plan.ImagePath)
	if err != nil {
		log.Printf("[API] planta %d sem arquivo em disco (%s): %v", plan.ID, plan.ImagePath, err)
		writeError(w, http.StatusNotFound, "imagem indisponível")
		return
	}

	w.Header().Set("Content-Type", plan.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "inline")
	if _, err := w.Write(data); err != nil {
		log.Printf("[API] erro ao enviar a planta %d: %v", plan.ID, err)
	}
}

func floorPlanPinsHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := planIDFromPath(w, r, "/pins")
	if !ok {
		return
	}

	sess := sessionFrom(r)
	var plan database.FloorPlan
	if err := database.DB.First(&plan, id).Error; err != nil {
		writeError(w, http.StatusNotFound, "planta não encontrada")
		return
	}
	if !auth.Allows(auth.RoleForSite(sess.Accesses, plan.SiteID), auth.RoleOperator) {
		writeError(w, http.StatusForbidden, "esta planta está fora do seu alcance")
		return
	}

	var body struct {
		Pins []struct {
			HostIP       string  `json:"host_ip"`
			Label        string  `json:"label"`
			X            float64 `json:"x"`
			Y            float64 `json:"y"`
			TargetPlanID *uint   `json:"target_plan_id"`
		} `json:"pins"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	pins := make([]database.FloorPlanPin, 0, len(body.Pins))
	for _, p := range body.Pins {
		if p.HostIP == "" && p.TargetPlanID == nil {
			writeError(w, http.StatusBadRequest, "cada pin precisa de host_ip ou target_plan_id")
			return
		}
		if p.TargetPlanID != nil && *p.TargetPlanID == plan.ID {
			writeError(w, http.StatusBadRequest, "um pin não pode apontar para a própria planta")
			return
		}
		if p.HostIP != "" && net.ParseIP(p.HostIP) == nil {
			writeError(w, http.StatusBadRequest, "host_ip inválido: "+p.HostIP)
			return
		}
		pins = append(pins, database.FloorPlanPin{
			PlanID:       plan.ID,
			HostIP:       p.HostIP,
			Label:        strings.TrimSpace(p.Label),
			X:            clampPercent(p.X),
			Y:            clampPercent(p.Y),
			TargetPlanID: p.TargetPlanID,
		})
	}

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("plan_id = ?", plan.ID).Delete(&database.FloorPlanPin{}).Error; err != nil {
			return err
		}
		if len(pins) == 0 {
			return nil
		}
		return tx.Create(&pins).Error
	})
	if err != nil {
		log.Printf("[API] erro ao gravar pins da planta %d: %v", plan.ID, err)
		writeError(w, http.StatusInternalServerError, "falha ao gravar os marcadores")
		return
	}

	saved, err := pinsWithState(plan)
	if err != nil {
		log.Printf("[API] erro ao reler pins da planta %d: %v", plan.ID, err)
		writeError(w, http.StatusInternalServerError, "falha ao reler os marcadores")
		return
	}
	if n := unknownPins(saved); n > 0 {
		log.Printf("[API] planta %d (unidade %v): %d marcador(es) apontam para endereço fora do inventário da unidade",
			plan.ID, plan.SiteID, n)
	}
	writeJSON(w, http.StatusOK, map[string]any{"pins": saved})
}

func pinsWithState(plan database.FloorPlan) ([]PinView, error) {
	var pins []database.FloorPlanPin
	if err := database.DB.Where("plan_id = ?", plan.ID).Order("id ASC").Find(&pins).Error; err != nil {
		return nil, err
	}

	byIP, err := hostsOfSite(plan.SiteID, pinIPs(pins))
	if err != nil {
		return nil, err
	}

	monitored, err := monitoredServersOfSite(plan.SiteID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	views := make([]PinView, 0, len(pins))
	for _, p := range pins {
		view := PinView{
			ID: p.ID, HostIP: p.HostIP, Label: p.Label,
			X: p.X, Y: p.Y, TargetPlanID: p.TargetPlanID,
		}
		if host, ok := byIP[p.HostIP]; ok {
			view.Known = true
			view.Hostname = host.Hostname
			view.DeviceType = host.DeviceType
			view.Online = hostOnline(host.LastSeen, now)
			_, view.Monitored = monitored.lookup(host.IP, host.Hostname)
			view.ServerID = monitored.serverID(host.IP, host.Hostname)
		}
		views = append(views, view)
	}
	return views, nil
}

func unknownPins(views []PinView) int {
	n := 0
	for _, v := range views {
		if v.HostIP != "" && !v.Known {
			n++
		}
	}
	return n
}

func siteKey(siteID *uint) uint {
	if siteID == nil {
		return 0
	}
	return *siteID
}

func pinIPs(pins []database.FloorPlanPin) []string {
	ips := make([]string, 0, len(pins))
	visto := make(map[string]struct{}, len(pins))
	for _, p := range pins {
		if p.HostIP == "" {
			continue
		}
		if _, dup := visto[p.HostIP]; dup {
			continue
		}
		visto[p.HostIP] = struct{}{}
		ips = append(ips, p.HostIP)
	}
	return ips
}

func hostsOfSite(siteID *uint, ips []string) (map[string]database.NetworkHost, error) {
	byIP := make(map[string]database.NetworkHost, len(ips))
	if len(ips) == 0 {
		return byIP, nil
	}

	var hosts []database.NetworkHost
	if err := database.DB.
		Where("ip IN ? AND COALESCE(site_id, 0) = ?", ips, siteKey(siteID)).
		Find(&hosts).Error; err != nil {
		return nil, err
	}
	for _, h := range hosts {
		byIP[h.IP] = h
	}
	return byIP, nil
}

func monitoredServersOfSite(siteID *uint) (serverIndex, error) {
	var servers []database.Server
	if err := database.DB.
		Where("COALESCE(site_id, 0) = ?", siteKey(siteID)).
		Find(&servers).Error; err != nil {
		return serverIndex{}, err
	}

	return indexServers(servers), nil
}

func planIDFromPath(w http.ResponseWriter, r *http.Request, suffix string) (uint, bool) {
	raw := strings.TrimPrefix(r.URL.Path, "/api/floorplans/")
	raw = strings.TrimSuffix(raw, suffix)
	raw = strings.Trim(raw, "/")

	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		writeError(w, http.StatusBadRequest, "id da planta inválido")
		return 0, false
	}
	return uint(id), true
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func optionalUint(raw string) *uint {
	n, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil || n == 0 {
		return nil
	}
	v := uint(n)
	return &v
}
