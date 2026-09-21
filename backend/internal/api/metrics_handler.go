package api

import (
	"log"
	"net/http"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/config"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/metricas"
)

type ContainerLiveStat struct {
	ServerID string  `json:"server_id"`
	DockerID string  `json:"docker_id"`
	Name     string  `json:"name"`
	Project  string  `json:"project"`
	State    string  `json:"state"`
	Status   string  `json:"status"`
	CPU      float64 `json:"cpu"`
	MemUsed  int64   `json:"mem_used"`
	MemLimit int64   `json:"mem_limit"`

	Health       *string `json:"health"`
	RestartCount *int    `json:"restart_count"`
	OOMKilled    *bool   `json:"oom_killed"`
}

type ServerLiveStat struct {
	ID             string   `json:"id"`
	HostIP         string   `json:"host_ip"`
	Name           string   `json:"name"`
	Uptime         float64  `json:"uptime"`
	DiskUsed       int64    `json:"disk_used"`
	DiskTotal      int64    `json:"disk_total"`
	CPU            *float64 `json:"cpu"`
	MemUsed        int64    `json:"mem_used"`
	MemTotal       int64    `json:"mem_total"`
	Load1          *float64 `json:"load1"`
	Online         bool     `json:"online"`
	SSHHandshakeMs *float64 `json:"ssh_handshake_ms"`

	Kind           string   `json:"kind"`
	SiteID         *uint    `json:"site_id"`
	OS             string   `json:"os"`
	Platform       string   `json:"platform"`
	Arch           string   `json:"arch"`
	LastUser       string   `json:"last_user"`
	AgentVersion   string   `json:"agent_version"`
	TemperatureC   *float64 `json:"temperature_c"`
	NetRxBps       *float64 `json:"net_rx_bps"`
	NetTxBps       *float64 `json:"net_tx_bps"`
	RTTMs          *float64 `json:"rtt_ms"`
	Addresses      []string `json:"addresses"`
	Aliases        []string `json:"aliases"`
	AbsenceAlert   bool     `json:"absence_alert"`
	BehindLB       bool     `json:"behind_lb"`
	BehindLBOrigem string   `json:"behind_lb_origem"`
	CollectNginx   bool     `json:"collect_nginx"`

	NginxEstado    string     `json:"nginx_estado"`
	NginxMotivo    string     `json:"nginx_motivo"`
	NginxPapel     string     `json:"nginx_papel"`
	NginxChecadoEm *time.Time `json:"nginx_checado_em"`

	LiveWindowSec int `json:"live_window_sec"`
}

type LbStat struct {
	UpstreamAddr  string `json:"upstream_addr"`
	ServerName    string `json:"server_name"`
	Status        string `json:"status"`
	RequestsCount int    `json:"requests_count"`
	ServerID      string `json:"server_id"`
}

type NginxTopologiaItem struct {
	ServerID    string    `json:"server_id"`
	Bloco       string    `json:"bloco"`
	Destino     string    `json:"destino"`
	ObservadoEm time.Time `json:"observado_em"`
}

type LiveResponse struct {
	Servers        []ServerLiveStat     `json:"servers"`
	Containers     []ContainerLiveStat  `json:"containers"`
	LoadBalancing  []LbStat             `json:"load_balancing"`
	NginxTopologia []NginxTopologiaItem `json:"nginx_topologia"`
	LBWindowSec    int                  `json:"lb_window_sec"`
}

type metricaDoCatalogo struct {
	Nome         string `json:"nome"`
	Rotulo       string `json:"rotulo"`
	Unidade      string `json:"unidade"`
	TemTendencia bool   `json:"tem_tendencia"`
	Escopo       string `json:"escopo"`
	EmRegra      bool   `json:"em_regra"`
}

