package storage

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/onkar717/visual-eyes/backend/models"
)

// PostgresStore is a GORM-backed PostgreSQL implementation of all storage interfaces:
// MetricStore, QueryableStore, LogStore, AlertStore, and RCAStore.
// A single instance is shared across all features.
type PostgresStore struct {
	db         *gorm.DB
	maxRecords int
	pruneOnce  sync.Once
	mu         sync.Mutex
}

// NewPostgresStore opens a connection to PostgreSQL using the provided DSN,
// runs AutoMigrate for all tables, and returns a ready-to-use store.
//
// Example DSN:
//
//	"host=localhost user=visual_eyes password=secret dbname=visual_eyes port=5432 sslmode=disable TimeZone=UTC"
func NewPostgresStore(dsn string, maxRecords int) (*PostgresStore, error) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true, // disable implicit prepared statements — safer for PgBouncer
	}), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	// Connection pool tuning.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	if err := db.AutoMigrate(
		&models.MetricRecord{},
		&models.PodLog{},
		&models.Alert{},
		&models.RCAResult{},
		&models.NotificationEvent{},
	); err != nil {
		return nil, fmt.Errorf("auto-migrate: %w", err)
	}

	// Create a partial index to speed up GetAllMetrics (latest-per-name query).
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_metric_records_name_ts ON metric_records (name, timestamp DESC)`)

	if maxRecords <= 0 {
		maxRecords = 10000
	}

	slog.Info("postgres store opened", "max_records_per_metric", maxRecords)
	return &PostgresStore{db: db, maxRecords: maxRecords}, nil
}

// ═══════════════════════════════════════════════════════════════════
// MetricStore
// ═══════════════════════════════════════════════════════════════════

// StoreMetrics persists a batch of metrics and asynchronously prunes old rows.
func (s *PostgresStore) StoreMetrics(metrics []models.Metric) error {
	records := make([]models.MetricRecord, 0, len(metrics))
	for _, m := range metrics {
		r, err := models.ToRecord(m)
		if err != nil {
			return fmt.Errorf("convert metric %q: %w", m.Name, err)
		}
		records = append(records, r)
	}

	if result := s.db.Create(&records); result.Error != nil {
		return fmt.Errorf("insert metrics: %w", result.Error)
	}

	go s.pruneMetrics(metrics)
	return nil
}

// GetAllMetrics returns the single latest value for each distinct metric name.
// Uses a DISTINCT ON query which is idiomatic and fast in PostgreSQL.
func (s *PostgresStore) GetAllMetrics() []models.Metric {
	var records []models.MetricRecord
	s.db.Raw(`
		SELECT DISTINCT ON (name) *
		FROM metric_records
		ORDER BY name, timestamp DESC
	`).Scan(&records)

	return toMetrics(records)
}

// ═══════════════════════════════════════════════════════════════════
// QueryableStore
// ═══════════════════════════════════════════════════════════════════

// QueryByName returns up to limit metric samples since a given time, oldest first.
func (s *PostgresStore) QueryByName(name string, since time.Time, limit int) ([]models.Metric, error) {
	var records []models.MetricRecord
	err := s.db.
		Where("name = ? AND timestamp >= ?", name, since).
		Order("timestamp ASC").
		Limit(limit).
		Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("query by name: %w", err)
	}
	return toMetrics(records), nil
}

// QueryByTags returns samples matching all given tag key=value pairs since a time.
// Uses PostgreSQL JSONB containment (@>) for efficient tag filtering.
func (s *PostgresStore) QueryByTags(tags map[string]string, since time.Time, limit int) ([]models.Metric, error) {
	// Build a JSONB containment object: {"key1":"val1","key2":"val2"}
	tagJSON := "{"
	i := 0
	for k, v := range tags {
		if i > 0 {
			tagJSON += ","
		}
		tagJSON += fmt.Sprintf(`"%s":"%s"`, k, v)
		i++
	}
	tagJSON += "}"

	var records []models.MetricRecord
	err := s.db.
		Where("tags @> ?::jsonb AND timestamp >= ?", tagJSON, since).
		Order("timestamp ASC").
		Limit(limit).
		Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("query by tags: %w", err)
	}
	return toMetrics(records), nil
}

// ═══════════════════════════════════════════════════════════════════
// LogStore
// ═══════════════════════════════════════════════════════════════════

func (s *PostgresStore) StoreLogs(logs []models.PodLog) error {
	if len(logs) == 0 {
		return nil
	}
	if err := s.db.Create(&logs).Error; err != nil {
		return fmt.Errorf("insert logs: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetLogs(pod, namespace string, limit int) ([]models.PodLog, error) {
	var logs []models.PodLog
	err := s.db.
		Where("pod = ? AND namespace = ?", pod, namespace).
		Order("timestamp DESC").
		Limit(limit).
		Find(&logs).Error
	if err != nil {
		return nil, fmt.Errorf("get logs: %w", err)
	}
	// Reverse to return oldest-first for display.
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}
	return logs, nil
}

func (s *PostgresStore) GetLogsSince(pod, namespace string, since time.Time) ([]models.PodLog, error) {
	var logs []models.PodLog
	err := s.db.
		Where("pod = ? AND namespace = ? AND timestamp >= ?", pod, namespace, since).
		Order("timestamp ASC").
		Find(&logs).Error
	if err != nil {
		return nil, fmt.Errorf("get logs since: %w", err)
	}
	return logs, nil
}

// ═══════════════════════════════════════════════════════════════════
// AlertStore
// ═══════════════════════════════════════════════════════════════════

func (s *PostgresStore) SaveAlert(a *models.Alert) error {
	if err := s.db.Create(a).Error; err != nil {
		return fmt.Errorf("save alert: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateAlert(a *models.Alert) error {
	if err := s.db.Save(a).Error; err != nil {
		return fmt.Errorf("update alert: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetActiveAlerts() ([]models.Alert, error) {
	var alerts []models.Alert
	err := s.db.Where("status = ?", models.AlertFiring).
		Order("fired_at DESC").Find(&alerts).Error
	if err != nil {
		return nil, fmt.Errorf("get active alerts: %w", err)
	}
	return alerts, nil
}

func (s *PostgresStore) GetAlertHistory(limit int) ([]models.Alert, error) {
	var alerts []models.Alert
	if err := s.db.Order("fired_at DESC").Limit(limit).Find(&alerts).Error; err != nil {
		return nil, fmt.Errorf("get alert history: %w", err)
	}
	return alerts, nil
}

func (s *PostgresStore) GetAlertByID(id uint) (*models.Alert, error) {
	var a models.Alert
	if err := s.db.First(&a, id).Error; err != nil {
		return nil, fmt.Errorf("get alert %d: %w", id, err)
	}
	return &a, nil
}

// ═══════════════════════════════════════════════════════════════════
// RCAStore
// ═══════════════════════════════════════════════════════════════════

func (s *PostgresStore) SaveRCAResult(r *models.RCAResult) error {
	if err := s.db.Create(r).Error; err != nil {
		return fmt.Errorf("save rca: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateRCAResult(r *models.RCAResult) error {
	if err := s.db.Save(r).Error; err != nil {
		return fmt.Errorf("update rca: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetRCAResult(alertID uint) (*models.RCAResult, error) {
	var r models.RCAResult
	if err := s.db.Where("alert_id = ?", alertID).First(&r).Error; err != nil {
		return nil, fmt.Errorf("get rca for alert %d: %w", alertID, err)
	}
	return &r, nil
}

// ═══════════════════════════════════════════════════════════════════
// Internal helpers
// ═══════════════════════════════════════════════════════════════════

// ═══════════════════════════════════════════════════════════════════
// NotificationStore
// ═══════════════════════════════════════════════════════════════════

func (s *PostgresStore) SaveNotificationEvent(e *models.NotificationEvent) error {
	if err := s.db.Create(e).Error; err != nil {
		return fmt.Errorf("save notification event: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetNotificationEvents(alertID uint) ([]models.NotificationEvent, error) {
	var events []models.NotificationEvent
	err := s.db.Where("alert_id = ?", alertID).Order("created_at DESC").Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("get notification events for alert %d: %w", alertID, err)
	}
	return events, nil
}

func (s *PostgresStore) GetRecentNotificationEvents(limit int) ([]models.NotificationEvent, error) {
	var events []models.NotificationEvent
	err := s.db.Order("created_at DESC").Limit(limit).Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("get recent notification events: %w", err)
	}
	return events, nil
}

// pruneMetrics deletes oldest rows to keep at most maxRecords per metric name.
func (s *PostgresStore) pruneMetrics(metrics []models.Metric) {
	seen := make(map[string]struct{}, len(metrics))
	for _, m := range metrics {
		seen[m.Name] = struct{}{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for name := range seen {
		var count int64
		s.db.Model(&models.MetricRecord{}).Where("name = ?", name).Count(&count)
		if count <= int64(s.maxRecords) {
			continue
		}
		excess := count - int64(s.maxRecords)
		s.db.Exec(`
			DELETE FROM metric_records
			WHERE id IN (
				SELECT id FROM metric_records WHERE name = $1 ORDER BY timestamp ASC LIMIT $2
			)`, name, excess)
	}
}

func toMetrics(records []models.MetricRecord) []models.Metric {
	out := make([]models.Metric, 0, len(records))
	for _, r := range records {
		m, err := r.ToDomain()
		if err != nil {
			slog.Warn("skip corrupt metric record", "id", r.ID, "error", err)
			continue
		}
		out = append(out, m)
	}
	return out
}
