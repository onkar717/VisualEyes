package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/onkar717/visual-eyes/server/models"
)

// SlackNotifier sends alert events to a Slack incoming webhook using Block Kit.
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

// --- Block Kit types ---

type blockKitPayload struct {
	Blocks []bkBlock `json:"blocks"`
}

type bkBlock struct {
	Type     string      `json:"type"`
	Text     *bkText     `json:"text,omitempty"`
	Fields   []bkText    `json:"fields,omitempty"`
	Elements []bkElement `json:"elements,omitempty"`
}

type bkText struct {
	Type  string `json:"type"` // plain_text | mrkdwn
	Text  string `json:"text"`
	Emoji bool   `json:"emoji,omitempty"`
}

type bkElement struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// sevEmoji maps severity to an emoji prefix for header readability.
func sevEmoji(sev models.AlertSeverity) string {
	switch sev {
	case models.SeverityCritical:
		return ":red_circle:"
	case models.SeverityWarning:
		return ":large_yellow_circle:"
	default:
		return ":large_green_circle:"
	}
}

// AlertFired sends a firing alert notification using Block Kit.
func (s *SlackNotifier) AlertFired(alert models.Alert) error {
	header := fmt.Sprintf("%s *[%s] %s*", sevEmoji(alert.Severity), string(alert.Severity), alert.RuleName)

	fields := []bkText{
		{Type: "mrkdwn", Text: fmt.Sprintf("*Resource*\n%s", alert.ResourceID)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Namespace*\n%s", alert.Namespace)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Metric*\n%s", alert.MetricName)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Value / Threshold*\n%.2f / %.2f", alert.Value, alert.Threshold)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Alert ID*\n%d", alert.ID)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Fired At*\n<!date^%d^{date_short_pretty} {time}|%s>",
			alert.FiredAt.Unix(), alert.FiredAt.Format(time.RFC3339))},
	}

	payload := blockKitPayload{Blocks: []bkBlock{
		{
			Type: "section",
			Text: &bkText{Type: "mrkdwn", Text: header},
		},
		{
			Type: "section",
			Text: &bkText{Type: "mrkdwn", Text: alert.Message},
		},
		{
			Type:   "section",
			Fields: fields,
		},
		{
			Type: "divider",
		},
		{
			Type: "context",
			Elements: []bkElement{
				{Type: "mrkdwn", Text: ":telescope: *VisualEyes* | Autonomous Observability Platform"},
			},
		},
	}}

	return s.send(payload)
}

// AlertResolved sends a resolution notification using Block Kit.
func (s *SlackNotifier) AlertResolved(alert models.Alert) error {
	payload := blockKitPayload{Blocks: []bkBlock{
		{
			Type: "section",
			Text: &bkText{
				Type: "mrkdwn",
				Text: fmt.Sprintf(":large_green_circle: *[RESOLVED] %s*", alert.RuleName),
			},
		},
		{
			Type: "section",
			Fields: []bkText{
				{Type: "mrkdwn", Text: fmt.Sprintf("*Resource*\n%s/%s", alert.Namespace, alert.ResourceID)},
				{Type: "mrkdwn", Text: fmt.Sprintf("*Metric*\n%s", alert.MetricName)},
				{Type: "mrkdwn", Text: fmt.Sprintf("*Alert ID*\n%d", alert.ID)},
				{Type: "mrkdwn", Text: fmt.Sprintf("*Resolved At*\n<!date^%d^{date_short_pretty} {time}|%s>",
					time.Now().Unix(), time.Now().Format(time.RFC3339))},
			},
		},
		{
			Type: "context",
			Elements: []bkElement{
				{Type: "mrkdwn", Text: ":telescope: *VisualEyes* | Autonomous Observability Platform"},
			},
		},
	}}

	return s.send(payload)
}

func (s *SlackNotifier) send(payload blockKitPayload) error {
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
