package database

import (
	"time"
	"gorm.io/gorm"
)

type Server struct {
	ID        string         `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name      string         `gorm:"size:255;not null" json:"name"`
	HostIP    string         `gorm:"size:255;not null" json:"host_ip"`
	User      string         `gorm:"size:100;default:'root'" json:"user"`
	Port      int            `gorm:"default:22" json:"port"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type Container struct {
	ID         string         `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	ServerID   string         `gorm:"type:uuid;index;not null"`
	DockerID   string         `gorm:"size:64;not null;index"`
	Name       string         `gorm:"size:255;not null"`
	ProjectDir string         `gorm:"size:500"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type MetricServer struct {
	ID              uint      `gorm:"primaryKey"`
	ServerID        string    `gorm:"type:uuid;index;not null"`
	UptimeSeconds   float64
	DiskUsedBytes   int64
	DiskTotalBytes  int64
	PingLatencyMs   float64
	CPUUsagePercent float64
	MemUsedBytes    int64
	MemTotalBytes   int64
	Timestamp       time.Time `gorm:"index;not null"`
}

type MetricContainer struct {
	ID              uint      `gorm:"primaryKey"`
	ContainerID     string    `gorm:"type:uuid;index;not null"`
	CPUUsagePercent float64   `gorm:"not null"`
	MemUsedBytes    int64     `gorm:"not null"`
	MemLimitBytes   int64     `gorm:"not null"`
	Status          string    `gorm:"size:50;not null"`
	Timestamp       time.Time `gorm:"index;not null"`
}

type MetricLoadBalancer struct {
	ID             uint      `gorm:"primaryKey"`
	UpstreamAddr   string    `gorm:"size:255;not null"`
	ServerName     string    `gorm:"size:255"`
	Status         string    `gorm:"size:10"`
	RequestsCount  int       `gorm:"not null"`
	Timestamp      time.Time `gorm:"index;not null"`
}
