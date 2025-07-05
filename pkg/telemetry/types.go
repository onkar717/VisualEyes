package telemetry

import (
	"context"
	"time"
)

// DataType represents the type of telemetry data
type DataType string

const (
	TypeMetric DataType = "metric"
)

// Metric represents a single metric data point
type Metric struct {
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Tags      map[string]string `json:"tags,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Unit      string            `json:"unit,omitempty"`
}

// Collector interface defines methods that must be implemented by metric collectors
type Collector interface {
	Name() string
	Type() DataType
	Collect(ctx context.Context) ([]Metric, error)
}
