package database

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Server struct {
	ID     string `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name   string `gorm:"size:255;not null;index:idx_server_site_name,priority:2" json:"name"`
	HostIP string `gorm:"size:255;not null" json:"host_ip"`
	User   string `gorm:"size:100;default:'root'" json:"user"`
	Port   int    `gorm:"default:22" json:"port"`
	Kind   string `gorm:"size:20;default:'ssh'" json:"kind"`

	CollectNginx bool `gorm:"default:false" json:"collect_nginx"`

	NginxEstado     string     `gorm:"size:16;not null;default:'desconhecido';index" json:"nginx_estado"`
	NginxMotivo     string     `gorm:"type:text;not null;default:''" json:"nginx_motivo"`
	NginxChecadoEm  *time.Time `json:"nginx_checado_em"`
	NginxPapel      string     `gorm:"size:16;not null;default:'nenhum';index" json:"nginx_papel"`
	NginxPapelDesde *time.Time `json:"nginx_papel_desde"`

	SiteID *uint `gorm:"index;index:idx_server_site_name,priority:1" json:"site_id"`

	OS           string `gorm:"size:64" json:"os"`
	Platform     string `gorm:"size:128" json:"platform"`
	Arch         string `gorm:"size:32" json:"arch"`
	AgentVersion string `gorm:"size:32" json:"agent_version"`
	LastUser     string `gorm:"size:128" json:"last_user"`

	ReportIntervalSec int `gorm:"default:0" json:"report_interval_sec"`

	MachineID string `gorm:"size:128;index" json:"machine_id"`

	BehindLB *bool `json:"behind_lb"`

	AbsenceAlert bool `gorm:"not null" json:"absence_alert"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type AlertRule struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Name      string     `gorm:"size:255;not null" json:"name"`
	Target    string     `gorm:"size:64;not null;default:'*'" json:"target"`
	Metric    string     `gorm:"size:32;not null" json:"metric"`
	Operator  string     `gorm:"size:4;not null" json:"operator"`
	Threshold float64    `gorm:"not null" json:"threshold"`
	Enabled   bool       `json:"enabled"`
	LastFired *time.Time `json:"last_fired"`

	Severity string `gorm:"size:16;not null;default:'warning'" json:"severity"`

	DependsOnServerID *string `gorm:"type:uuid;index" json:"depends_on_server_id"`

	TargetSiteID *uint `gorm:"index" json:"target_site_id"`

	ForDurationSec int `gorm:"default:0" json:"for_duration_sec"`

	CreatedAt time.Time `json:"created_at"`
}

type LogEntry struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ServerID  string    `gorm:"type:uuid;index:idx_logentry_srv_ts,priority:1" json:"server_id"`
	Source    string    `gorm:"size:20;index" json:"source"`
	Container string    `gorm:"size:255;index" json:"container"`
	Line      string    `gorm:"type:text" json:"line"`
	Timestamp time.Time `gorm:"index:idx_logentry_srv_ts,priority:2,sort:desc" json:"timestamp"`
}

type User struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Username     string     `gorm:"size:64;uniqueIndex;not null" json:"username"`
	Nome         string     `gorm:"size:120;not null;default:''" json:"nome"`
	Email        string     `gorm:"size:160;not null;default:''" json:"email"`
	PasswordHash string     `gorm:"size:255;not null" json:"-"`
	Role         string     `gorm:"size:16;not null;default:'viewer'" json:"role"`
	Active       bool       `json:"active"`
	LastLogin    *time.Time `json:"last_login"`
	CreatedAt    time.Time  `json:"created_at"`
}

type UserSiteAccess struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	UserID uint   `gorm:"index;not null" json:"user_id"`
	SiteID *uint  `gorm:"index" json:"site_id"`
	Role   string `gorm:"size:16;not null" json:"role"`
}

