package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	maxDashboardsPerUser = 20
	maxPanelsPerBoard    = 12
	maxBoardNameRunes    = 80
	maxPanelTitleRunes   = 80
)

var validPanelMetrics = map[string]bool{
	"cpu": true, "mem": true, "disk": true, "load": true,
	"temperature": true, "net_rx": true, "net_tx": true, "rtt": true,
}

var errServerOutOfReach = errors.New("servidor não encontrado")

type panelInput struct {
	Title    string `json:"title"`
	ServerID string `json:"server_id"`
	Metric   string `json:"metric"`
	Range    string `json:"range"`
	Width    int    `json:"width"`
}

type dashboardInput struct {
	Name   string       `json:"name"`
	Panels []panelInput `json:"panels"`
}

type panelView struct {
	ID       uint   `json:"id"`
	Position int    `json:"position"`
	Title    string `json:"title"`
	ServerID string `json:"server_id"`
	Metric   string `json:"metric"`
	Range    string `json:"range"`
	Width    int    `json:"width"`
}

type dashboardView struct {
	ID        uint        `json:"id"`
	Name      string      `json:"name"`
	Panels    []panelView `json:"panels"`
	UpdatedAt time.Time   `json:"updated_at"`
}

func requireUserSession(w http.ResponseWriter, r *http.Request, msg string) (auth.Session, bool) {
	sess := sessionFrom(r)
	if sess.UserID == 0 {
		writeError(w, http.StatusForbidden, msg)
		return sess, false
	}
	return sess, true
}

func dashboardsHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := requireUserSession(w, r, "painéis exigem sessão de usuário")
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		listDashboards(w, sess)
	case http.MethodPost:
		saveDashboard(w, r, sess, nil)
	case http.MethodPut:
		board, found := ownedDashboard(w, r, sess)
		if found {
			saveDashboard(w, r, sess, &board)
		}
	case http.MethodDelete:
		board, found := ownedDashboard(w, r, sess)
		if !found {
			return
		}
		if err := database.DB.Delete(&board).Error; err != nil {
			log.Printf("[Dashboards] erro ao remover %d: %v", board.ID, err)
			writeError(w, http.StatusInternalServerError, "falha ao remover o dashboard")
			return
		}
		auditTarget(r, "dashboard", strconv.FormatUint(uint64(board.ID), 10), board.Name, nil)
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

func ownedDashboard(w http.ResponseWriter, r *http.Request, sess auth.Session) (database.Dashboard, bool) {
	var board database.Dashboard
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "id é obrigatório")
		return board, false
	}
	err := database.DB.Where("id = ? AND owner_user_id = ?", id, sess.UserID).First(&board).Error
	if err != nil {
		writeError(w, http.StatusNotFound, "dashboard não encontrado")
		return board, false
	}
	return board, true
}

func listDashboards(w http.ResponseWriter, sess auth.Session) {
	var boards []database.Dashboard
	err := database.DB.Where("owner_user_id = ?", sess.UserID).
		Preload("Panels", func(db *gorm.DB) *gorm.DB { return db.Order("position ASC") }).
		Order("id ASC").Find(&boards).Error
	if err != nil {
		log.Printf("[Dashboards] erro ao listar: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao listar os dashboards")
		return
	}

	sites, err := panelServerSites(boards)
	if err != nil {
		log.Printf("[Dashboards] erro ao resolver servidores: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao listar os dashboards")
		return
	}

	out := make([]dashboardView, 0, len(boards))
	for _, b := range boards {
		out = append(out, viewOf(b, func(p database.DashboardPanel) bool {
			site, exists := sites[p.ServerID]
			return exists && auth.CanSeeSite(sess.Accesses, site)
		}))
	}
	writeJSON(w, http.StatusOK, out)
}

func panelServerSites(boards []database.Dashboard) (map[string]*uint, error) {
	var ids []string
	for _, b := range boards {
		for _, p := range b.Panels {
			ids = append(ids, p.ServerID)
		}
	}
	sites := map[string]*uint{}
	if len(ids) == 0 {
		return sites, nil
	}
	var servers []database.Server
	if err := database.DB.Select("id", "site_id").Where("id IN ?", ids).Find(&servers).Error; err != nil {
		return nil, err
	}
	for _, s := range servers {
		sites[s.ID] = s.SiteID
	}
	return sites, nil
}