func catalogoDeMetricasHandler(w http.ResponseWriter, _ *http.Request) {
	todas := metricas.Todas()
	out := make([]metricaDoCatalogo, 0, len(todas))
	for _, m := range todas {
		out = append(out, metricaDoCatalogo{
			Nome: m.Nome, Rotulo: m.Rotulo, Unidade: m.Unidade,
			TemTendencia: m.TemTendencia(), Escopo: m.Escopo, EmRegra: m.Avaliavel(),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

const containerLiveWindow = "30 seconds"

const lbWindowSec = 5

const membroPadraoDias = 7

func janelaDaMalha() time.Duration {
	pedida := config.Dias("LB_MEMBERSHIP_DAYS", membroPadraoDias)
	retencao := database.RetentionDays("METRIC_RETENTION_DAYS", database.DefaultMetricRetentionDays)
	if retencao > 0 && pedida > retencao {
		return retencao
	}
	return pedida
}

func AvisarJanelaDaMalha() {
	pedida := config.Dias("LB_MEMBERSHIP_DAYS", membroPadraoDias)
	if efetiva := janelaDaMalha(); efetiva < pedida {
		log.Printf("[API] LB_MEMBERSHIP_DAYS pede %s, mas METRIC_RETENTION_DAYS poda metric_load_balancers em %s; a malha usa %s",
			pedida, efetiva, efetiva)
	}
}

func membroDaMalha(s database.Server, enderecos []string, upstreams, declarados map[string]bool) (bool, string) {
	if s.BehindLB != nil {
		return *s.BehindLB, "manual"
	}
	for _, endereco := range enderecos {
		if upstreams[endereco] {
			return true, "trafego"
		}
	}
	for _, endereco := range enderecos {
		if declarados[endereco] {
			return true, "configuracao"
		}
	}
	return false, "nenhum"
}

func unirEnderecos(hostIP string, conhecidos []string) []string {
	fora := make([]string, 0, len(conhecidos)+1)
	vistos := map[string]bool{}
	if hostIP != "" {
		fora = append(fora, hostIP)
		vistos[hostIP] = true
	}
	for _, endereco := range conhecidos {
		if vistos[endereco] {
			continue
		}
		vistos[endereco] = true
		fora = append(fora, endereco)
	}
	return fora
}

func liveMetricsHandler(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	scope, status := resolveScope(sess, r)
	if status != 0 {
		writeError(w, status, "site_id inválido ou fora do seu alcance")
		return
	}

	var servers []database.Server
	if err := scope.apply(database.From(r.Context()).Order("name ASC")).Find(&servers).Error; err != nil {
		log.Printf("[API] erro ao listar servidores: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao ler servidores")
		return
	}

	serverMetrics, err := lastServerMetrics()
	if err != nil {
		log.Printf("[API] erro ao ler métricas de host: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao ler métricas")
		return
	}

	res := LiveResponse{
		Servers:        make([]ServerLiveStat, 0, len(servers)),
		Containers:     []ContainerLiveStat{},
		LoadBalancing:  []LbStat{},
		NginxTopologia: []NginxTopologiaItem{},
		LBWindowSec:    lbWindowSec,
	}

	ids := make([]string, 0, len(servers))
	for _, s := range servers {
		ids = append(ids, s.ID)
	}
	enderecos, err := database.EnderecosPorServidor(ids)
	if err != nil {
		log.Printf("[API] erro ao ler endereços dos servidores: %v", err)
		enderecos = map[string][]string{}
	}

	aliases, err := database.AliasesPorServidor(ids)
	if err != nil {
		log.Printf("[API] erro ao ler aliases dos servidores: %v", err)
		aliases = map[string][]string{}
	}

	upstreams, err := database.UpstreamsRecentes(janelaDaMalha())
	if err != nil {
		log.Printf("[API] erro ao ler upstreams recentes: %v", err)
		upstreams = map[string]bool{}
	}

	declarados, err := database.UpstreamsDeclarados()
	if err != nil {
		log.Printf("[API] erro ao ler upstreams declarados: %v", err)
		declarados = map[string]bool{}
	}

	for _, s := range servers {
		stat := ServerLiveStat{
			ID: s.ID, HostIP: s.HostIP, Name: s.Name,
			Kind: s.Kind, SiteID: s.SiteID, OS: s.OS, Platform: s.Platform,
			Arch: s.Arch, LastUser: s.LastUser, AgentVersion: s.AgentVersion,
			CollectNginx: s.CollectNginx, AbsenceAlert: s.AbsenceAlert,
			Aliases: aliases[s.ID],

			NginxEstado: s.NginxEstado, NginxMotivo: s.NginxMotivo,
			NginxPapel: s.NginxPapel, NginxChecadoEm: s.NginxChecadoEm,
		}
		if stat.Aliases == nil {
			stat.Aliases = []string{}
		}
		stat.Addresses = unirEnderecos(s.HostIP, enderecos[s.ID])
		stat.BehindLB, stat.BehindLBOrigem = membroDaMalha(s, stat.Addresses, upstreams, declarados)
		window := database.LiveWindowFor(s.ReportIntervalSec)
		stat.LiveWindowSec = int(window / time.Second)

		if m, ok := serverMetrics[s.ID]; ok && time.Since(m.Timestamp) <= window {
			stat.Uptime = m.UptimeSeconds
			stat.DiskUsed = m.DiskUsedBytes
			stat.DiskTotal = m.DiskTotalBytes
			stat.CPU = m.CPUUsagePercent
			stat.MemUsed = m.MemUsedBytes
			stat.MemTotal = m.MemTotalBytes
			stat.Load1 = m.LoadAvg1
			stat.SSHHandshakeMs = m.SSHHandshakeMs
			stat.TemperatureC = m.TemperatureC
			stat.NetRxBps = m.NetRxBps
			stat.NetTxBps = m.NetTxBps
			stat.RTTMs = m.RTTMs
			stat.Online = true
		}
		res.Servers = append(res.Servers, stat)
	}

	inScope := make(map[string]bool, len(res.Servers))
	for _, s := range res.Servers {
		inScope[s.ID] = true
	}

	var containers []database.Container
	if err := database.From(r.Context()).Order("name ASC").Find(&containers).Error; err != nil {
		log.Printf("[API] erro ao listar containers: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao ler containers")
		return
	}

	containerMetrics, err := lastContainerMetrics()
	if err != nil {
		log.Printf("[API] erro ao ler métricas de container: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao ler métricas")
		return
	}

	for _, c := range containers {
		if !inScope[c.ServerID] {
			continue
		}
		m, ok := containerMetrics[c.ID]
		if !ok {
			continue
		}
		res.Containers = append(res.Containers, ContainerLiveStat{
			ServerID: c.ServerID,
			DockerID: c.DockerID,
			Name:     c.Name,
			Project:  c.ProjectDir,
			State:    m.State,
			Status:   m.Status,
			CPU:      m.CPUUsagePercent,
			MemUsed:  m.MemUsedBytes,
			MemLimit: m.MemLimitBytes,

			Health:       m.Health,
			RestartCount: m.RestartCount,
			OOMKilled:    m.OOMKilled,
		})
	}

	lbTx := scope.apply(database.From(r.Context()).Model(&database.MetricLoadBalancer{})).
		Select("upstream_addr, server_name, status, SUM(requests_count) AS requests_count, COALESCE(server_id::text, '') AS server_id").
		Where("timestamp >= NOW() - make_interval(secs => ?)", lbWindowSec).
		Group("upstream_addr, server_name, status, COALESCE(server_id::text, '')")
	if err := lbTx.Scan(&res.LoadBalancing).Error; err != nil {
		log.Printf("[API] erro ao agregar load balancer: %v", err)
	}
	if res.LoadBalancing == nil {
		res.LoadBalancing = []LbStat{}
	}

	if topologia, err := topologiaDoNginx(ids); err != nil {
		log.Printf("[API] erro ao ler a topologia do nginx: %v", err)
	} else {
		res.NginxTopologia = topologia
	}

	writeJSON(w, http.StatusOK, res)
}

func topologiaDoNginx(ids []string) ([]NginxTopologiaItem, error) {
	fora := []NginxTopologiaItem{}
	if len(ids) == 0 {
		return fora, nil
	}
	err := database.DB.Model(&database.NginxUpstream{}).
		Select("server_id::text AS server_id, bloco, destino, observado_em").
		Where("server_id IN ?", ids).
		Order("server_id ASC, bloco ASC, destino ASC").
		Scan(&fora).Error
	if err != nil {
		return nil, err
	}
	if fora == nil {
		fora = []NginxTopologiaItem{}
	}
	return fora, nil
}

func lastServerMetrics() (map[string]database.MetricServer, error) {
	var rows []database.MetricServer
	err := database.DB.Raw(database.ConsultaUltimasMetricas).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	byServer := make(map[string]database.MetricServer, len(rows))
	for _, m := range rows {
		byServer[m.ServerID] = m
	}
	return byServer, nil
}

func lastContainerMetrics() (map[string]database.MetricContainer, error) {
	var rows []database.MetricContainer
	err := database.DB.Raw(`
		SELECT DISTINCT ON (container_id) *
		FROM metric_containers
		WHERE timestamp >= NOW() - INTERVAL '` + containerLiveWindow + `'
		ORDER BY container_id, timestamp DESC
	`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	byContainer := make(map[string]database.MetricContainer, len(rows))
	for _, m := range rows {
		byContainer[m.ContainerID] = m
	}
	return byContainer, nil
}