type Site struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:255;not null" json:"name"`
	Code      string    `gorm:"size:64;uniqueIndex" json:"code"`
	Address   string    `gorm:"size:500" json:"address"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	CreatedAt time.Time `json:"created_at"`
}

type NetworkHost struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	IP        string    `gorm:"size:45;index" json:"ip"`
	Hostname  string    `gorm:"size:255" json:"hostname"`
	MAC       string    `gorm:"size:32;index" json:"mac"`
	OpenPorts string    `gorm:"type:text" json:"open_ports"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `gorm:"index" json:"last_seen"`

	DeviceType string `gorm:"size:64;index" json:"device_type"`

	DeviceTypeLocked bool `gorm:"default:false" json:"device_type_locked"`

	SiteID *uint `gorm:"index" json:"site_id"`

	SiteLocked bool `gorm:"default:false" json:"site_locked"`

	Floor    string `gorm:"size:64" json:"floor"`
	Sector   string `gorm:"size:128;index" json:"sector"`
	Room     string `gorm:"size:64" json:"room"`
	Rack     string `gorm:"size:64" json:"rack"`
	AssetTag string `gorm:"size:64;index" json:"asset_tag"`
	Owner    string `gorm:"size:255" json:"owner"`
	Notes    string `gorm:"type:text" json:"notes"`
}

func networkHostSiteExpr(prefix string) string {
	return "COALESCE(" + prefix + "site_id, 0)"
}

func NetworkHostConflictTarget() []clause.Column {
	return []clause.Column{
		{Name: networkHostSiteExpr(""), Raw: true},
		{Name: "ip"},
	}
}

func AdoptNetworkHostsWithoutSite(siteID uint, ips []string) error {
	if siteID == 0 || len(ips) == 0 {
		return nil
	}
	return DB.Exec(`
		UPDATE network_hosts AS h
		SET site_id = ?
		WHERE h.site_id IS NULL
		  AND h.site_locked = false
		  AND h.ip IN ?
		  AND NOT EXISTS (
			SELECT 1 FROM network_hosts AS o
			WHERE o.ip = h.ip AND o.site_id = ?
		  )`, siteID, ips, siteID).Error
}

