package rca

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/onkar717/visual-eyes/server/models"
	"github.com/onkar717/visual-eyes/server/storage"
)

// PythonClient calls the ai-sre FastAPI service for CrewAI-powered RCA.
// Falls back silently to the Go pipeline when the service is unavailable.
type PythonClient struct {
	baseURL    string
	httpClient *http.Client
	callbackURL string // Go /internal/rca/stage-event URL for SSE callbacks
}

// NewPythonClient returns a client pointed at the ai-sre service.
// baseURL defaults to AI_SRE_URL env var or http://localhost:8001.
func NewPythonClient(callbackURL string) *PythonClient {
	base := os.Getenv("AI_SRE_URL")
	if base == "" {
		base = "http://localhost:8001"
	}
	return &PythonClient{
		baseURL:     base,
		callbackURL: callbackURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute, // CrewAI can take several minutes
		},
	}
}

// IsAvailable pings /health and returns true if the service is up.
func (c *PythonClient) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// pythonRCARequest mirrors the Pydantic RCARequest in main.py.
type pythonRCARequest struct {
	AlertID       uint               `json:"alert_id"`
	Alert         pythonAlertInfo    `json:"alert"`
	Context       pythonAlertContext `json:"context"`
	GoCallbackURL string             `json:"go_callback_url"`
	DryRun        bool               `json:"dry_run"`
}

type pythonAlertInfo struct {
	RuleName   string  `json:"rule_name"`
	Severity   string  `json:"severity"`
	Message    string  `json:"message"`
	ResourceID string  `json:"resource_id"`
	Namespace  string  `json:"namespace"`
	Value      float64 `json:"value"`
	Threshold  float64 `json:"threshold"`
}

type pythonAlertContext struct {
	RecentMetrics   []map[string]any `json:"recent_metrics"`
	RelatedMetrics  []map[string]any `json:"related_metrics"`
	PodLogs         []map[string]any `json:"pod_logs"`
	PrevLogs        []map[string]any `json:"prev_logs"`
	K8sEvents       []map[string]any `json:"k8s_events"`
	LogClassification map[string]any `json:"log_classification"`
	Anomalies       []map[string]any `json:"anomalies"`
}

// pythonRCAResponse mirrors the Pydantic RCAResponse in main.py.
type pythonRCAResponse struct {
	AlertID            uint              `json:"alert_id"`
	HasIssue           bool              `json:"has_issue"`
	Severity           string            `json:"severity"`
	Category           string            `json:"category"`
	Title              string            `json:"title"`
	RootCause          string            `json:"root_cause"`
	Explanation        string            `json:"explanation"`
	ContributingFactors []string         `json:"contributing_factors"`
	Confidence         int               `json:"confidence"`
	Commands           []pythonCommand   `json:"commands"`
	AffectedNamespaces []string          `json:"affected_namespaces"`
	AffectedServices   []pythonService   `json:"affected_services"`
	RunbookUsed        *string           `json:"runbook_used"`
	ScanDurationSecs   float64           `json:"scan_duration_seconds"`
	LLMModel           string            `json:"llm_model"`
	Error              *string           `json:"error"`
}

type pythonCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	IsAutoSafe  bool   `json:"is_auto_safe"`
	Risk        string `json:"risk"`
	Step        int    `json:"step"`
}

type pythonService struct {
	ServiceName string `json:"service_name"`
	Namespace   string `json:"namespace"`
	ImpactLevel string `json:"impact_level"`
}

