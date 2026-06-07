package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/onkar717/visual-eyes/backend/models"
)

// SlackNotifier sends alert events to a Slack incoming webhook.
type SlackNotifier struct {
	webhookURL string
	client     *http.Client
}

// NewSlackNotifier creates a notifier that posts to the given webhook URL.
func NewSlackNotifier(webhookURL string) *SlackNotifier {
	return &SlackNotifier{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

type slackPayload struct {
	Attachments []slackAttachment `json:"attachments"`
}

type slackAttachment struct {
	Color    string       `json:"color"`
	Title    string       `json:"title"`
	Text     string       `json:"text"`
	Fields   []slackField `json:"fields,omitempty"`
	Footer   string       `json:"footer"`
	Ts       int64        `json:"ts"`
}

type slackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// AlertFired sends a firing alert notification.
func (s *SlackNotifier) AlertFired(alert models.Alert) error {
	color := "#e01e5a" // critical — red
	switch alert.Severity {
	case models.SeverityWarning:
		color = "#ecb22e" // warning — amber
	case models.SeverityInfo:
		color = "#2eb886" // info — green
	}

	return s.send(slackPayload{
		Attachments: []slackAttachment{
			{
				Color: color,
				Title: fmt.Sprintf("[%s] %s", string(alert.Severity), alert.RuleName),
				Text:  alert.Message,
				Fields: []slackField{
					{Title: "Resource", Value: alert.ResourceID, Short: true},
					{Title: "Namespace", Value: alert.Namespace, Short: true},
					{Title: "Value", Value: fmt.Sprintf("%.2f", alert.Value), Short: true},
					{Title: "Threshold", Value: fmt.Sprintf("%.2f", alert.Threshold), Short: true},
					{Title: "Metric", Value: alert.MetricName, Short: true},
					{Title: "Alert ID", Value: fmt.Sprintf("%d", alert.ID), Short: true},
				},
				Footer: "VisualEyes",
				Ts:     alert.FiredAt.Unix(),
			},
		},
	})
}

// AlertResolved sends a resolution notification.
func (s *SlackNotifier) AlertResolved(alert models.Alert) error {
	return s.send(slackPayload{
		Attachments: []slackAttachment{
			{
				Color: "#2eb886",
				Title: fmt.Sprintf("[RESOLVED] %s", alert.RuleName),
				Text:  fmt.Sprintf("Alert cleared on *%s/%s*", alert.Namespace, alert.ResourceID),
				Fields: []slackField{
					{Title: "Alert ID", Value: fmt.Sprintf("%d", alert.ID), Short: true},
					{Title: "Metric", Value: alert.MetricName, Short: true},
				},
				Footer: "VisualEyes",
				Ts:     time.Now().Unix(),
			},
		},
	})
}

func (s *SlackNotifier) send(payload slackPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("slack marshal: %w", err)
	}
	resp, err := s.client.Post(s.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}
