package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/onkar717/visual-eyes/server/models"
)

const pdEventsV2URL = "https://events.pagerduty.com/v2/enqueue"

// PagerDutyNotifier sends alert events via the PagerDuty Events v2 API.
type PagerDutyNotifier struct {
	routingKey string
	client     *http.Client
}

// NewPagerDutyNotifier creates a notifier that fires to the given PD routing key.
func NewPagerDutyNotifier(routingKey string) *PagerDutyNotifier {
	return &PagerDutyNotifier{
		routingKey: routingKey,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

type pdEvent struct {
	RoutingKey  string    `json:"routing_key"`
	EventAction string    `json:"event_action"` // trigger | resolve
	DedupKey    string    `json:"dedup_key"`
	Payload     pdPayload `json:"payload"`
}

type pdPayload struct {
	Summary       string            `json:"summary"`
	Source        string            `json:"source"`
	Severity      string            `json:"severity"` // critical | error | warning | info
	Timestamp     string            `json:"timestamp"`
	CustomDetails map[string]string `json:"custom_details,omitempty"`
}

// AlertFired implements Notifier.
func (p *PagerDutyNotifier) AlertFired(alert models.Alert) error {
	return p.send(pdEvent{
		RoutingKey:  p.routingKey,
		EventAction: "trigger",
		DedupKey:    fmt.Sprintf("veye-alert-%d", alert.ID),
		Payload: pdPayload{
			Summary:   fmt.Sprintf("[%s] %s   %s/%s", string(alert.Severity), alert.RuleName, alert.Namespace, alert.ResourceID),
			Source:    "visual-eyes",
			Severity:  pdSeverity(alert.Severity),
			Timestamp: alert.FiredAt.UTC().Format(time.RFC3339),
			CustomDetails: map[string]string{
				"alert_id":   fmt.Sprintf("%d", alert.ID),
				"metric":     alert.MetricName,
				"value":      fmt.Sprintf("%.2f", alert.Value),
				"threshold":  fmt.Sprintf("%.2f", alert.Threshold),
				"namespace":  alert.Namespace,
				"resource":   alert.ResourceID,
				"message":    alert.Message,
			},
		},
	})
}

// AlertResolved implements Notifier.
func (p *PagerDutyNotifier) AlertResolved(alert models.Alert) error {
	return p.send(pdEvent{
		RoutingKey:  p.routingKey,
		EventAction: "resolve",
		DedupKey:    fmt.Sprintf("veye-alert-%d", alert.ID),
		Payload: pdPayload{
			Summary:   fmt.Sprintf("[RESOLVED] %s", alert.RuleName),
			Source:    "visual-eyes",
			Severity:  "info",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	})
}

func (p *PagerDutyNotifier) send(event pdEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("pagerduty marshal: %w", err)
	}
	resp, err := p.client.Post(pdEventsV2URL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("pagerduty post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("pagerduty returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func pdSeverity(s models.AlertSeverity) string {
	switch s {
	case models.SeverityCritical:
		return "critical"
	case models.SeverityWarning:
		return "warning"
	default:
		return "info"
	}
}
