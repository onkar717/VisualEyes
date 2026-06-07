package http

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/onkar717/visual-eyes/server/models"
)

// Common timeout constants
const (
	DefaultHTTPTimeout        = 10 * time.Second
	DefaultCollectionInterval = 15 * time.Second
	ContentTypeJSON           = "application/json"
)

// ClientConfig configures HTTP client behavior
type ClientConfig struct {
	Timeout            time.Duration
	InsecureSkipVerify bool
}

// NewClient creates a configured HTTP client
func NewClient(config ClientConfig) *http.Client {
	client := &http.Client{
		Timeout: config.Timeout,
	}

	if config.InsecureSkipVerify {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	return client
}

// NewDefaultClient creates an HTTP client with default timeout
func NewDefaultClient() *http.Client {
	return NewClient(ClientConfig{
		Timeout: DefaultHTTPTimeout,
	})
}

// SendMetrics sends metrics to the specified endpoint
func SendMetrics(client *http.Client, endpoint string, metrics []models.Metric) error {
	data, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("error marshaling metrics: %v", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", ContentTypeJSON)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
