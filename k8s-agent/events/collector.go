// Package events collects Kubernetes Events from the API server and ships them
// to the VisualEyes backend. Events carry higher-level signal than raw logs:
// OOMKilled, BackOff, FailedScheduling, etc. The RCA engine uses these to
// correlate with metric anomalies.
package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// KubeEvent is the serialised form shipped to the backend.
type KubeEvent struct {
	Namespace  string    `json:"namespace"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`       // involved object kind
	Object     string    `json:"object"`     // involved object name
	Reason     string    `json:"reason"`     // e.g. "OOMKilling", "BackOff"
	Message    string    `json:"message"`
	Type       string    `json:"type"`       // Normal | Warning
	Count      int32     `json:"count"`
	FirstSeen  time.Time `json:"firstSeen"`
	LastSeen   time.Time `json:"lastSeen"`
	SourceNode string    `json:"sourceNode"`
}

// Collector fetches recent Warning events from all namespaces.
type Collector struct {
	client     kubernetes.Interface
	httpClient *http.Client
	endpoint   string
	lastSeen   time.Time
}

// NewCollector creates a Collector using the provided kubernetes client.
func NewCollector(client kubernetes.Interface, backendEndpoint string) *Collector {
	return &Collector{
		client:   client,
		endpoint: backendEndpoint,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		lastSeen: time.Now().Add(-5 * time.Minute),
	}
}

// Collect fetches events newer than lastSeen, ships them to the backend, and
// advances the lastSeen cursor. Only Warning events are collected to reduce noise.
func (c *Collector) Collect(ctx context.Context) error {
	eventList, err := c.client.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: "type=Warning",
	})
	if err != nil {
		return fmt.Errorf("list events: %w", err)
	}

	var fresh []KubeEvent
	for _, ev := range eventList.Items {
		if !isNewer(ev, c.lastSeen) {
			continue
		}
		fresh = append(fresh, toKubeEvent(ev))
		// Advance cursor to the latest event seen.
		if t := eventTime(ev); t.After(c.lastSeen) {
			c.lastSeen = t
		}
	}

	if len(fresh) == 0 {
		return nil
	}

	return c.ship(fresh)
}

func (c *Collector) ship(events []KubeEvent) error {
	body, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}
	resp, err := c.httpClient.Post(c.endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post events: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		slog.Warn("backend rejected events", "status", resp.StatusCode, "count", len(events))
	} else {
		slog.Debug("shipped k8s events", "count", len(events))
	}
	return nil
}

func toKubeEvent(ev corev1.Event) KubeEvent {
	return KubeEvent{
		Namespace:  ev.Namespace,
		Name:       ev.Name,
		Kind:       ev.InvolvedObject.Kind,
		Object:     ev.InvolvedObject.Name,
		Reason:     ev.Reason,
		Message:    ev.Message,
		Type:       ev.Type,
		Count:      ev.Count,
		FirstSeen:  ev.FirstTimestamp.Time,
		LastSeen:   ev.LastTimestamp.Time,
		SourceNode: ev.Source.Host,
	}
}

func isNewer(ev corev1.Event, since time.Time) bool {
	return eventTime(ev).After(since)
}

func eventTime(ev corev1.Event) time.Time {
	if !ev.LastTimestamp.IsZero() {
		return ev.LastTimestamp.Time
	}
	if ev.EventTime.Time != (time.Time{}) {
		return ev.EventTime.Time
	}
	return ev.CreationTimestamp.Time
}
