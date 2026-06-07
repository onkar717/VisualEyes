package storage

import (
	"time"

	"github.com/onkar717/visual-eyes/server/models"
)

// MetricStore is the minimal interface all storage backends must satisfy.
type MetricStore interface {
	StoreMetrics(metrics []models.Metric) error
	GetAllMetrics() []models.Metric
}

// QueryableStore extends MetricStore with time-range history queries.
// Callers type-assert to QueryableStore before using history features.
type QueryableStore interface {
	MetricStore
	QueryByName(name string, since time.Time, limit int) ([]models.Metric, error)
	QueryByTags(tags map[string]string, since time.Time, limit int) ([]models.Metric, error)
}

// LogStore persists and retrieves pod log lines.
type LogStore interface {
	StoreLogs(logs []models.PodLog) error
	GetLogs(pod, namespace string, limit int) ([]models.PodLog, error)
	GetLogsSince(pod, namespace string, since time.Time) ([]models.PodLog, error)
}

// AlertStore persists and retrieves alert records.
type AlertStore interface {
	SaveAlert(a *models.Alert) error
	UpdateAlert(a *models.Alert) error
	GetActiveAlerts() ([]models.Alert, error)
	GetAlertHistory(limit int) ([]models.Alert, error)
	GetAlertByID(id uint) (*models.Alert, error)
}

// RCAStore persists and retrieves RCA results.
type RCAStore interface {
	SaveRCAResult(r *models.RCAResult) error
	UpdateRCAResult(r *models.RCAResult) error
	GetRCAResult(alertID uint) (*models.RCAResult, error)
}

// NotificationStore persists alert delivery event records.
type NotificationStore interface {
	SaveNotificationEvent(e *models.NotificationEvent) error
	GetNotificationEvents(alertID uint) ([]models.NotificationEvent, error)
	GetRecentNotificationEvents(limit int) ([]models.NotificationEvent, error)
}

// IncidentStats is the aggregated summary returned by /api/stats.
type IncidentStats struct {
	TotalIncidents int            `json:"total_incidents"`
	OpenIncidents  int            `json:"open_incidents"`
	AvgMTTRSeconds float64        `json:"avg_mttr_seconds"`
	MTTRCount      int            `json:"mttr_count"`
	BySeverity     map[string]int `json:"by_severity"`
	ByStatus       map[string]int `json:"by_status"`
}

// IncidentStore persists and queries the full incident lifecycle.
type IncidentStore interface {
	SaveIncident(inc *models.Incident) error
	UpdateIncident(inc *models.Incident) error
	GetIncidentByID(id uint) (*models.Incident, error)
	GetIncidentByAlertID(alertID uint) (*models.Incident, error)
	// GetRecentIncidents returns incidents filtered by severity/status, optionally within the last `hours` hours (0 = no time filter).
	GetRecentIncidents(severityFilter, statusFilter string, limit, hours int) ([]models.Incident, error)
	MTTRStats() (avgSeconds float64, count int, err error)
	GetStats() (IncidentStats, error)
}
