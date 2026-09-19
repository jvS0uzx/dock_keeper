package discovery

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/safego"
	"github.com/jvS0uzx/dockkeeper_collector/scan"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultInterval = 15 * time.Minute
)

type Sweeper struct {
	cfg      scan.Config
	interval time.Duration

	siteCode    string
	siteID      *uint
	siteChecked bool

	mu      sync.Mutex
	running bool
	lastRun time.Time

	disabledByCollector bool
}

var Default = &Sweeper{}

func Configure() {
	cidrs := splitList(os.Getenv("DISCOVERY_CIDRS"))
	if len(cidrs) == 0 {
		log.Println("[Discovery] DISCOVERY_CIDRS não definido: inventário de rede desligado")
		return
	}

	interval := defaultInterval
	if raw := os.Getenv("DISCOVERY_INTERVAL_MIN"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			interval = time.Duration(n) * time.Minute
		}
	}

	cfg := scan.Config{CIDRs: cidrs}
	if ports := splitList(os.Getenv("DISCOVERY_PORTS")); len(ports) > 0 {
		for _, p := range ports {
			if n, err := strconv.Atoi(p); err == nil && n > 0 && n < 65536 {
				cfg.Ports = append(cfg.Ports, n)
			}
		}
	}

	Default.cfg = cfg.WithDefaults()
	Default.interval = interval
	Default.siteCode = strings.ToLower(strings.TrimSpace(os.Getenv("DISCOVERY_SITE")))

	if Default.siteCode == "" {
		log.Printf("[Discovery] inventário ativo: %s a cada %s (sem unidade: defina DISCOVERY_SITE para classificar os hosts)",
			strings.Join(cidrs, ", "), interval)
		return
	}
	log.Printf("[Discovery] inventário ativo: %s a cada %s, unidade %q",
		strings.Join(cidrs, ", "), interval, Default.siteCode)
}

func (s *Sweeper) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.cfg.CIDRs) > 0 && !s.disabledByCollector
}

func (s *Sweeper) Start(ctx context.Context) {
	if !s.Enabled() {
		return
	}
	safego.Run(ctx, "discovery:varredura", func(ctx context.Context) {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			s.Run(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
}

func (s *Sweeper) Run(ctx context.Context) int {
	if !s.claim() {
		log.Println("[Discovery] varredura anterior ainda rodando; ciclo descartado")
		return -1
	}
	defer s.release()

	started := time.Now()
	hosts, errs := scan.Run(ctx, s.cfg)
	for _, err := range errs {
		log.Printf("[Discovery] %v", err)
	}
	if ctx.Err() != nil {
		return len(hosts)
	}

	persist(hosts, s.resolveSite())
	prune()

	log.Printf("[Discovery] varredura concluída: %d hosts em %s", len(hosts), time.Since(started).Round(time.Millisecond))
	return len(hosts)
}

func (s *Sweeper) resolveSite() *uint {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.siteChecked || s.siteCode == "" {
		return s.siteID
	}
	s.siteChecked = true

	var site database.Site
	if err := database.DB.Where("code = ?", s.siteCode).First(&site).Error; err != nil {
		log.Printf("[Discovery] unidade %q não cadastrada; hosts ficarão sem classificação", s.siteCode)
		return nil
	}
	s.siteID = &site.ID

	var coletores int64
	if err := database.DB.Model(&database.DeviceCredential{}).
		Where("site_id = ? AND kind = ? AND revoked_at IS NULL", site.ID, "collector").
		Count(&coletores).Error; err == nil && coletores > 0 {
		log.Printf("[Discovery] a unidade %q já tem %d coletor(es) registrado(s): "+
			"varredura local DESLIGADA para os dois não disputarem o inventário. "+
			"Remova DISCOVERY_CIDRS, ou revogue o coletor se quiser varrer daqui.",
			s.siteCode, coletores)
		s.disabledByCollector = true
	}

	return s.siteID
}

func (s *Sweeper) claim() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	s.lastRun = time.Now()
	return true
}

func (s *Sweeper) release() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

func (s *Sweeper) LastRun() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastRun
}

func persist(hosts []scan.Host, siteID *uint) {
	if len(hosts) == 0 {
		return
	}
	now := time.Now().UTC()

	records := make([]database.NetworkHost, 0, len(hosts))
	for _, h := range hosts {
		records = append(records, database.NetworkHost{
			IP:         h.IP,
			Hostname:   h.Hostname,
			MAC:        h.MAC,
			OpenPorts:  joinPorts(h.OpenPorts),
			DeviceType: DeviceType(h.OpenPorts),
			SiteID:     siteID,
			FirstSeen:  now,
			LastSeen:   now,
		})
	}

	if siteID != nil {
		ips := make([]string, 0, len(records))
		for _, r := range records {
			ips = append(ips, r.IP)
		}
		if err := database.AdoptNetworkHostsWithoutSite(*siteID, ips); err != nil {
			log.Printf("[Discovery] erro ao adotar hosts sem unidade: %v", err)
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
		log.Printf("[Discovery] erro ao gravar o inventário: %v", err)
	}
}

var hostRetention = database.RetentionDays("HOST_RETENTION_DAYS", database.DefaultHostRetentionDays)

func prune() {
	cutoff := time.Now().UTC().Add(-hostRetention)
	res := database.DB.Where("last_seen < ?", cutoff).Delete(&database.NetworkHost{})
	if res.Error != nil {
		log.Printf("[Discovery] erro ao podar hosts antigos: %v", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("[Discovery] %d hosts sem contato há %s removidos", res.RowsAffected, hostRetention)
	}
}

func joinPorts(ports []int) string {
	parts := make([]string, len(ports))
	for i, p := range ports {
		parts[i] = strconv.Itoa(p)
	}
	return strings.Join(parts, ",")
}

func splitList(raw string) []string {
	var out []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
