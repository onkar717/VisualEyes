package models

import (
	"fmt"
	"time"
)

// Metric represents a single telemetry measurement
type Metric struct {
	ID        uint              `json:"-"`
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Tags      map[string]string `json:"tags,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Unit      string            `json:"unit,omitempty"`
	CreatedAt time.Time         `json:"-"`
}

// Validate checks if the metric has all required fields
func (m *Metric) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("metric name is required")
	}
	if m.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	return nil
}
