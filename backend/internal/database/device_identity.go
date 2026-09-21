package database

import "time"

type EnrollmentToken struct {
	ID uint `gorm:"primaryKey" json:"id"`

	TokenHash string `gorm:"size:64;uniqueIndex;not null" json:"-"`

	SiteID uint   `gorm:"index;not null" json:"site_id"`
	Kind   string `gorm:"size:16;not null" json:"kind"`

	ExpiresAt time.Time  `gorm:"index;not null" json:"expires_at"`
	UsedAt    *time.Time `json:"used_at"`

	CreatedBy uint      `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type DeviceCredential struct {
	DeviceID string `gorm:"size:32;primaryKey" json:"device_id"`

	SecretHash string `gorm:"size:64;not null" json:"-"`

	SiteID uint   `gorm:"index;not null" json:"site_id"`
	Kind   string `gorm:"size:16;not null" json:"kind"`

	MachineID string `gorm:"size:128;index" json:"machine_id"`
	Hostname  string `gorm:"size:255" json:"hostname"`

	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at"`

	ReportIntervalSec int `gorm:"not null;default:0" json:"report_interval_sec"`

	RevokedAt *time.Time `gorm:"index" json:"revoked_at"`
}
