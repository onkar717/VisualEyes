package storage

import (
	"log"
	"sync"
	"time"

	"github.com/onkar717/visual-eyes/backend/models"
)

// MemoryStore represents an in-memory metric store
type MemoryStore struct {
	metrics map[string][]models.Metric
	mutex   sync.RWMutex
}

// NewMemoryStore creates a new in-memory metric store
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		metrics: make(map[string][]models.Metric),
	}
}

// StoreMetrics stores multiple metrics in memory
func (s *MemoryStore) StoreMetrics(metrics []models.Metric) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, metric := range metrics {
		// Create a unique key for each metric type
		key := metric.Name
		if len(metric.Tags) > 0 {
			// Add tags to the key to differentiate similar metrics
			for k, v := range metric.Tags {
				key += "_" + k + "_" + v
			}
		}

		// Add timestamp if not set
		if metric.Timestamp.IsZero() {
			metric.Timestamp = time.Now()
		}

		// Store the metric
		s.metrics[key] = append(s.metrics[key], metric)

		// Keep only the last 100 metrics per key to prevent memory bloat
		if len(s.metrics[key]) > 100 {
			s.metrics[key] = s.metrics[key][len(s.metrics[key])-100:]
		}
	}

	log.Printf("Stored %d metrics in memory", len(metrics))
	return nil
}

// GetAllMetrics returns all stored metrics
func (s *MemoryStore) GetAllMetrics() []models.Metric {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var allMetrics []models.Metric
	for _, metrics := range s.metrics {
		if len(metrics) > 0 {
			// Get the latest metric for each key
			allMetrics = append(allMetrics, metrics[len(metrics)-1])
		}
	}

	log.Printf("Retrieved %d metrics from memory", len(allMetrics))
	return allMetrics
}