// RunPipeline calls the Python ai-sre service and converts the response to RCAResponse.
func (c *PythonClient) RunPipeline(ctx context.Context, ac AlertContext) (*RCAResponse, int, error) {
	req := c.buildRequest(ac)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, 0, fmt.Errorf("python_client: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/run-rca", bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("python_client: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	slog.Info("python_client: calling ai-sre service",
		"alert_id", ac.Alert.ID,
		"url", c.baseURL+"/run-rca",
	)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("python_client: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("python_client: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("python_client: service returned %d: %s",
			resp.StatusCode, string(respBody)[:500])
	}

	var pyResp pythonRCAResponse
	if err := json.Unmarshal(respBody, &pyResp); err != nil {
		return nil, 0, fmt.Errorf("python_client: parse response: %w", err)
	}

	return c.convertResponse(&pyResp), 0, nil
}

func (c *PythonClient) buildRequest(ac AlertContext) pythonRCARequest {
	// Serialise pre-built context into maps so Python agents can use it as hints.
	recentMetrics := metricsToMaps(ac.RecentMetrics)
	relatedMetrics := metricsToMaps(ac.RelatedMetrics)
	podLogs := podLogsToMaps(ac.PodLogs)
	prevLogs := podLogsToMaps(ac.PrevLogs)
	k8sEvents := eventsToMaps(ac.K8sEvents)
	anomalies := anomaliesToMaps(ac.Anomalies)

	logClass := map[string]any{}
	if b, err := json.Marshal(ac.LogClassification); err == nil {
		_ = json.Unmarshal(b, &logClass)
	}

	return pythonRCARequest{
		AlertID: ac.Alert.ID,
		Alert: pythonAlertInfo{
			RuleName:   ac.Alert.RuleName,
			Severity:   string(ac.Alert.Severity),
			Message:    ac.Alert.Message,
			ResourceID: ac.Alert.ResourceID,
			Namespace:  ac.Alert.Namespace,
			Value:      ac.Alert.Value,
			Threshold:  ac.Alert.Threshold,
		},
		Context: pythonAlertContext{
			RecentMetrics:     recentMetrics,
			RelatedMetrics:    relatedMetrics,
			PodLogs:           podLogs,
			PrevLogs:          prevLogs,
			K8sEvents:         k8sEvents,
			LogClassification: logClass,
			Anomalies:         anomalies,
		},
		GoCallbackURL: c.callbackURL,
		DryRun:        false,
	}
}

func (c *PythonClient) convertResponse(py *pythonRCAResponse) *RCAResponse {
	cmds := make([]FixCommand, 0, len(py.Commands))
	for _, cmd := range py.Commands {
		safe := cmd.IsAutoSafe && isSafe(cmd.Command)
		cmds = append(cmds, FixCommand{
			Command:     cmd.Command,
			Description: cmd.Description,
			IsAutoSafe:  safe,
			Risk:        cmd.Risk,
		})
	}

	services := make([]ServiceImpact, 0, len(py.AffectedServices))
	for _, svc := range py.AffectedServices {
		services = append(services, ServiceImpact{
			Service:     svc.ServiceName,
			Namespace:   svc.Namespace,
			ImpactLevel: svc.ImpactLevel,
		})
	}

	affectedSvcNames := make([]string, 0, len(py.AffectedServices))
	for _, svc := range py.AffectedServices {
		affectedSvcNames = append(affectedSvcNames, svc.ServiceName)
	}

	rb := ""
	if py.RunbookUsed != nil {
		rb = *py.RunbookUsed
	}

	return &RCAResponse{
		HasIssue:            py.HasIssue,
		Severity:            py.Severity,
		Category:            py.Category,
		RootCause:           py.RootCause,
		Explanation:         py.Explanation,
		ContributingFactors: py.ContributingFactors,
		AffectedServices:    affectedSvcNames,
		Confidence:          py.Confidence,
		Commands:            cmds,
		AffectedNamespaces:  py.AffectedNamespaces,
		ServiceImpacts:      services,
		RunbookUsed:         rb,
		RawOutput:           fmt.Sprintf("ai-sre/%s duration=%.1fs", py.LLMModel, py.ScanDurationSecs),
	}
}

// ── Context serialisation helpers ─────────────────────────────────────────────

func metricsToMaps(metrics []models.Metric) []map[string]any {
	out := make([]map[string]any, 0, len(metrics))
	for _, m := range metrics {
		if b, err := json.Marshal(m); err == nil {
			var mv map[string]any
			if json.Unmarshal(b, &mv) == nil {
				out = append(out, mv)
			}
		}
	}
	return out
}

func podLogsToMaps(logs []models.PodLog) []map[string]any {
	out := make([]map[string]any, 0, len(logs))
	for _, l := range logs {
		if b, err := json.Marshal(l); err == nil {
			var mv map[string]any
			if json.Unmarshal(b, &mv) == nil {
				out = append(out, mv)
			}
		}
	}
	return out
}

func eventsToMaps(events []storage.K8sEvent) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, e := range events {
		if b, err := json.Marshal(e); err == nil {
			var mv map[string]any
			if json.Unmarshal(b, &mv) == nil {
				out = append(out, mv)
			}
		}
	}
	return out
}

func anomaliesToMaps(anomalies []AnomalyResult) []map[string]any {
	out := make([]map[string]any, 0, len(anomalies))
	for _, a := range anomalies {
		if b, err := json.Marshal(a); err == nil {
			var mv map[string]any
			if json.Unmarshal(b, &mv) == nil {
				out = append(out, mv)
			}
		}
	}
	return out
}
