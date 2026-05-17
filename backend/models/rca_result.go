package models

import "time"

// RemediationStatus tracks the execution state of a fix command.
type RemediationStatus string

const (
	RemediationPending  RemediationStatus = "pending"
	RemediationExecuted RemediationStatus = "executed"
	RemediationSkipped  RemediationStatus = "skipped"
	RemediationFailed   RemediationStatus = "failed"
)

// FixCommand is a single kubectl / shell command proposed by the RCA engine.
type FixCommand struct {
	Command    string            `json:"command"`
	IsAutoSafe bool              `json:"isAutoSafe"`
	Status     RemediationStatus `json:"status"`
	Output     string            `json:"output,omitempty"`
	ExecError  string            `json:"error,omitempty"`
}

// RCAResult stores the AI analysis output for a given alert.
type RCAResult struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	AlertID     uint      `gorm:"index;not null"           json:"alertID"`
	Explanation string    `gorm:"type:text"                json:"explanation"`
	RootCause   string    `gorm:"type:text"                json:"rootCause"`
	Commands    string    `gorm:"type:text"                json:"commands"` // JSON []FixCommand
	Status      string    `json:"status"`                                  // pending | done | failed
	Model       string    `json:"model"`
	InputTokens int       `json:"inputTokens"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
