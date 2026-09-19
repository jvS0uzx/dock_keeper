package database

import "time"

type AuditLog struct {
	ID uint      `gorm:"primaryKey" json:"id"`
	At time.Time `gorm:"index;not null" json:"at"`

	ActorUserID   *uint  `gorm:"index" json:"actor_user_id"`
	ActorUsername string `gorm:"size:64;index" json:"actor_username"`
	ActorRole     string `gorm:"size:16" json:"actor_role"`

	SourceIP  string `gorm:"size:45;index" json:"source_ip"`
	UserAgent string `gorm:"size:255" json:"user_agent"`

	Action string `gorm:"size:64;index;not null" json:"action"`

	TargetType  string `gorm:"size:32;index" json:"target_type"`
	TargetID    string `gorm:"size:64;index" json:"target_id"`
	TargetLabel string `gorm:"size:255" json:"target_label"`

	SiteID *uint `gorm:"index" json:"site_id"`

	Result string `gorm:"size:16;index;not null" json:"result"`

	Detail string `gorm:"type:jsonb" json:"detail"`
}