type FloorPlan struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	SiteID      *uint     `gorm:"index" json:"site_id"`
	Name        string    `gorm:"size:255;not null" json:"name"`
	ImagePath   string    `gorm:"size:500" json:"-"`
	ContentType string    `gorm:"size:64" json:"content_type"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type FloorPlanPin struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	PlanID uint `gorm:"index;not null" json:"plan_id"`

	HostIP string `gorm:"size:45;index" json:"host_ip"`
	Label  string `gorm:"size:255" json:"label"`

	X float64 `json:"x"`
	Y float64 `json:"y"`

	TargetPlanID *uint `json:"target_plan_id"`
}

type Container struct {
	ID         string `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	ServerID   string `gorm:"type:uuid;index;not null"`
	DockerID   string `gorm:"size:64;not null;index"`
	Name       string `gorm:"size:255;not null"`
	ProjectDir string `gorm:"size:500"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type MetricServer struct {
	ID              uint   `gorm:"primaryKey"`
	ServerID        string `gorm:"type:uuid;not null;index:idx_metricserver_srv_ts,priority:1"`
	UptimeSeconds   float64
	DiskUsedBytes   int64
	DiskTotalBytes  int64
	CPUUsagePercent *float64
	MemUsedBytes    int64
	MemTotalBytes   int64
	LoadAvg1        *float64

	SSHHandshakeMs *float64 `gorm:"column:ping_latency_ms"`

	TemperatureC *float64

	NetRxBps *float64 `json:"net_rx_bps"`
	NetTxBps *float64 `json:"net_tx_bps"`

	RTTMs *float64 `gorm:"column:rtt_ms" json:"rtt_ms"`

	Timestamp time.Time `gorm:"not null;index:idx_metricserver_srv_ts,priority:2,sort:desc"`
}

type MetricServerTrend struct {
	ID       uint      `gorm:"primaryKey"`
	ServerID string    `gorm:"type:uuid;not null;uniqueIndex:idx_trend_srv_bucket,priority:1"`
	Bucket   time.Time `gorm:"not null;uniqueIndex:idx_trend_srv_bucket,priority:2"`

	CPUAvg, CPUMax           *float64
	LoadAvg1Avg, LoadAvg1Max *float64
	Samples                  int

	MemPercentAvg  *float64
	DiskPercentAvg *float64
	TemperatureAvg *float64
	TemperatureMax *float64

	NetRxAvg *float64
	NetTxAvg *float64

	RTTAvg *float64 `gorm:"column:rtt_avg"`
	RTTMax *float64 `gorm:"column:rtt_max"`
}

const (
	AlertStatusOpen     = "open"
	AlertStatusAcked    = "acked"
	AlertStatusResolved = "resolved"

	AlertDeliveryPendente = "pendente"
	AlertDeliveryEnviado  = "enviado"
	AlertDeliveryFalhou   = "falhou"
	AlertDeliverySemCanal = "sem_canal"

	AlertDeliveryDispensado = "dispensado"

	AlvoTipoHost      = "host"
	AlvoTipoContainer = "container"
	AlvoTipoServico   = "servico"
	AlvoTipoInterface = "interface"

	NginxDesconhecido = "desconhecido"
	NginxAusente      = "ausente"
	NginxInativo      = "inativo"
	NginxSemUpstream  = "sem_upstream"
	NginxCandidato    = "candidato"

	NginxPapelNenhum    = "nenhum"
	NginxPapelPrincipal = "principal"
	NginxPapelReserva   = "reserva"
)

type Alert struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Key      string `gorm:"size:128;not null;index" json:"key"`
	Severity string `gorm:"size:16;not null" json:"severity"`
	Text     string `gorm:"type:text;not null" json:"text"`
	Status   string `gorm:"size:16;not null;index" json:"status"`

	ServerID *string `gorm:"type:uuid;index" json:"server_id"`
	SiteID   *uint   `gorm:"index" json:"site_id"`
	RuleID   *uint   `gorm:"index" json:"rule_id"`

	CreatedAt  time.Time  `gorm:"index" json:"created_at"`
	AckedAt    *time.Time `json:"acked_at"`
	AckedBy    *uint      `json:"acked_by"`
	ResolvedAt *time.Time `json:"resolved_at"`

	Delivery      string     `gorm:"size:16;not null;index" json:"delivery"`
	Attempts      int        `gorm:"not null" json:"attempts"`
	NextAttemptAt *time.Time `gorm:"index" json:"next_attempt_at"`
	LastAttemptAt *time.Time `json:"last_attempt_at"`
	LastError     string     `gorm:"type:text" json:"last_error"`

	RenotifyCount  int        `gorm:"not null;default:0" json:"renotify_count"`
	LastNotifiedAt *time.Time `json:"last_notified_at"`
	LastSeenAt     *time.Time `json:"last_seen_at"`

	AlvoTipo *string  `gorm:"size:16;index" json:"alvo_tipo"`
	AlvoID   *string  `gorm:"size:128" json:"alvo_id"`
	AlvoNome *string  `gorm:"size:255" json:"alvo_nome"`
	Metrica  *string  `gorm:"size:64" json:"metrica"`
	Valor    *float64 `json:"valor"`
	Limiar   *float64 `json:"limiar"`
	Unidade  *string  `gorm:"size:16" json:"unidade"`
}

type NginxUpstream struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	ServerID string `gorm:"type:uuid;not null;uniqueIndex:idx_upstream_dono_bloco_destino,priority:1;index" json:"server_id"`
	Bloco    string `gorm:"size:128;not null;uniqueIndex:idx_upstream_dono_bloco_destino,priority:2" json:"bloco"`
	Destino  string `gorm:"size:255;not null;uniqueIndex:idx_upstream_dono_bloco_destino,priority:3" json:"destino"`

	ObservadoEm time.Time `gorm:"not null;index" json:"observado_em"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AlertDelivery struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	AlertID uint   `gorm:"not null;uniqueIndex:idx_entrega_alerta_canal,priority:1" json:"alert_id"`
	Canal   string `gorm:"size:32;not null;uniqueIndex:idx_entrega_alerta_canal,priority:2" json:"canal"`

	Status        string     `gorm:"size:16;not null;index" json:"status"`
	Attempts      int        `gorm:"not null;default:0" json:"attempts"`
	NextAttemptAt *time.Time `gorm:"index" json:"next_attempt_at"`
	LastAttemptAt *time.Time `json:"last_attempt_at"`
	LastError     string     `gorm:"type:text" json:"last_error"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MetricContainer struct {
	ID              uint    `gorm:"primaryKey"`
	ContainerID     string  `gorm:"type:uuid;not null;index:idx_metriccontainer_ct_ts,priority:1"`
	CPUUsagePercent float64 `gorm:"not null"`
	MemUsedBytes    int64   `gorm:"not null"`
	MemLimitBytes   int64   `gorm:"not null"`
	State           string  `gorm:"size:50;not null;default:'running'"`
	Status          string  `gorm:"size:255;not null;default:''"`
	Health          *string `gorm:"size:32"`
	RestartCount    *int
	OOMKilled       *bool
	Timestamp       time.Time `gorm:"not null;index:idx_metriccontainer_ct_ts,priority:2,sort:desc"`
}

type MetricLoadBalancer struct {
	ID           uint   `gorm:"primaryKey"`
	UpstreamAddr string `gorm:"size:255;not null"`
	ServerName   string `gorm:"size:255"`
	Status       string `gorm:"size:10"`

	ServerID *string `gorm:"type:uuid;index" json:"server_id"`
	SiteID   *uint   `gorm:"index" json:"site_id"`

	RequestsCount int       `gorm:"not null"`
	Timestamp     time.Time `gorm:"index;not null"`
}

type Domain struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:255;not null;unique" json:"domain"`
	ServerID  *string   `gorm:"type:uuid;index" json:"server_id"`
	CreatedAt time.Time `json:"created_at"`

	Valid    bool   `gorm:"default:false" json:"valid"`
	Issuer   string `gorm:"size:255" json:"issuer"`
	DaysLeft int    `gorm:"default:0" json:"days_left"`
	ErrorMsg string `gorm:"size:500" json:"error_msg"`

	InvalidReason string `gorm:"size:40;index" json:"invalid_reason"`

	LastCheck *time.Time `json:"last_check"`
}

