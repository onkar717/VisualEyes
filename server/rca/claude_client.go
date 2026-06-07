package rca

import (
	"context"
	"fmt"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/onkar717/visual-eyes/server/models"
)

// ServiceImpact mirrors models.ServiceImpact for JSON parsing from LLM output.
type ServiceImpact struct {
	Service      string   `json:"service"`
	Namespace    string   `json:"namespace"`
	ImpactLevel  string   `json:"impact_level"` // down|degraded|at_risk
	AffectedPods []string `json:"affected_pods,omitempty"`
	ErrorRatePct float64  `json:"error_rate_pct,omitempty"`
	P99LatencyMs float64  `json:"p99_latency_ms,omitempty"`
}

// EvidenceItem mirrors models.Evidence for JSON parsing from LLM output.
type EvidenceItem struct {
	Type        string  `json:"type"`    // metric|log|event|node|runbook
	Source      string  `json:"source"`
	Description string  `json:"description"`
	Raw         string  `json:"raw,omitempty"`
	MetricName  string  `json:"metric_name,omitempty"`
	MetricValue float64 `json:"metric_value,omitempty"`
	PodName     string  `json:"pod_name,omitempty"`
	Namespace   string  `json:"namespace,omitempty"`
}

// RemediationStepItem mirrors models.RemediationStep for JSON parsing from LLM output.
type RemediationStepItem struct {
	StepNumber    int    `json:"step_number"`
	Description   string `json:"description"`
	Command       string `json:"command,omitempty"`
	IsDestructive bool   `json:"is_destructive"`
	IsAutomated   bool   `json:"is_automated"`
}

// RCAResponse is the structured JSON any LLM returns from the final pipeline stage.
type RCAResponse struct {
	Explanation         string                `json:"explanation"`
	RootCause           string                `json:"root_cause"`
	Confidence          int                   `json:"confidence"`
	Severity            string                `json:"severity"` // SEV1|SEV2|SEV3|SEV4
	Category            string                `json:"category"` // crashloop|oom|high_cpu|high_memory|disk|network|other
	ContributingFactors []string              `json:"contributing_factors"`
	AffectedServices    []string              `json:"affected_services"`
	AffectedNamespaces  []string              `json:"affected_namespaces,omitempty"`
	AffectedPodCount    int                   `json:"affected_pod_count,omitempty"`
	ServiceImpacts      []ServiceImpact       `json:"service_impacts,omitempty"`
	Evidence            []EvidenceItem        `json:"evidence,omitempty"`
	RemediationPlan     []RemediationStepItem `json:"remediation_plan,omitempty"`
	Commands            []FixCommand          `json:"commands"`
	HasIssue            bool                  `json:"has_issue"`    // false = cluster healthy, skip remediation
	RunbookUsed         string                `json:"runbook_used"` // matched runbook filename
	RawOutput           string                `json:"-"`            // raw commander text for audit
}

// FixCommand is one proposed remediation action.
type FixCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	IsAutoSafe  bool   `json:"is_auto_safe"`
	Risk        string `json:"risk"`
}

// ClaudeClient wraps the Anthropic SDK and implements LLMProvider.
type ClaudeClient struct {
	client      *anthropic.Client
	model       string
	maxTokens   int
	temperature float64
}

// NewClaudeClient creates a ClaudeClient with the given API key, model, and temperature.
func NewClaudeClient(apiKey, model string, maxTokens int, temperature float64) *ClaudeClient {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	if temperature <= 0 {
		temperature = 0.1
	}
	return &ClaudeClient{client: &client, model: model, maxTokens: maxTokens, temperature: temperature}
}

// Name implements LLMProvider.
func (c *ClaudeClient) Name() string { return c.model }

// Complete implements LLMProvider — single-turn chat with system + user message.
func (c *ClaudeClient) Complete(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, int, error) {
	if maxTokens <= 0 {
		maxTokens = c.maxTokens
	}
	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: int64(maxTokens),
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
	})
	if err != nil {
		return "", 0, fmt.Errorf("claude api: %w", err)
	}
	if len(msg.Content) == 0 {
		return "", int(msg.Usage.InputTokens), fmt.Errorf("claude returned empty response")
	}
	for _, block := range msg.Content {
		if block.Type == "text" {
			return block.Text, int(msg.Usage.InputTokens), nil
		}
	}
	return "", int(msg.Usage.InputTokens), fmt.Errorf("claude returned no text block")
}

// stripFences removes accidental ```json / ``` wrappers from LLM output.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// ToFixCommands converts the response into models.FixCommand slices.
func (r *RCAResponse) ToFixCommands() []models.FixCommand {
	cmds := make([]models.FixCommand, len(r.Commands))
	for i, c := range r.Commands {
		cmds[i] = models.FixCommand{
			Command:    c.Command,
			IsAutoSafe: isSafe(c.Command),
			Status:     models.RemediationPending,
		}
	}
	return cmds
}
