package api

import (
	"context"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/discovery"
	"github.com/jvS0uzx/dockkeeper_collector/scan"
)

const defaultHostOfflineAfter = 30 * time.Minute

var hostOfflineAfter = defaultHostOfflineAfter

func hostOnline(lastSeen, now time.Time) bool {
	return lastSeen.After(now.Add(-hostOfflineAfter))
}

type NetworkHostView struct {
	IP        string    `json:"ip"`
	Hostname  string    `json:"hostname"`
	MAC       string    `json:"mac"`
	OpenPorts []string  `json:"open_ports"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Online    bool      `json:"online"`
	Monitored bool      `json:"monitored"`
	Kind      string    `json:"kind"`

	DeviceType       string `json:"device_type"`
	SiteID           *uint  `json:"site_id"`
	DeviceTypeLocked bool   `json:"device_type_locked"`
	SiteLocked       bool   `json:"site_locked"`
	Floor            string `json:"floor"`
	Sector           string `json:"sector"`
	Room             string `json:"room"`
	Rack             string `json:"rack"`
	AssetTag         string `json:"asset_tag"`
	Owner            string `json:"owner"`
	Notes            string `json:"notes"`
}

type NetworkInventory struct {
	Hosts      []NetworkHostView `json:"hosts"`
	Total      int               `json:"total"`
	Online     int               `json:"online"`
	Monitored  int               `json:"monitored"`
	LastScan   *time.Time        `json:"last_scan"`
	ScanActive bool              `json:"scan_active"`
}

func networkHostsHandler(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	scope, status := resolveScope(sess, r)
	if status != 0 {
		writeError(w, status, "site_id inválido ou fora do seu alcance")
		return
	}

	var hosts []database.NetworkHost
	if err := scope.apply(database.From(r.Context())).Find(&hosts).Error; err != nil {
		log.Printf("[API] erro ao listar hosts da rede: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao ler o inventário")
		return
	}

	monitored, err := monitoredServers(scope)
	if err != nil {
		log.Printf("[API] erro ao listar servidores: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao ler servidores")
		return
	}

	inventory := NetworkInventory{Hosts: make([]NetworkHostView, 0, len(hosts))}
	now := time.Now().UTC()

	for _, h := range hosts {
		kind, isMonitored := monitored.lookup(h.IP, h.Hostname)
		view := NetworkHostView{
			IP:        h.IP,
			Hostname:  h.Hostname,
			MAC:       h.MAC,
			OpenPorts: splitPorts(h.OpenPorts),
			FirstSeen: h.FirstSeen,
			LastSeen:  h.LastSeen,
			Online:    hostOnline(h.LastSeen, now),
			Monitored: isMonitored,
			Kind:      kind,

			DeviceType:       h.DeviceType,
			SiteID:           h.SiteID,
			DeviceTypeLocked: h.DeviceTypeLocked,
			SiteLocked:       h.SiteLocked,
			Floor:            h.Floor,
			Sector:           h.Sector,
			Room:             h.Room,
			Rack:             h.Rack,
			AssetTag:         h.AssetTag,
			Owner:            h.Owner,
			Notes:            h.Notes,
		}
		if view.Online {
			inventory.Online++
		}
		if isMonitored {
			inventory.Monitored++
		}
		inventory.Hosts = append(inventory.Hosts, view)
	}
	sort.Slice(inventory.Hosts, func(i, j int) bool {
		return scan.LessIP(inventory.Hosts[i].IP, inventory.Hosts[j].IP)
	})
	inventory.Total = len(inventory.Hosts)

	if last := discovery.Default.LastRun(); !last.IsZero() {
		inventory.LastScan = &last
	}
	inventory.ScanActive = discovery.Default.Enabled()

	writeJSON(w, http.StatusOK, inventory)
}

func networkScanHandler(w http.ResponseWriter, r *http.Request) {
	if !discovery.Default.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "inventário desligado: defina DISCOVERY_CIDRS no .env")
		return
	}
	go discovery.Default.Run(context.Background())
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "scanning"})
}

type serverIndex struct {
	byIP     map[string]string
	byName   map[string]string
	idByIP   map[string]string
	idByName map[string]string
}

func (idx serverIndex) serverID(ip, hostname string) string {
	if id, ok := idx.idByIP[ip]; ok {
		return id
	}
	if hostname == "" {
		return ""
	}
	short := strings.ToLower(strings.SplitN(hostname, ".", 2)[0])
	return idx.idByName[short]
}

func (idx serverIndex) lookup(ip, hostname string) (string, bool) {
	if kind, ok := idx.byIP[ip]; ok {
		return kind, true
	}
	if hostname == "" {
		return "", false
	}
	short := strings.ToLower(strings.SplitN(hostname, ".", 2)[0])
	kind, ok := idx.byName[short]
	return kind, ok
}

func monitoredServers(scope siteScope) (serverIndex, error) {
	var servers []database.Server
	if err := scope.apply(database.DB.Model(&database.Server{})).
		Find(&servers).Error; err != nil {
		return serverIndex{}, err
	}
	return indexServers(servers), nil
}

func indexServers(servers []database.Server) serverIndex {
	idx := serverIndex{
		byIP:     make(map[string]string, len(servers)),
		byName:   make(map[string]string, len(servers)),
		idByIP:   make(map[string]string, len(servers)),
		idByName: make(map[string]string, len(servers)),
	}
	for _, s := range servers {
		idx.byIP[s.HostIP] = s.Kind
		idx.byName[strings.ToLower(s.Name)] = s.Kind
		idx.idByIP[s.HostIP] = s.ID
		idx.idByName[strings.ToLower(s.Name)] = s.ID
	}
	return idx
}

func splitPorts(raw string) []string {
	if raw == "" {
		return []string{}
	}
	return strings.Split(raw, ",")
}
