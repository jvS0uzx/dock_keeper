package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/audit"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/discovery"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	maxInventoryHosts = 5000
	maxPortasPorHost  = 4096
)

type inventoryHost struct {
	IP        string `json:"ip"`
	Hostname  string `json:"hostname"`
	MAC       string `json:"mac"`
	OpenPorts []int  `json:"open_ports"`
}

type inventoryPayload struct {
	SiteCode         string          `json:"site_code"`
	CollectorVersion string          `json:"collector_version"`
	Hosts            []inventoryHost `json:"hosts"`

	ReportIntervalSec int `json:"report_interval_sec"`
}

var errTooManyHosts = errors.New("inventário grande demais")

func decodeInventoryPayload(r io.Reader) (inventoryPayload, error) {
	var p inventoryPayload
	dec := json.NewDecoder(r)

	tok, err := dec.Token()
	if err != nil {
		return p, err
	}
	if tok == nil {
		return p, nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return p, errors.New("corpo não é um objeto JSON")
	}

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return p, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return p, errors.New("chave de objeto inválida")
		}

		switch key {
		case "site_code":
			if err := dec.Decode(&p.SiteCode); err != nil {
				return p, err
			}
		case "collector_version":
			if err := dec.Decode(&p.CollectorVersion); err != nil {
				return p, err
			}
		case "report_interval_sec":
			if err := dec.Decode(&p.ReportIntervalSec); err != nil {
				return p, err
			}
		case "hosts":
			p.Hosts = nil
			tok, err := dec.Token()
			if err != nil {
				return p, err
			}
			if tok == nil {
				continue
			}
			if d, ok := tok.(json.Delim); !ok || d != '[' {
				return p, errors.New("hosts não é uma lista")
			}
			for dec.More() {
				if len(p.Hosts) >= maxInventoryHosts {
					return p, errTooManyHosts
				}
				var h inventoryHost
				if err := dec.Decode(&h); err != nil {
					return p, err
				}
				p.Hosts = append(p.Hosts, h)
			}
			if _, err := dec.Token(); err != nil {
				return p, err
			}
		default:
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return p, err
			}
		}
	}

	if _, err := dec.Token(); err != nil {
		return p, err
	}
	return p, nil
}

func InventoryIngestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if recusasAnonimasNoTeto(w, r) {
		return
	}
	cred, err := authenticateDevice(r)
	if err != nil {
		contarRecusaAnonima(r)
		refuseDeviceAuth(w, r, err, "inventory.legacy_token_disabled")
		return
	}
	if !cred.allowsKind(kindCollector) {
		refuseDeviceKind(w, r, cred, "inventory.kind_mismatch", kindCollector, "inventário")
		return
	}
	if !limitarTaxa(w, r, chaveDeIngestao(r, cred), tetoDeIngestao()) {
		return
	}

	p, err := decodeInventoryPayload(r.Body)
	if err != nil {
		if errors.Is(err, errTooManyHosts) {
			writeError(w, http.StatusRequestEntityTooLarge, "inventário grande demais")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	siteID, err := resolveInventorySite(cred, p.SiteCode)
	if err != nil {
		if errors.Is(err, errSiteMismatch) {
			auditInventorySiteMismatch(cred, p)
			writeError(w, http.StatusConflict, "unidade declarada não confere com a credencial do dispositivo")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	saved, err := storeInventory(p.Hosts, siteID)
	if err != nil {
		log.Printf("[Inventory] erro ao gravar o envio da unidade %q: %v", p.SiteCode, err)
		writeError(w, http.StatusInternalServerError, "falha ao gravar o inventário")
		return
	}

	if cred.DeviceID != "" && p.ReportIntervalSec > 0 {
		err := database.DB.Model(&database.DeviceCredential{}).Where("device_id = ?", cred.DeviceID).
			Update("report_interval_sec", p.ReportIntervalSec).Error
		if err != nil {
			log.Printf("[Inventory] erro ao gravar o intervalo declarado pelo coletor %s: %v", cred.DeviceID, err)
		}
	}

	log.Printf("[Inventory] unidade %q (coletor %s) enviou %d hosts, %d gravados",
		p.SiteCode, p.CollectorVersion, len(p.Hosts), saved)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "stored": saved})
}

func resolveSite(code string) (*uint, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return nil, errInvalidSite("site_code é obrigatório")
	}

	var site database.Site
	if err := database.DB.Where("code = ?", code).First(&site).Error; err != nil {
		return nil, errInvalidSite("unidade " + code + " não cadastrada no painel")
	}
	return &site.ID, nil
}

