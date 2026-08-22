package database

import (
	"gorm.io/gorm"
	"time"
)

type Server struct {
	ID        string         `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name      string         `gorm:"size:255;not null" json:"name"`
	HostIP    string         `gorm:"size:255;not null" json:"host_ip"`
	User      string         `gorm:"size:100;default:'root'" json:"user"`
	Port      int            `gorm:"default:22" json:"port"`
	Kind      string         `gorm:"size:20;default:'ssh'" json:"kind"` // "ssh" (coleta via SSH) ou "agent" (push)
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// AlertRule é uma regra configurável de alerta sobre métricas de host.
type AlertRule struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Name      string     `gorm:"size:255;not null" json:"name"`
	Target    string     `gorm:"size:64;not null;default:'*'" json:"target"` // server_id ou "*" (todos)
	Metric    string     `gorm:"size:32;not null" json:"metric"`             // cpu | mem | disk | load
	Operator  string     `gorm:"size:4;not null" json:"operator"`            // ">" ou "<"
	Threshold float64    `gorm:"not null" json:"threshold"`
	Enabled   bool       `gorm:"default:true" json:"enabled"`
	LastFired *time.Time `json:"last_fired"`
	CreatedAt time.Time  `json:"created_at"`
}

// LogEntry armazena linhas de log (auth.log / container) para histórico e busca.
type LogEntry struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ServerID  string    `gorm:"type:uuid;index:idx_logentry_srv_ts,priority:1" json:"server_id"`
	Source    string    `gorm:"size:20;index" json:"source"` // "auth" | "container"
	Container string    `gorm:"size:255;index" json:"container"`
	Line      string    `gorm:"type:text" json:"line"`
	Timestamp time.Time `gorm:"index:idx_logentry_srv_ts,priority:2,sort:desc" json:"timestamp"`
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
	PingLatencyMs   float64
	CPUUsagePercent float64
	MemUsedBytes    int64
	MemTotalBytes   int64
	LoadAvg1        float64
	Timestamp       time.Time `gorm:"not null;index:idx_metricserver_srv_ts,priority:2,sort:desc"`
}

type MetricContainer struct {
	ID              uint      `gorm:"primaryKey"`
	ContainerID     string    `gorm:"type:uuid;not null;index:idx_metriccontainer_ct_ts,priority:1"`
	CPUUsagePercent float64   `gorm:"not null"`
	MemUsedBytes    int64     `gorm:"not null"`
	MemLimitBytes   int64     `gorm:"not null"`
	State           string    `gorm:"size:50;not null;default:'running'"`
	Status          string    `gorm:"size:255;not null;default:''"`
	Timestamp       time.Time `gorm:"not null;index:idx_metriccontainer_ct_ts,priority:2,sort:desc"`
}

type MetricLoadBalancer struct {
	ID            uint      `gorm:"primaryKey"`
	UpstreamAddr  string    `gorm:"size:255;not null"`
	ServerName    string    `gorm:"size:255"`
	Status        string    `gorm:"size:10"`
	RequestsCount int       `gorm:"not null"`
	Timestamp     time.Time `gorm:"index;not null"`
}

type Domain struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:255;not null;unique" json:"domain"`
	ServerID  string    `gorm:"type:uuid;index" json:"server_id"`
	CreatedAt time.Time `json:"created_at"`

	// Estado do certificado, preenchido pelo worker de SSL (não pelo usuário).
	Valid     bool       `gorm:"default:false" json:"valid"`
	Issuer    string     `gorm:"size:255" json:"issuer"`
	DaysLeft  int        `gorm:"default:0" json:"days_left"`
	ErrorMsg  string     `gorm:"size:500" json:"error_msg"`
	LastCheck *time.Time `json:"last_check"`
}
