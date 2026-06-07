package rca

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/onkar717/visual-eyes/backend/models"
)

const systemPrompt = `You are an expert Site Reliability Engineer (SRE) assistant embedded in a Kubernetes monitoring platform called VisualEyes.

Your job is to analyse metric alerts and produce:
1. A concise explanation of what is happening (2-3 sentences, plain English, no jargon).
2. The likely root cause (1-2 sentences).
3. A prioritised list of remediation commands.

STRICT OUTPUT RULES:
- Output ONLY valid JSON, no markdown, no code fences, no explanation outside the JSON.
- Use this exact schema:
{
  "explanation": "string",
  "root_cause": "string",
  "confidence": 0-100,
  "commands": [
    {
      "command": "kubectl ...",
      "description": "one line what this does",
      "is_auto_safe": true|false,
      "risk": "low|medium|high"
    }
  ]
}

AUTO-SAFE COMMAND RULES:
- Mark is_auto_safe=true ONLY for these command prefixes:
    kubectl delete pod
    kubectl rollout restart
- Everything else must have is_auto_safe=false.
- Never suggest cluster-wide destructive commands (delete namespace, delete node, etc.).
- If no fix is needed or safe, return commands=[].

CONFIDENCE: set 0-100 based on how much signal you have. Low signal = low confidence.`

// RCAResponse is the structured JSON Claude returns.
type RCAResponse struct {
	Explanation string       `json:"explanation"`
	RootCause   string       `json:"root_cause"`
	Confidence  int          `json:"confidence"`
	Commands    []FixCommand `json:"commands"`
}

// FixCommand is one proposed remediation action.
type FixCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	IsAutoSafe  bool   `json:"is_auto_safe"`
	Risk        string `json:"risk"`
}

// ClaudeClient wraps the Anthropic SDK to produce RCA analyses.
type ClaudeClient struct {
	client    *anthropic.Client
	model     string
	maxTokens int
}

// NewClaudeClient creates a ClaudeClient with the given API key and model.
func NewClaudeClient(apiKey, model string, maxTokens int) *ClaudeClient {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return &ClaudeClient{client: &client, model: model, maxTokens: maxTokens}
}

// Analyze sends the alert context to Claude and returns a parsed RCAResponse.
func (c *ClaudeClient) Analyze(ctx context.Context, ac AlertContext) (*RCAResponse, int, error) {
	userMessage := fmt.Sprintf(
		"Analyse the following Kubernetes alert and provide root cause analysis.\n\n%s",
		ac.Format(),
	)

	slog.Debug("calling claude for rca",
		"model", c.model,
		"alert_id", ac.Alert.ID,
		"rule", ac.Alert.RuleName,
	)

	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: int64(c.maxTokens),
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userMessage)),
		},
	})
	if err != nil {
		return nil, 0, fmt.Errorf("claude api error: %w", err)
	}

	inputTokens := int(msg.Usage.InputTokens)

	if len(msg.Content) == 0 {
		return nil, inputTokens, fmt.Errorf("claude returned empty response")
	}

	raw := ""
	for _, block := range msg.Content {
		if block.Type == "text" {
			raw = block.Text
			break
		}
	}

	// Strip any accidental markdown fences.
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var resp RCAResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, inputTokens, fmt.Errorf("parse claude response: %w\nraw: %s", err, raw)
	}

	slog.Info("rca analysis complete",
		"alert_id", ac.Alert.ID,
		"rule", ac.Alert.RuleName,
		"confidence", resp.Confidence,
		"commands", len(resp.Commands),
		"input_tokens", inputTokens,
	)

	// Enforce the auto-safe allowlist regardless of what Claude said.
	for i := range resp.Commands {
		resp.Commands[i].IsAutoSafe = isSafe(resp.Commands[i].Command)
	}

	return &resp, inputTokens, nil
}

// ToFixCommands converts the response into models.FixCommand slices.
func (r *RCAResponse) ToFixCommands() []models.FixCommand {
	cmds := make([]models.FixCommand, len(r.Commands))
	for i, c := range r.Commands {
		cmds[i] = models.FixCommand{
			Command:    c.Command,
			IsAutoSafe: c.IsAutoSafe,
			Status:     models.RemediationPending,
		}
	}
	return cmds
}
