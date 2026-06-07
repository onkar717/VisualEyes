package models

import (
	"fmt"
	"math/rand"
	"time"
)

// IncidentSeverity follows Google/PagerDuty SRE conventions.
type IncidentSeverity string

const (
	IncidentSEV1 IncidentSeverity = "SEV1" // Critical — service down, data loss risk
	IncidentSEV2 IncidentSeverity = "SEV2" // High — major degradation
	IncidentSEV3 IncidentSeverity = "SEV3" // Medium — partial degradation
	IncidentSEV4 IncidentSeverity = "SEV4" // Low — no user impact / healthy
)

// IncidentStatus tracks the lifecycle of an incident.
type IncidentStatus string

const (
	IncidentOpen          IncidentStatus = "OPEN"
	IncidentInvestigating IncidentStatus = "INVESTIGATING"
	IncidentMitigated     IncidentStatus = "MITIGATED"
	IncidentResolved      IncidentStatus = "RESOLVED"
)

// Incident is the top-level response record created when an alert fires and
// RCA completes. It carries the full AI diagnosis, severity classification,
// and lifecycle timeline — enabling MTTR tracking and incident reporting.
type Incident struct {
	ID           uint             `gorm:"primaryKey;autoIncrement"    json:"id"`
	IncidentCode string           `gorm:"uniqueIndex;not null"        json:"incidentCode"` // INC-XXXXXXXX
	AlertID      uint             `gorm:"index;not null"              json:"alertID"`
	RCAID        *uint            `gorm:"index"                       json:"rcaID,omitempty"`

	Title    string           `gorm:"not null"      json:"title"`
	Severity IncidentSeverity `gorm:"not null;index" json:"severity"`
	Category string           `gorm:"not null"      json:"category"`
	Status   IncidentStatus   `gorm:"not null;index" json:"status"`

	RootCause           string `gorm:"type:text" json:"rootCause"`
	ContributingFactors string `gorm:"type:text" json:"contributingFactors"` // JSON []string
	AffectedServices    string `gorm:"type:text" json:"affectedServices"`    // JSON []string
	ConfidenceScore     int    `json:"confidenceScore"`

	// Timing
	DetectedAt  time.Time  `gorm:"index;not null" json:"detectedAt"`
	MitigatedAt *time.Time `json:"mitigatedAt,omitempty"`
	ResolvedAt  *time.Time `json:"resolvedAt,omitempty"`
	MTTRSeconds *int       `json:"mttrSeconds,omitempty"` // seconds from DetectedAt → Mitigated/Resolved

	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

// ComputeMTTR sets MTTRSeconds from DetectedAt to whichever end time is set.
func (inc *Incident) ComputeMTTR() {
	var end *time.Time
	if inc.ResolvedAt != nil {
		end = inc.ResolvedAt
	} else if inc.MitigatedAt != nil {
		end = inc.MitigatedAt
	}
	if end == nil {
		return
	}
	secs := int(end.Sub(inc.DetectedAt).Seconds())
	if secs < 0 {
		secs = 0
	}
	inc.MTTRSeconds = &secs
}

// NewIncidentCode generates a unique INC-XXXXXXXX code.
func NewIncidentCode() string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return fmt.Sprintf("INC-%s", string(b))
}

// SeverityFromRCA maps an RCA severity string (SEV1-4) to IncidentSeverity.
func SeverityFromRCA(s string) IncidentSeverity {
	switch s {
	case "SEV1":
		return IncidentSEV1
	case "SEV2":
		return IncidentSEV2
	case "SEV3":
		return IncidentSEV3
	default:
		return IncidentSEV4
	}
}
