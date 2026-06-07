package storage

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/onkar717/visual-eyes/server/models"
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
		&models.Incident{},
		&models.RemediationLogEntry{},
		&models.ClusterHealth{},
		&models.ClusterSnapshot{},
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

// ═══════════════════════════════════════════════════════════════════
// IncidentStore
// ═══════════════════════════════════════════════════════════════════

func (s *PostgresStore) SaveIncident(inc *models.Incident) error {
	if err := s.db.Create(inc).Error; err != nil {
		return fmt.Errorf("save incident: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateIncident(inc *models.Incident) error {
	if err := s.db.Save(inc).Error; err != nil {
		return fmt.Errorf("update incident: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetIncidentByID(id uint) (*models.Incident, error) {
	var inc models.Incident
	if err := s.db.First(&inc, id).Error; err != nil {
		return nil, fmt.Errorf("get incident %d: %w", id, err)
	}
	return &inc, nil
}

func (s *PostgresStore) GetIncidentByAlertID(alertID uint) (*models.Incident, error) {
	var inc models.Incident
	if err := s.db.Where("alert_id = ?", alertID).Order("created_at DESC").First(&inc).Error; err != nil {
		return nil, fmt.Errorf("get incident for alert %d: %w", alertID, err)
	}
	return &inc, nil
}

func (s *PostgresStore) GetRecentIncidents(severityFilter, statusFilter string, limit, hours int) ([]models.Incident, error) {
	q := s.db.Order("detected_at DESC")
	if severityFilter != "" {
		q = q.Where("severity = ?", severityFilter)
	}
	if statusFilter != "" {
		q = q.Where("status = ?", statusFilter)
	}
	if hours > 0 {
		cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
		q = q.Where("detected_at >= ?", cutoff)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	var incidents []models.Incident
	if err := q.Find(&incidents).Error; err != nil {
		return nil, fmt.Errorf("get recent incidents: %w", err)
	}
	return incidents, nil
}

func (s *PostgresStore) GetStats() (IncidentStats, error) {
	st := IncidentStats{
		BySeverity: make(map[string]int),
		ByStatus:   make(map[string]int),
	}
	var total int64
	s.db.Model(&models.Incident{}).Count(&total)
	st.TotalIncidents = int(total)

	var open int64
	s.db.Model(&models.Incident{}).Where("status IN ?", []string{"OPEN", "INVESTIGATING"}).Count(&open)
	st.OpenIncidents = int(open)

	type sevRow struct {
		Severity string
		Count    int
	}
	var sevRows []sevRow
	s.db.Model(&models.Incident{}).Select("severity, count(*) as count").Group("severity").Scan(&sevRows)
	for _, r := range sevRows {
		st.BySeverity[r.Severity] = r.Count
	}

	type statusRow struct {
		Status string
		Count  int
	}
	var statusRows []statusRow
	s.db.Model(&models.Incident{}).Select("status, count(*) as count").Group("status").Scan(&statusRows)
	for _, r := range statusRows {
		st.ByStatus[r.Status] = r.Count
	}

	avg, count, err := s.MTTRStats()
	st.AvgMTTRSeconds = avg
	st.MTTRCount = count
	if avgs, counts, merr := s.mttrBySeverityFull(); merr == nil {
		st.MTTRBySeverity = avgs
		st.MTTRCountBySeverity = counts
	}
	return st, err
}

func (s *PostgresStore) MTTRStats() (float64, int, error) {
	var result struct {
		Avg   float64 `gorm:"column:avg"`
		Count int     `gorm:"column:count"`
	}
	err := s.db.Raw(`
		SELECT AVG(mttr_seconds) AS avg, COUNT(*) AS count
		FROM incidents
		WHERE mttr_seconds IS NOT NULL
	`).Scan(&result).Error
	if err != nil {
		return 0, 0, fmt.Errorf("mttr stats: %w", err)
	}
	return result.Avg, result.Count, nil
}

// mttrBySeverityFull returns both avg MTTR and incident count per severity.
func (s *PostgresStore) mttrBySeverityFull() (map[string]float64, map[string]int, error) {
	type row struct {
		Severity string  `gorm:"column:severity"`
		Avg      float64 `gorm:"column:avg"`
		Count    int     `gorm:"column:cnt"`
	}
	var rows []row
	err := s.db.Raw(`
		SELECT severity, AVG(mttr_seconds) AS avg, COUNT(*) AS cnt
		FROM incidents
		WHERE mttr_seconds IS NOT NULL
		GROUP BY severity
	`).Scan(&rows).Error
	if err != nil {
		return nil, nil, fmt.Errorf("mttr by severity: %w", err)
	}
	avgs := make(map[string]float64, len(rows))
	counts := make(map[string]int, len(rows))
	for _, r := range rows {
		avgs[r.Severity] = r.Avg
		counts[r.Severity] = r.Count
	}
	return avgs, counts, nil
}

func (s *PostgresStore) MTTRStatsBySeverity() (map[string]float64, error) {
	avgs, _, err := s.mttrBySeverityFull()
	return avgs, err
}

func (s *PostgresStore) FindOpenByCategory(category, namespace string, windowHours int) (*models.Incident, error) {
	var inc models.Incident
	cutoff := time.Now().Add(-time.Duration(windowHours) * time.Hour)
	err := s.db.Where(
		"category = ? AND status IN ('OPEN','INVESTIGATING') AND created_at >= ?",
		category, cutoff,
	).Order("created_at DESC").First(&inc).Error
	if err != nil {
		return nil, err
	}
	return &inc, nil
}

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

// ClusterStore
func (s *PostgresStore) UpsertCluster(c *models.ClusterHealth) error {
	return s.db.Save(c).Error
}

func (s *PostgresStore) GetCluster(name string) (*models.ClusterHealth, error) {
	var c models.ClusterHealth
	err := s.db.Where("name = ?", name).First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *PostgresStore) ListClusters() ([]models.ClusterHealth, error) {
	var clusters []models.ClusterHealth
	err := s.db.Order("name ASC").Find(&clusters).Error
	return clusters, err
}

// RemediationLogStore
func (s *PostgresStore) SaveRemediationLog(e *models.RemediationLogEntry) error {
	return s.db.Create(e).Error
}

func (s *PostgresStore) GetRemediationLogs(incidentID uint) ([]models.RemediationLogEntry, error) {
	var logs []models.RemediationLogEntry
	err := s.db.Where("incident_id = ?", incidentID).Order("step_number ASC").Find(&logs).Error
	return logs, err
}

func (s *PostgresStore) GetRecentRemediationLogs(limit int) ([]models.RemediationLogEntry, error) {
	var logs []models.RemediationLogEntry
	err := s.db.Order("executed_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

// ═══════════════════════════════════════════════════════════════════
// ClusterSnapshotStore
// ═══════════════════════════════════════════════════════════════════

func (s *PostgresStore) SaveSnapshot(snap *models.ClusterSnapshot) error {
	return s.db.Create(snap).Error
}

func (s *PostgresStore) GetSnapshots(clusterName string, hours, limit int) ([]models.ClusterSnapshot, error) {
	if limit <= 0 {
		limit = 288 // 24 h at 5-min resolution
	}
	q := s.db.Where("cluster_name = ?", clusterName).Order("recorded_at ASC")
	if hours > 0 {
		q = q.Where("recorded_at >= ?", time.Now().Add(-time.Duration(hours)*time.Hour))
	}
	var snaps []models.ClusterSnapshot
	err := q.Limit(limit).Find(&snaps).Error
	return snaps, err
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
