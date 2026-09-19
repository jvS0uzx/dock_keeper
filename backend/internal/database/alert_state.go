package database

import "time"

type AlertState struct {
	Key string `gorm:"size:128;primaryKey" json:"key"`

	RuleID   uint   `gorm:"index;not null" json:"rule_id"`
	ServerID string `gorm:"size:64;index" json:"server_id"`
	Severity string `gorm:"size:16" json:"severity"`

	FirstBreachAt time.Time `json:"first_breach_at"`

	LastBreachAt   time.Time  `json:"last_breach_at"`
	LastNotifiedAt *time.Time `gorm:"index" json:"last_notified_at"`

	Active bool `gorm:"index;default:false" json:"active"`

	UpdatedAt time.Time `json:"updated_at"`
}
