package discovery

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joaov/vd_stats/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultInterval = 15 * time.Minute

	// Um host que sumiu do inventário há mais de 30 dias vira ruído: some.

)

// Sweeper roda a varredura periódica e persiste o inventário. Uma varredura
// por vez: se o intervalo for curto demais ou a rede lenta, a próxima é
// descartada em vez de empilhar goroutines sondando a mesma faixa.
type Sweeper struct {
	cfg      Config
	interval time.Duration

	// siteCode é o código da unidade a que esta varredura pertence. O id só é
	// resolvido na primeira execução: Configure() roda antes de o banco estar
	// conectado, então consultar a tabela ali bateria num ponteiro nulo.
	siteCode    string
	siteID      *uint
	siteChecked bool

	mu      sync.Mutex
	running bool
	lastRun time.Time

	// disabledByCollector desliga a varredura depois do boot, quando se descobre
	// que a unidade já tem coletor. A conferência não cabe em Configure: ela
	// depende do banco, que ainda não subiu naquele ponto.
	disabledByCollector bool
}

var Default = &Sweeper{}

// Configure lê as variáveis de ambiente da varredura.
// DISCOVERY_CIDRS vazio deixa o recurso desligado.
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

	cfg := Config{CIDRs: cidrs}
	if ports := splitList(os.Getenv("DISCOVERY_PORTS")); len(ports) > 0 {
		for _, p := range ports {
			if n, err := strconv.Atoi(p); err == nil && n > 0 && n < 65536 {
				cfg.Ports = append(cfg.Ports, n)
			}
		}
	}

	Default.cfg = cfg.withDefaults()
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

// Enabled diz se há faixa configurada.
func (s *Sweeper) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.cfg.CIDRs) > 0 && !s.disabledByCollector
}

// Start dispara a varredura periódica até o contexto ser cancelado.
func (s *Sweeper) Start(ctx context.Context) {
	if !s.Enabled() {
		return
	}
	go func() {
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
	}()
}

// Run executa uma varredura e persiste o resultado. Devolve quantos hosts
// responderam, ou -1 se outra varredura já estava em andamento.
func (s *Sweeper) Run(ctx context.Context) int {
	if !s.claim() {
		log.Println("[Discovery] varredura anterior ainda rodando; ciclo descartado")
		return -1
	}
	defer s.release()

	started := time.Now()
	hosts, errs := Scan(ctx, s.cfg)
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

// resolveSite converte DISCOVERY_SITE em id, uma única vez.
//
// Unidade configurada mas ausente do banco não derruba a varredura: o
// inventário continua sendo gravado sem classificação, que é melhor do que
// perder a coleta inteira por um código digitado errado.
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

	// Coletor registrado para a unidade e varredura local ligada é dois
	// escritores no mesmo inventário: cada ciclo sobrescreve o do outro, e a
	// diferença nas listas de porta faz o tipo do equipamento alternar sozinho.
	// O coletor vence porque enxerga a rede da filial; o painel só enxerga a
	// dele.
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

// LastRun informa quando começou a última varredura.
func (s *Sweeper) LastRun() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastRun
}

// persist grava o inventário num único INSERT ... ON CONFLICT.
//
// Não dá para usar FirstOrCreate aqui: o GORM monta o WHERE com todos os campos
// não-zero da struct passada como condição, e first_seen/last_seen são o
// instante atual — nunca casam com a linha existente, então ele tenta inserir e
// esbarra no índice único de ip.
//
// O upsert resolve em uma consulta só para a varredura inteira:
//   - first_seen fica de fora do DoUpdates, preservando a primeira aparição;
//   - hostname e mac usam COALESCE/NULLIF para que um DNS reverso que falhou
//     (ou um ARP incompleto) não apague o valor já conhecido.
func persist(hosts []Host, siteID *uint) {
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

	// Adota antes de gravar: sem isso a linha que a varredura anterior deixou
	// sem unidade não colide com a nova, e o mesmo endereço vira duas linhas.
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
		// Precisa casar com o índice único do inventário, que é por unidade e
		// não mais global: alvo que não corresponde a índice nenhum faz o
		// Postgres recusar o INSERT inteiro.
		Columns: database.NetworkHostConflictTarget(),
		DoUpdates: clause.Assignments(map[string]any{
			"last_seen":  now,
			"open_ports": gorm.Expr("EXCLUDED.open_ports"),
			// O tipo é reinferido a cada varredura porque as portas mudam — mas
			// não quando o operador já corrigiu à mão, senão a correção dele
			// duraria só até o próximo ciclo.
			"device_type": gorm.Expr("CASE WHEN network_hosts.device_type_locked THEN network_hosts.device_type ELSE EXCLUDED.device_type END"),
			// Mesma regra para a unidade; os demais campos cadastrais (sala,
			// dono, patrimônio) nunca são tocados aqui.
			"site_id":  gorm.Expr("CASE WHEN network_hosts.site_locked THEN network_hosts.site_id ELSE COALESCE(EXCLUDED.site_id, network_hosts.site_id) END"),
			"hostname": gorm.Expr("COALESCE(NULLIF(EXCLUDED.hostname, ''), network_hosts.hostname)"),
			"mac":      gorm.Expr("COALESCE(NULLIF(EXCLUDED.mac, ''), network_hosts.mac)"),
		}),
	}).Create(&records).Error
	if err != nil {
		log.Printf("[Discovery] erro ao gravar o inventário: %v", err)
	}
}

// hostRetention é quanto tempo um host fica no inventário sem ser visto.
// Curto demais apaga notebook de quem tirou férias; longo demais deixa a tela
// cheia de máquina que não existe mais.
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
