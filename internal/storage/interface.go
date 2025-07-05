package storage

import (
	"github.com/onkar717/visual-eyes/internal/models"
)

// MetricStore defines the interface for storing and retrieving metrics
type MetricStore interface {
	StoreMetrics(metrics []models.Metric) error
	GetAllMetrics() []models.Metric
	GetMetricsByName(name string) []models.Metric
}
