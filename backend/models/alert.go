package models

import "time"

// AlertSeverity classifies how urgent an alert is.
type AlertSeverity string

const (
	SeverityCritical AlertSeverity = "critical"
	SeverityWarning  AlertSeverity = "warning"
	SeverityInfo     AlertSeverity = "info"
)

// AlertStatus tracks the lifecycle of an alert.
type AlertStatus string

const (
	AlertFiring   AlertStatus = "firing"
	AlertResolved AlertStatus = "resolved"
)

// Alert represents a single fired or resolved anomaly detected by the engine.
type Alert struct {
	ID         uint          `gorm:"primaryKey;autoIncrement" json:"id"`
	RuleName   string        `gorm:"index;not null"           json:"ruleName"`
	MetricName string        `gorm:"index"                    json:"metricName"` // metric that triggered the rule
	Severity   AlertSeverity `gorm:"not null"                 json:"severity"`
	Status     AlertStatus   `gorm:"index;not null"           json:"status"`
	ResourceID string        `gorm:"index"                    json:"resourceID"` // node/pod name
	Namespace  string        `gorm:"index"                    json:"namespace"`
	Value      float64       `json:"value"`
	Threshold  float64       `json:"threshold"`
	Message    string        `gorm:"type:text"                json:"message"`
	FiredAt    time.Time     `gorm:"index;not null"           json:"firedAt"`
	ResolvedAt *time.Time    `json:"resolvedAt,omitempty"`
	RCAStatus  string        `json:"rcaStatus"` // pending | running | done | failed | skipped
	RCAID      *uint         `json:"rcaID,omitempty"`
}