func viewOf(b database.Dashboard, visible func(database.DashboardPanel) bool) dashboardView {
	v := dashboardView{ID: b.ID, Name: b.Name, UpdatedAt: b.UpdatedAt, Panels: []panelView{}}
	for _, p := range b.Panels {
		if !visible(p) {
			continue
		}
		v.Panels = append(v.Panels, panelView{
			ID: p.ID, Position: p.Position, Title: p.Title, ServerID: p.ServerID,
			Metric: p.Metric, Range: p.Range, Width: p.Width,
		})
	}
	return v
}

func validateDashboard(in *dashboardInput) string {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return "name é obrigatório"
	}
	if len([]rune(in.Name)) > maxBoardNameRunes {
		return "name passa de 80 caracteres"
	}
	if len(in.Panels) > maxPanelsPerBoard {
		return "um dashboard aceita no máximo 12 painéis"
	}
	for i := range in.Panels {
		p := &in.Panels[i]
		p.Title = strings.TrimSpace(p.Title)
		p.ServerID = strings.TrimSpace(p.ServerID)
		if p.Width == 0 {
			p.Width = 1
		}
		switch {
		case p.ServerID == "":
			return "server_id é obrigatório em cada painel"
		case len([]rune(p.Title)) > maxPanelTitleRunes:
			return "title passa de 80 caracteres"
		case !validPanelMetrics[p.Metric]:
			return "métrica inválida"
		case historyRanges[p.Range] == 0:
			return "range inválido"
		case p.Width != 1 && p.Width != 2:
			return "width precisa ser 1 ou 2"
		}
	}
	return ""
}

func checkPanelServers(sess auth.Session, panels []panelInput) error {
	seen := map[string]bool{}
	for _, p := range panels {
		if seen[p.ServerID] {
			continue
		}
		seen[p.ServerID] = true
		var server database.Server
		if err := database.DB.Select("id", "site_id").Where("id = ?", p.ServerID).First(&server).Error; err != nil {
			return errServerOutOfReach
		}
		if !auth.CanSeeSite(sess.Accesses, server.SiteID) {
			return errServerOutOfReach
		}
	}
	return nil
}

func saveDashboard(w http.ResponseWriter, r *http.Request, sess auth.Session, existing *database.Dashboard) {
	var in dashboardInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if msg := validateDashboard(&in); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if err := checkPanelServers(sess, in.Panels); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	nameTaken := database.DB.Where("owner_user_id = ? AND name = ?", sess.UserID, in.Name)
	if existing != nil {
		nameTaken = nameTaken.Where("id <> ?", existing.ID)
	}
	var clash int64
	if err := nameTaken.Model(&database.Dashboard{}).Count(&clash).Error; err == nil && clash > 0 {
		writeError(w, http.StatusConflict, "você já tem um dashboard com esse nome")
		return
	}
	if existing == nil {
		var total int64
		database.DB.Model(&database.Dashboard{}).Where("owner_user_id = ?", sess.UserID).Count(&total)
		if total >= maxDashboardsPerUser {
			writeError(w, http.StatusBadRequest, "limite de 20 dashboards por usuário atingido")
			return
		}
	}

	board := database.Dashboard{OwnerUserID: sess.UserID, Name: in.Name}
	if existing != nil {
		board = *existing
		board.Name = in.Name
	}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if existing == nil {
			if err := tx.Omit("Panels").Create(&board).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&board).Updates(map[string]any{"name": board.Name, "updated_at": time.Now().UTC()}).Error; err != nil {
				return err
			}
			if err := tx.Where("dashboard_id = ?", board.ID).Delete(&database.DashboardPanel{}).Error; err != nil {
				return err
			}
		}
		board.Panels = make([]database.DashboardPanel, 0, len(in.Panels))
		for i, p := range in.Panels {
			board.Panels = append(board.Panels, database.DashboardPanel{
				DashboardID: board.ID, Position: i, Title: p.Title, ServerID: p.ServerID,
				Metric: p.Metric, Range: p.Range, Width: p.Width,
			})
		}
		if len(board.Panels) > 0 {
			return tx.Create(&board.Panels).Error
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "idx_dashboard_owner_name") {
			writeError(w, http.StatusConflict, "você já tem um dashboard com esse nome")
			return
		}
		log.Printf("[Dashboards] erro ao gravar %q: %v", in.Name, err)
		writeError(w, http.StatusInternalServerError, "falha ao gravar o dashboard")
		return
	}

	database.DB.First(&board, board.ID)
	auditTarget(r, "dashboard", strconv.FormatUint(uint64(board.ID), 10), board.Name, nil)
	status := http.StatusOK
	if existing == nil {
		status = http.StatusCreated
	}
	writeJSON(w, status, viewOf(board, func(database.DashboardPanel) bool { return true }))
}