type errInvalidSite string

func (e errInvalidSite) Error() string { return string(e) }

func storeInventory(hosts []inventoryHost, siteID *uint) (int, error) {
	if len(hosts) == 0 {
		return 0, nil
	}
	now := time.Now().UTC()

	records := make([]database.NetworkHost, 0, len(hosts))
	for _, h := range hosts {
		ip := strings.TrimSpace(h.IP)
		if net.ParseIP(ip) == nil {
			continue
		}
		if len(h.OpenPorts) > maxPortasPorHost {
			log.Printf("[Inventory] host %s descartado: %d portas no envio, teto é %d", ip, len(h.OpenPorts), maxPortasPorHost)
			continue
		}
		records = append(records, database.NetworkHost{
			IP:         ip,
			Hostname:   strings.TrimSpace(h.Hostname),
			MAC:        strings.ToLower(strings.TrimSpace(h.MAC)),
			OpenPorts:  joinPorts(h.OpenPorts),
			DeviceType: discovery.DeviceType(h.OpenPorts),
			SiteID:     siteID,
			FirstSeen:  now,
			LastSeen:   now,
		})
	}
	if len(records) == 0 {
		return 0, nil
	}

	if siteID != nil {
		ips := make([]string, 0, len(records))
		for _, r := range records {
			ips = append(ips, r.IP)
		}
		if err := database.AdoptNetworkHostsWithoutSite(*siteID, ips); err != nil {
			return 0, err
		}
	}

	err := database.DB.Clauses(clause.OnConflict{
		Columns: database.NetworkHostConflictTarget(),
		DoUpdates: clause.Assignments(map[string]any{
			"last_seen":   now,
			"open_ports":  gorm.Expr("EXCLUDED.open_ports"),
			"device_type": gorm.Expr("CASE WHEN network_hosts.device_type_locked THEN network_hosts.device_type ELSE EXCLUDED.device_type END"),
			"site_id":     gorm.Expr("CASE WHEN network_hosts.site_locked THEN network_hosts.site_id ELSE COALESCE(EXCLUDED.site_id, network_hosts.site_id) END"),
			"hostname":    gorm.Expr("COALESCE(NULLIF(EXCLUDED.hostname, ''), network_hosts.hostname)"),
			"mac":         gorm.Expr("COALESCE(NULLIF(EXCLUDED.mac, ''), network_hosts.mac)"),
		}),
	}).Create(&records).Error
	if err != nil {
		return 0, err
	}
	return len(records), nil
}

func joinPorts(ports []int) string {
	return discovery.JoinPorts(ports)
}

var errSiteMismatch = errors.New("unidade declarada diverge da credencial")

func resolveInventorySite(cred deviceAuth, siteCode string) (*uint, error) {
	if cred.SiteID == nil {
		return resolveSite(siteCode)
	}

	if strings.TrimSpace(siteCode) != "" {
		declarada, err := resolveSite(siteCode)
		if err != nil {
			return nil, err
		}
		if !cred.siteMatches(declarada) {
			return nil, errSiteMismatch
		}
	}

	site := *cred.SiteID
	return &site, nil
}

func auditInventorySiteMismatch(cred deviceAuth, p inventoryPayload) {
	audit.Record(audit.Entry{
		Action:     "inventory.site_mismatch",
		TargetType: "device",
		TargetID:   cred.DeviceID,
		SiteID:     cred.SiteID,
		Result:     audit.ResultDenied,
		Detail: map[string]any{
			"site_code_no_envio": p.SiteCode,
			"hosts_no_envio":     len(p.Hosts),
			"collector_version":  p.CollectorVersion,
		},
	})
}