type Dashboard struct {
	ID          uint             `gorm:"primaryKey" json:"id"`
	OwnerUserID uint             `gorm:"not null;uniqueIndex:idx_dashboard_owner_name,priority:1" json:"-"`
	Name        string           `gorm:"size:80;not null;uniqueIndex:idx_dashboard_owner_name,priority:2" json:"name"`
	Panels      []DashboardPanel `gorm:"constraint:OnDelete:CASCADE" json:"panels"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

type DashboardPanel struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	DashboardID uint   `gorm:"not null;index" json:"-"`
	Position    int    `gorm:"not null" json:"position"`
	Title       string `gorm:"size:80" json:"title"`
	ServerID    string `gorm:"type:uuid;not null" json:"server_id"`
	Metric      string `gorm:"size:16;not null" json:"metric"`
	Range       string `gorm:"size:8;not null" json:"range"`
	Width       int    `gorm:"not null" json:"width"`
}

type Annotation struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	ServerID     *string   `gorm:"type:uuid;index:idx_annotation_srv_at,priority:1" json:"server_id"`
	At           time.Time `gorm:"not null;index:idx_annotation_srv_at,priority:2" json:"at"`
	Text         string    `gorm:"size:280;not null" json:"text"`
	AuthorUserID uint      `gorm:"not null" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}
