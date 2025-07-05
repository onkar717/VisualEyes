package client

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/onkar717/visual-eyes/internal/agent/metrics/kubernetes/types"
)

const (
	insecureKubeletEndpoint = "http://127.0.0.1:10255/stats/summary"
	secureKubeletEndpoint   = "https://127.0.0.1:10250/stats/summary"
	tokenFile               = "/var/run/secrets/kubernetes.io/serviceaccount/token"
)

// KubeletClient handles communication with the Kubelet API
type KubeletClient struct {
	client   *http.Client
	endpoint string
	token    string
	isSecure bool
}

// IsInCluster detects if we're running inside a Kubernetes cluster
func IsInCluster() bool {
	_, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token")
	return err == nil
}

// NewClient creates a new KubeletClient
func NewClient() (*KubeletClient, error) {
	isSecure := true
	endpoint := secureKubeletEndpoint

	// Try to read service account token
	token, err := os.ReadFile(tokenFile)
	if err != nil {
		isSecure = false
		endpoint = insecureKubeletEndpoint
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Required since Kubelet typically uses self-signed certs
			},
		},
	}

	return &KubeletClient{
		client:   client,
		endpoint: endpoint,
		token:    string(token),
		isSecure: isSecure,
	}, nil
}

// GetSummary fetches stats from the Kubelet Summary API
func (k *KubeletClient) GetSummary() (*types.Stats, error) {
	req, err := http.NewRequest("GET", k.endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if k.isSecure {
		req.Header.Set("Authorization", "Bearer "+k.token)
	}

	resp, err := k.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch stats: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kubelet API returned %d: %s", resp.StatusCode, string(body))
	}

	var stats types.Stats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &stats, nil
}
