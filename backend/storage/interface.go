package storage

import (
	"time"

	"github.com/onkar717/visual-eyes/backend/models"
)

// MetricStore is the minimal interface all storage backends must satisfy.
type MetricStore interface {
	StoreMetrics(metrics []models.Metric) error
	GetAllMetrics() []models.Metric
}

// QueryableStore extends MetricStore with time-range history queries.
// SQLiteStore (Commit 2) implements this; MemoryStore does not.
// Callers type-assert to QueryableStore before using history features.
type QueryableStore interface {
	MetricStore
	QueryByName(name string, since time.Time, limit int) ([]models.Metric, error)
	QueryByTags(tags map[string]string, since time.Time, limit int) ([]models.Metric, error)
}

// LogStore persists and retrieves pod log lines.
// Wired in Commit 4.
type LogStore interface {
	StoreLogs(logs []models.PodLog) error
	GetLogs(pod, namespace string, limit int) ([]models.PodLog, error)
	GetLogsSince(pod, namespace string, since time.Time) ([]models.PodLog, error)
}

// AlertStore persists and retrieves alert records.
// Wired in Commit 3.
type AlertStore interface {
	SaveAlert(a *models.Alert) error
	UpdateAlert(a *models.Alert) error
	GetActiveAlerts() ([]models.Alert, error)
	GetAlertHistory(limit int) ([]models.Alert, error)
	GetAlertByID(id uint) (*models.Alert, error)
}

// RCAStore persists and retrieves RCA results.
// Wired in Commit 5.
type RCAStore interface {
	SaveRCAResult(r *models.RCAResult) error
	UpdateRCAResult(r *models.RCAResult) error
	GetRCAResult(alertID uint) (*models.RCAResult, error)
}
