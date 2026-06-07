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

// RCAResult stores the full AI analysis output for a given alert.
// The multi-stage pipeline populates all fields; single-stage fills the core set.
type RCAResult struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	AlertID     uint      `gorm:"index;not null"           json:"alertID"`
	Explanation string    `gorm:"type:text"                json:"explanation"`
	RootCause   string    `gorm:"type:text"                json:"rootCause"`
	Commands    string    `gorm:"type:text"                json:"commands"` // JSON []FixCommand

	// AI quality signals
	ConfidenceScore     int    `json:"confidenceScore"`
	Severity            string `json:"severity"`  // SEV1|SEV2|SEV3|SEV4
	Category            string `json:"category"`  // crashloop|oom|high_cpu|…
	ContributingFactors string `gorm:"type:text"  json:"contributingFactors"` // JSON []string
	AffectedServices    string `gorm:"type:text"  json:"affectedServices"`    // JSON []string

	Status      string    `json:"status"` // pending | done | failed
	Model       string    `json:"model"`
	InputTokens int       `json:"inputTokens"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
