// Package client provides a typed HTTP client for the VisualEyes backend API.
package client

import (
	"encoding/json"
	"fmt"
	"net/http"
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
