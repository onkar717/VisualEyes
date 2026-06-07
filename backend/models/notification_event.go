package models

import "time"

// NotificationEvent records a single delivery attempt for an alert notification.
type NotificationEvent struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	AlertID   uint      `gorm:"index;not null"           json:"alertID"`
	RuleName  string    `gorm:"not null"                 json:"ruleName"`
	Severity  string    `gorm:"not null"                 json:"severity"`
	EventType string    `gorm:"not null"                 json:"eventType"` // "fired" | "resolved"
	Channel   string    `gorm:"not null"                 json:"channel"`   // "slack", "noop", …
	Success   bool      `gorm:"not null"                 json:"success"`
	ErrMsg    string    `gorm:"column:error;default:''"  json:"error,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime"           json:"createdAt"`
}
