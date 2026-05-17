package models

import (
	"encoding/json"
	"fmt"
	"time"
)

// MetricRecord is the GORM-persisted form of a Metric.
// Tags use PostgreSQL JSONB for efficient containment queries (@>).
type MetricRecord struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	Name      string    `gorm:"index;not null"`
	Value     float64   `gorm:"not null"`
	Tags      string    `gorm:"type:jsonb;default:'{}'"` // PostgreSQL JSONB
	Timestamp time.Time `gorm:"index;not null"`
	Unit      string
}

// ToRecord converts a domain Metric into a persistable MetricRecord.
func ToRecord(m Metric) (MetricRecord, error) {
	tags, err := json.Marshal(m.Tags)
	if err != nil {
		return MetricRecord{}, fmt.Errorf("marshal tags: %w", err)
	}
	return MetricRecord{
		Name:      m.Name,
		Value:     m.Value,
		Tags:      string(tags),
		Timestamp: m.Timestamp,
		Unit:      m.Unit,
	}, nil
}

// ToDomain converts a persisted MetricRecord back to a domain Metric.
func (r MetricRecord) ToDomain() (Metric, error) {
	var tags map[string]string
	if r.Tags != "" && r.Tags != "null" {
		if err := json.Unmarshal([]byte(r.Tags), &tags); err != nil {
			return Metric{}, fmt.Errorf("unmarshal tags: %w", err)
		}
	}
	return Metric{
		Name:      r.Name,
		Value:     r.Value,
		Tags:      tags,
		Timestamp: r.Timestamp,
		Unit:      r.Unit,
	}, nil
}
