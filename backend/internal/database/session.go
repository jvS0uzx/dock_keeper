package database

import "time"

type UserSession struct {
	TokenHash string `gorm:"size:64;primaryKey" json:"-"`

	UserID uint   `gorm:"index;not null" json:"user_id"`
	Role   string `gorm:"size:16;not null" json:"role"`

	Username string `gorm:"size:64;index" json:"username"`

	ExpiresAt  time.Time `gorm:"index;not null" json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}
