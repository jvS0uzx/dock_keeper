package api

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/joaov/vd_stats/internal/audit"
	"github.com/joaov/vd_stats/internal/database"
	"github.com/joaov/vd_stats/internal/discovery"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Teto de hosts por envio. Uma /16 inteira não cabe num ciclo de inventário e
// indica coletor mal configurado.
const maxInventoryHosts = 5000

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
}

// InventoryIngestHandler recebe o inventário varrido por um coletor remoto.
//
// É a contraparte do cmd/collector: o painel só enxerga a rede onde roda, então
// cada unidade tem um coletor que varre localmente e faz push do resultado.
// Autentica pelo mesmo X-Agent-Token da ingestão de métricas — é tráfego
// máquina-a-máquina, sem CORS de browser.
func InventoryIngestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cred, err := authenticateDevice(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "credencial de dispositivo inválida")
		return
	}

	var p inventoryPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(p.Hosts) > maxInventoryHosts {
		writeError(w, http.StatusRequestEntityTooLarge, "inventário grande demais")
		return
	}

	// Sob credencial própria a unidade sai dela, e o site_code do corpo vira
	// material de conferência. Um coletor comprometido numa filial declarando
	// outra é o caso que o item S7 existe para fechar: sob o token único ele
	// reescrevia o inventário inteiro da filial vizinha a cada ciclo.
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

	log.Printf("[Inventory] unidade %q (coletor %s) enviou %d hosts, %d gravados",
		p.SiteCode, p.CollectorVersion, len(p.Hosts), saved)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "stored": saved})
}

// resolveSite exige que a unidade já exista. Criar automaticamente permitiria
// que um token vazado poluísse o cadastro com unidades inventadas.
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

// storeInventory faz o upsert dos hosts recebidos, preservando o cadastro que
// o operador preencheu no painel.
func storeInventory(hosts []inventoryHost, siteID *uint) (int, error) {
	if len(hosts) == 0 {
		return 0, nil
	}
	now := time.Now().UTC()

	records := make([]database.NetworkHost, 0, len(hosts))
	for _, h := range hosts {
		ip := strings.TrimSpace(h.IP)
		// Endereço malformado viraria linha órfã no inventário.
		if net.ParseIP(ip) == nil {
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

	// Mesma adoção da varredura local: a linha sem unidade tem chave (0, ip) e
	// não colidiria com a do coletor, duplicando o endereço no inventário.
	if siteID != nil {
		ips := make([]string, 0, len(records))
		for _, r := range records {
			ips = append(ips, r.IP)
		}
		if err := database.AdoptNetworkHostsWithoutSite(*siteID, ips); err != nil {
			return 0, err
		}
	}

	// first_seen fica fora do DoUpdates para preservar a primeira aparição;
	// hostname e mac só sobrescrevem quando vieram preenchidos. Os campos
	// cadastrais (sala, dono, patrimônio) nunca são tocados por um coletor.
	err := database.DB.Clauses(clause.OnConflict{
		// A chave é (unidade, ip): o mesmo 192.168.0.10 em duas filiais são dois
		// equipamentos, e sob a chave antiga um coletor sobrescrevia o host do
		// outro a cada ciclo.
		Columns: database.NetworkHostConflictTarget(),
		DoUpdates: clause.Assignments(map[string]any{
			"last_seen":  now,
			"open_ports": gorm.Expr("EXCLUDED.open_ports"),
			// Travas do operador vencem o coletor. O COALESCE sozinho não
			// bastava: o coletor sempre manda a unidade dele, então EXCLUDED
			// nunca era nulo e revertia todo host movido pelo painel.
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
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		if p > 0 && p < 65536 {
			parts = append(parts, strconv.Itoa(p))
		}
	}
	return strings.Join(parts, ",")
}

// errSiteMismatch separa a divergência de unidade dos demais erros de validação:
// as duas respondem status diferente, e só uma delas é evento de segurança.
var errSiteMismatch = errors.New("unidade declarada diverge da credencial")

// resolveInventorySite decide de quem é o envio.
//
// Com credencial própria a unidade é a dela, sempre. O site_code do corpo, se
// vier, é conferido — nunca aceito. No modo legado, sem credencial que carregue
// unidade, o comportamento antigo continua: o corpo decide, porque o token
// compartilhado não sabe dizer de onde o envio veio.
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

// auditInventorySiteMismatch registra a tentativa de um coletor reivindicar
// unidade alheia. O envio é descartado inteiro: aceitar parte dele é aceitar a
// parte que o atacante escolheu.
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
