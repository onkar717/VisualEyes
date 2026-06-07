// Package client provides a typed HTTP client for the VisualEyes backend API.
package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Alert mirrors the backend models.Alert JSON shape.
type Alert struct {
	ID         uint    `json:"id"`
	RuleName   string  `json:"ruleName"`
	Severity   string  `json:"severity"`
	Status     string  `json:"status"`
	ResourceID string  `json:"resourceID"`
	Namespace  string  `json:"namespace"`
	Value      float64 `json:"value"`
	Threshold  float64 `json:"threshold"`
	Message    string  `json:"message"`
	FiredAt    string  `json:"firedAt"`
	ResolvedAt string  `json:"resolvedAt,omitempty"`
	RCAStatus  string  `json:"rcaStatus"`
	RCAID      *uint   `json:"rcaID,omitempty"`
}

// FixCommand mirrors models.FixCommand.
type FixCommand struct {
	Command    string `json:"command"`
	IsAutoSafe bool   `json:"isAutoSafe"`
	Status     string `json:"status"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
}

// RCAResult mirrors models.RCAResult.
type RCAResult struct {
	ID          uint   `json:"id"`
	AlertID     uint   `json:"alertID"`
	Explanation string `json:"explanation"`
	RootCause   string `json:"rootCause"`
	Commands    string `json:"commands"` // JSON string of []FixCommand
	Status      string `json:"status"`
	Model       string `json:"model"`
	InputTokens int    `json:"inputTokens"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// PodLog mirrors models.PodLog.
type PodLog struct {
	ID        uint   `json:"id"`
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Node      string `json:"node"`
	Stream    string `json:"stream"`
	Line      string `json:"line"`
	Timestamp string `json:"timestamp"`
}

// MetricValue is a single metric snapshot value.
type MetricValue struct {
	Value     float64           `json:"value"`
	Unit      string            `json:"unit"`
	Tags      map[string]string `json:"tags"`
	Timestamp string            `json:"timestamp"`
}

// Snapshot is the /api/metrics/snapshot response.
type Snapshot struct {
	Timestamp string                         `json:"timestamp"`
	Metrics   map[string]map[string]MetricValue `json:"metrics"`
}

// HealthResponse is the /healthz response.
type HealthResponse struct {
	Status     string            `json:"status"`
	Uptime     string            `json:"uptime"`
	Components map[string]bool   `json:"components"`
}

// ScanIssue is a single finding from /api/scan.
type ScanIssue struct {
	Severity string `json:"severity"`
	Category string `json:"category"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
	Value    string `json:"value,omitempty"`
}

// ScanSummary is the high-level metrics from /api/scan.
type ScanSummary struct {
	ActiveAlerts   int     `json:"activeAlerts"`
	CriticalAlerts int     `json:"criticalAlerts"`
	WarningAlerts  int     `json:"warningAlerts"`
	CPUPercent     float64 `json:"cpuPercent"`
	MemoryPercent  float64 `json:"memoryPercent"`
	DiskPercent    float64 `json:"diskPercent"`
}

// ScanResult is the /api/scan response.
type ScanResult struct {
	Timestamp  string      `json:"timestamp"`
	Overall    string      `json:"overall"`
	IssueCount int         `json:"issueCount"`
	Issues     []ScanIssue `json:"issues"`
	Summary    ScanSummary `json:"summary"`
}

// NotificationEvent mirrors models.NotificationEvent.
type NotificationEvent struct {
	ID        uint   `json:"id"`
	AlertID   uint   `json:"alertID"`
	RuleName  string `json:"ruleName"`
	Severity  string `json:"severity"`
	EventType string `json:"eventType"`
	Channel   string `json:"channel"`
	Success   bool   `json:"success"`
	ErrMsg    string `json:"error,omitempty"`
	CreatedAt string `json:"createdAt"`
}

// Incident mirrors models.Incident — full lifecycle record.
type Incident struct {
	ID                  uint   `json:"id"`
	IncidentCode        string `json:"incidentCode"`
	AlertID             uint   `json:"alertID"`
	RCAID               *uint  `json:"rcaID,omitempty"`
	Title               string `json:"title"`
	Severity            string `json:"severity"` // SEV1|SEV2|SEV3|SEV4
	Category            string `json:"category"`
	Status              string `json:"status"` // OPEN|INVESTIGATING|MITIGATED|RESOLVED
	RootCause           string `json:"rootCause"`
	ContributingFactors string `json:"contributingFactors"` // JSON []string
	AffectedServices    string `json:"affectedServices"`    // JSON []string
	ConfidenceScore     int    `json:"confidenceScore"`
	MTTRSeconds         *int   `json:"mttrSeconds,omitempty"`
	DetectedAt          string `json:"detectedAt"`
	MitigatedAt         string `json:"mitigatedAt,omitempty"`
	ResolvedAt          string `json:"resolvedAt,omitempty"`
	CreatedAt           string `json:"createdAt"`
}

// IncidentListResponse is the /api/incidents/full response envelope.
type IncidentListResponse struct {
	Incidents      []Incident `json:"incidents"`
	Count          int        `json:"count"`
	MTTRAvgSeconds float64    `json:"mttr_avg_seconds"`
	MTTRCount      int        `json:"mttr_count"`
}

// K8sMetrics is the /api/kubernetes/metrics response.
type K8sMetrics struct {
	Timestamp string `json:"timestamp"`
	Metrics   struct {
		Nodes struct {
			Total int `json:"total"`
			Ready int `json:"ready"`
		} `json:"nodes"`
		Pods struct {
			Total   int `json:"total"`
			Running int `json:"running"`
		} `json:"pods"`
		Resources struct {
			CPU    struct{ Usage, Total float64 } `json:"cpu"`
			Memory struct{ Usage, Total float64 } `json:"memory"`
		} `json:"resources"`
	} `json:"metrics"`
}

// Client is the VisualEyes API client.
type Client struct {
	base string
	http *http.Client
}

// New creates a Client pointed at the given base URL (e.g. "http://localhost:8080").
func New(base string) *Client {
	return &Client{
		base: base,
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) get(path string, out any) error {
	resp, err := c.http.Get(c.base + path)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Health calls /healthz.
func (c *Client) Health() (*HealthResponse, error) {
	var r HealthResponse
	return &r, c.get("/healthz", &r)
}

// Snapshot calls /api/metrics/snapshot.
func (c *Client) Snapshot() (*Snapshot, error) {
	var r Snapshot
	return &r, c.get("/api/metrics/snapshot", &r)
}

// K8sMetrics calls /api/kubernetes/metrics.
func (c *Client) K8s() (*K8sMetrics, error) {
	var r K8sMetrics
	return &r, c.get("/api/kubernetes/metrics", &r)
}

// Alerts calls /api/alerts?status=<filter>.
func (c *Client) Alerts(status string) ([]Alert, error) {
	var r struct {
		Alerts []Alert `json:"alerts"`
		Count  int     `json:"count"`
	}
	if status == "" {
		status = "firing"
	}
	return r.Alerts, c.get("/api/alerts?status="+status, &r)
}

// AlertByID calls /api/alerts/{id}.
func (c *Client) AlertByID(id uint) (*Alert, error) {
	var r Alert
	return &r, c.get(fmt.Sprintf("/api/alerts/%d", id), &r)
}

// RCA calls /api/rca/{alertID}.
func (c *Client) RCA(alertID uint) (*RCAResult, error) {
	var r RCAResult
	return &r, c.get(fmt.Sprintf("/api/rca/%d", alertID), &r)
}

// Scan calls /api/scan and returns a cluster health assessment.
func (c *Client) Scan() (*ScanResult, error) {
	var r ScanResult
	return &r, c.get("/api/scan", &r)
}

// FullIncidents calls /api/incidents/full and returns structured incident records.
// hours=0 means no time filter; hours>0 restricts to the last N hours.
func (c *Client) FullIncidents(limit int, severity, status string, hours int) (*IncidentListResponse, error) {
	var r IncidentListResponse
	q := fmt.Sprintf("/api/incidents/full?limit=%d", limit)
	if severity != "" {
		q += "&severity=" + severity
	}
	if status != "" {
		q += "&status=" + status
	}
	if hours > 0 {
		q += fmt.Sprintf("&hours=%d", hours)
	}
	return &r, c.get(q, &r)
}

// UpdateIncidentStatus PATCHes an incident to a new status.
func (c *Client) UpdateIncidentStatus(id uint, status string) (*Incident, error) {
	body := fmt.Sprintf(`{"status":%q}`, status)
	resp, err := c.http.Post(
		fmt.Sprintf("%s/api/incidents/full/%d", c.base, id),
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("patch incident: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("patch incident: HTTP %d", resp.StatusCode)
	}
	var inc Incident
	return &inc, json.NewDecoder(resp.Body).Decode(&inc)
}

// HealthScore extracts the health_score from /healthz.
func (c *Client) HealthScore() (float64, error) {
	var r struct {
		HealthScore float64 `json:"health_score"`
	}
	return r.HealthScore, c.get("/healthz", &r)
}

// GetIncident calls GET /api/incidents/full/{id} — single incident detail.
func (c *Client) GetIncident(id uint) (*Incident, error) {
	var inc Incident
	return &inc, c.get(fmt.Sprintf("/api/incidents/full/%d", id), &inc)
}

// ClusterHealth mirrors models.ClusterHealth.
type ClusterHealth struct {
	ID            uint      `json:"id"`
	Name          string    `json:"name"`
	Namespace     string    `json:"namespace"`
	LastSeen      time.Time `json:"last_seen"`
	HealthScore   float64   `json:"health_score"`
	TotalNodes    int       `json:"total_nodes"`
	ReadyNodes    int       `json:"ready_nodes"`
	TotalPods     int       `json:"total_pods"`
	RunningPods   int       `json:"running_pods"`
	PendingPods   int       `json:"pending_pods"`
	FailedPods    int       `json:"failed_pods"`
	CrashloopPods int       `json:"crashloop_pods"`
	OpenIncidents int       `json:"open_incidents"`
}

// ListClusters calls /api/clusters and returns all registered clusters.
func (c *Client) ListClusters() ([]ClusterHealth, error) {
	var clusters []ClusterHealth
	return clusters, c.get("/api/clusters", &clusters)
}

// Incidents calls /api/incidents and returns recent notification delivery events.
func (c *Client) Incidents(limit int, alertID uint) ([]NotificationEvent, error) {
	var r []NotificationEvent
	q := fmt.Sprintf("/api/incidents?limit=%d", limit)
	if alertID > 0 {
		q += fmt.Sprintf("&alert_id=%d", alertID)
	}
	return r, c.get(q, &r)
}

// Logs calls /api/pod-logs with optional filters.
func (c *Client) Logs(pod, namespace, container string, limit int) ([]PodLog, error) {
	var r struct {
		Logs  []PodLog `json:"logs"`
		Count int      `json:"count"`
	}
	q := fmt.Sprintf("/api/pod-logs?limit=%d", limit)
	if pod != "" {
		q += "&pod=" + pod
	}
	if namespace != "" {
		q += "&namespace=" + namespace
	}
	if container != "" {
		q += "&container=" + container
	}
	return r.Logs, c.get(q, &r)
}
