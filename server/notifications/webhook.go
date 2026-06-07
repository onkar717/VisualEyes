package notifications

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/onkar717/visual-eyes/server/models"
)

// WebhookNotifier sends alert events as JSON POST to a configurable HTTP endpoint.
// Optionally signs each request with HMAC-SHA256 (X-VisualEyes-Signature header)
// for receiver verification   compatible with the standard-webhooks spec.
type WebhookNotifier struct {
	url    string
	secret string
	client *http.Client
}

// NewWebhookNotifier creates a notifier that POSTs to the given URL.
// secret may be empty (no signing).
func NewWebhookNotifier(url, secret string) *WebhookNotifier {
	return &WebhookNotifier{
		url:    url,
		secret: secret,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

type webhookPayload struct {
	EventType string       `json:"event_type"` // alert.fired | alert.resolved
	Source    string       `json:"source"`
	Timestamp string       `json:"timestamp"`
	Alert     models.Alert `json:"alert"`
}

// AlertFired implements Notifier.
func (w *WebhookNotifier) AlertFired(alert models.Alert) error {
	return w.post(webhookPayload{
		EventType: "alert.fired",
		Source:    "visual-eyes",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Alert:     alert,
	})
}

// AlertResolved implements Notifier.
func (w *WebhookNotifier) AlertResolved(alert models.Alert) error {
	return w.post(webhookPayload{
		EventType: "alert.resolved",
		Source:    "visual-eyes",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Alert:     alert,
	})
}

func (w *WebhookNotifier) post(payload webhookPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "VisualEyes-Webhook/1.0")

	if w.secret != "" {
		sig := hmacSHA256(body, w.secret)
		req.Header.Set("X-VisualEyes-Signature", "sha256="+sig)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func hmacSHA256(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
