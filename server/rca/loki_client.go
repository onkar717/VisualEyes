package rca

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/onkar717/visual-eyes/server/models"
)

// LokiClient queries a Loki instance for pod log lines.
// Used by ContextBuilder to supplement or replace stored push logs.
type LokiClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewLokiClient creates a client for the given Loki base URL.
func NewLokiClient(baseURL string) *LokiClient {
	return &LokiClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// lokiQueryResponse is the minimal shape of Loki's /loki/api/v1/query_range response.
type lokiQueryResponse struct {
	Data struct {
		Result []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"` // [[nanosecond-ts, line], ...]
		} `json:"result"`
	} `json:"data"`
}

// QueryLogs fetches log lines for a given pod from Loki, looking back `lookback`
// from now and returning at most `limit` lines. Returns nil (no error) if Loki
// returns no results or is unreachable — callers fall back to stored logs.
func (c *LokiClient) QueryLogs(pod, namespace string, lookback time.Duration, limit int) ([]models.PodLog, error) {
	if limit <= 0 {
		limit = 100
	}

	// LogQL stream selector: match pod and namespace labels (standard Kubernetes labels).
	logQL := fmt.Sprintf(`{pod="%s", namespace="%s"}`, pod, namespace)

	start := time.Now().Add(-lookback).UnixNano()
	end := time.Now().UnixNano()

	params := url.Values{}
	params.Set("query", logQL)
	params.Set("start", fmt.Sprintf("%d", start))
	params.Set("end", fmt.Sprintf("%d", end))
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("direction", "backward")

	reqURL := c.baseURL + "/loki/api/v1/query_range?" + params.Encode()
	resp, err := c.httpClient.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("loki query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("loki returned %d: %s", resp.StatusCode, string(body))
	}

	var lresp lokiQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&lresp); err != nil {
		return nil, fmt.Errorf("loki decode: %w", err)
	}

	var lines []models.PodLog
	for _, stream := range lresp.Data.Result {
		for _, val := range stream.Values {
			if len(val) < 2 {
				continue
			}
			var ts time.Time
			var nsec int64
			if _, err := fmt.Sscanf(val[0], "%d", &nsec); err == nil {
				ts = time.Unix(0, nsec)
			} else {
				ts = time.Now()
			}
			lines = append(lines, models.PodLog{
				Pod:       pod,
				Namespace: namespace,
				Stream:    stream.Stream["stream"], // stdout | stderr
				Line:      val[1],
				Timestamp: ts,
			})
		}
	}

	return lines, nil
}
