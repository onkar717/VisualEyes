package storage

import (
	"github.com/onkar717/visual-eyes/backend/models"
)

// MetricStore defines the interface for storing and retrieving metrics
type MetricStore interface {
	StoreMetrics(metrics []models.Metric) error
	GetAllMetrics() []models.Metric
}
