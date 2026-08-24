package api

import (
	"context"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/joaov/vd_stats/internal/database"
	"github.com/joaov/vd_stats/internal/discovery"
)

// Host sem contato há mais tempo que isto aparece como offline no inventário.
const hostOfflineAfter = 30 * time.Minute

type NetworkHostView struct {
	IP        string    `json:"ip"`
	Hostname  string    `json:"hostname"`
	MAC       string    `json:"mac"`
	OpenPorts []string  `json:"open_ports"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Online    bool      `json:"online"`
	Monitored bool      `json:"monitored"` // já cadastrado como servidor do painel
	Kind      string    `json:"kind"`      // "ssh" | "agent" | ""

	// Inventário: inferido pela varredura e completado pelo operador.
	DeviceType string `json:"device_type"`
	SiteID     *uint  `json:"site_id"`
	// Travas do operador. O painel usa para mostrar o cadeado e para abrir o
	// campo em "Automático" quando ninguém fixou nada.
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

// networkHostsHandler devolve o inventário da rede cruzado com os servidores
// já monitorados — é o que responde "quais máquinas ainda não têm agente".
func networkHostsHandler(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	scope, status := resolveScope(sess, r)
	if status != 0 {
		writeError(w, status, "site_id inválido ou fora do seu alcance")
		return
	}

	var hosts []database.NetworkHost
	if err := scope.apply(database.DB).Find(&hosts).Error; err != nil {
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
	cutoff := time.Now().UTC().Add(-hostOfflineAfter)

	for _, h := range hosts {
		kind, isMonitored := monitored.lookup(h.IP, h.Hostname)
		view := NetworkHostView{
			IP:        h.IP,
			Hostname:  h.Hostname,
			MAC:       h.MAC,
			OpenPorts: splitPorts(h.OpenPorts),
			FirstSeen: h.FirstSeen,
			LastSeen:  h.LastSeen,
			Online:    h.LastSeen.After(cutoff),
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
	// Ordena por endereço numérico. ORDER BY no banco compararia texto, e aí
	// 172.16.1.100 viria antes de 172.16.1.9.
	sort.Slice(inventory.Hosts, func(i, j int) bool {
		return lessIP(inventory.Hosts[i].IP, inventory.Hosts[j].IP)
	})
	inventory.Total = len(inventory.Hosts)

	if last := discovery.Default.LastRun(); !last.IsZero() {
		inventory.LastScan = &last
	}
	inventory.ScanActive = discovery.Default.Enabled()

	writeJSON(w, http.StatusOK, inventory)
}

// networkScanHandler dispara uma varredura fora do ciclo automático.
func networkScanHandler(w http.ResponseWriter, r *http.Request) {
	if !discovery.Default.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "inventário desligado: defina DISCOVERY_CIDRS no .env")
		return
	}
	// A varredura leva segundos; responde já e roda em background — por isso
	// não herda o contexto da requisição, que morre com a resposta. O Sweeper
	// recusa varreduras concorrentes por conta própria.
	go discovery.Default.Run(context.Background())
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "scanning"})
}

// serverIndex cruza o inventário com os servidores já monitorados.
//
// O IP sozinho não basta: o agente push se registra com o endereço de onde a
// requisição saiu, que pode diferir do IP visto na varredura (localhost, NAT,
// DHCP). O nome do host serve de segunda chave.
type serverIndex struct {
	byIP   map[string]string
	byName map[string]string
	// id do Server por IP e por nome curto, para o painel poder abrir a tela
	// da máquina a partir de um marcador da planta.
	idByIP   map[string]string
	idByName map[string]string
}

// serverID resolve o id do Server correspondente ao host do inventário.
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
	// Compara só o rótulo curto: a varredura devolve FQDN, o agente manda o
	// hostname puro.
	short := strings.ToLower(strings.SplitN(hostname, ".", 2)[0])
	kind, ok := idx.byName[short]
	return kind, ok
}

// monitoredServers indexa os servidores que a sessão alcança.
//
// O recorte não é enfeite: Server.HostIP não é único, e desde que o inventário
// passou a admitir o mesmo IP em duas unidades, indexar o parque inteiro fazia
// o host da filial A ser anotado com o server_id do servidor homônimo da filial
// B — o mesmo defeito que o marcador da planta baixa tinha.
func monitoredServers(scope siteScope) (serverIndex, error) {
	var servers []database.Server
	if err := scope.apply(database.DB.Model(&database.Server{})).
		Find(&servers).Error; err != nil {
		return serverIndex{}, err
	}
	return indexServers(servers), nil
}

// indexServers monta o índice a partir de uma lista já recortada.
//
// Recebe a lista pronta, e não a consulta, para o recorte ficar visível em cada
// chamador: são dois recortes diferentes — o alcance da sessão, na tela de
// inventário, e a unidade da planta, no mapa — e esconder qual está em uso foi
// exatamente o que deixou a tela de inventário indexando o parque inteiro.
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

// lessIP compara dois IPv4 octeto a octeto.
func lessIP(a, b string) bool {
	ipA, ipB := net.ParseIP(a).To4(), net.ParseIP(b).To4()
	if ipA == nil || ipB == nil {
		return a < b
	}
	for i := range ipA {
		if ipA[i] != ipB[i] {
			return ipA[i] < ipB[i]
		}
	}
	return false
}

func splitPorts(raw string) []string {
	if raw == "" {
		return []string{}
	}
	return strings.Split(raw, ",")
}
