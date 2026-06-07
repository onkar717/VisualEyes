package logs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// wireLog is the JSON payload sent to the backend /api/pod-logs endpoint.
type wireLog struct {
	Pod       string    `json:"pod"`
	Namespace string    `json:"namespace"`
	Container string    `json:"container"`
	Node      string    `json:"node"`
	Stream    string    `json:"stream"`
	Line      string    `json:"line"`
	Timestamp time.Time `json:"timestamp"`
}

// Shipper batches LogLines and POSTs them to the backend.
type Shipper struct {
	endpoint   string
	httpClient *http.Client
}

// NewShipper creates a Shipper targeting the given backend endpoint.
func NewShipper(endpoint string) *Shipper {
	return &Shipper{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Ship sends a batch of log lines to the backend. Silently retries once on
// transient errors; logs and continues on persistent failure.
func (s *Shipper) Ship(lines []LogLine) error {
	if len(lines) == 0 {
		return nil
	}

	payload := make([]wireLog, len(lines))
	for i, l := range lines {
		payload[i] = wireLog{
			Pod:       l.Pod,
			Namespace: l.Namespace,
			Container: l.Container,
			Node:      l.Node,
			Stream:    l.Stream,
			Line:      l.Line,
			Timestamp: l.Timestamp,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal log batch: %w", err)
	}

	resp, err := s.httpClient.Post(s.endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		slog.Warn("backend rejected log batch", "status", resp.StatusCode, "lines", len(lines))
	} else {
		slog.Debug("shipped log batch", "lines", len(lines))
	}
	return nil
}
