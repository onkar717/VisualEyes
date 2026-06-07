package models

import "time"

// RemediationLogEntry records the execution of a single remediation step.
// One entry is created per step when the processor auto-executes safe commands
// or when veye apply posts results back via /api/remediation-log.
type RemediationLogEntry struct {
	ID         uint              `gorm:"primaryKey;autoIncrement" json:"id"`
	IncidentID uint              `gorm:"index"                    json:"incident_id"`
	AlertID    uint              `gorm:"index"                    json:"alert_id"`
	StepNumber int               `gorm:"not null"                 json:"step_number"`
	Command    string            `gorm:"not null"                 json:"command"`
	Status     RemediationStatus `gorm:"not null;index"           json:"status"`
	Output     string            `gorm:"type:text"                json:"output,omitempty"`
	ExecError  string            `gorm:"type:text"                json:"exec_error,omitempty"`
	DryRun     bool              `json:"dry_run"`
	ExecutedAt time.Time         `gorm:"index;not null"           json:"executed_at"`
	DurationMs int64             `json:"duration_ms"`
}
