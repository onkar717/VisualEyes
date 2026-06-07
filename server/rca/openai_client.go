package rca

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// openAIBaseURLs maps short provider names to their OpenAI-compatible API base URLs.
var openAIBaseURLs = map[string]string{
	"openai":  "https://api.openai.com/v1",
	"groq":    "https://api.groq.com/openai/v1",
	"mistral": "https://api.mistral.ai/v1",
}

// OpenAIClient calls any OpenAI-compatible chat completions endpoint.
// Supports OpenAI, Groq (llama-3.3-70b-versatile etc.), and Mistral.
type OpenAIClient struct {
	baseURL     string
	apiKey      string
	model       string
	temperature float64
	http        *http.Client
}

// NewOpenAIClient creates a client for the given provider (openai|groq|mistral)
// or accepts an explicit baseURL (for self-hosted / proxy endpoints).
func NewOpenAIClient(provider, apiKey, model, baseURL string, temperature float64) *OpenAIClient {
	if baseURL == "" {
		if u, ok := openAIBaseURLs[strings.ToLower(provider)]; ok {
			baseURL = u
		} else {
			baseURL = openAIBaseURLs["openai"]
		}
	}
	if temperature <= 0 {
		temperature = 0.1
	}
	return &OpenAIClient{
		baseURL:     strings.TrimRight(baseURL, "/"),
		apiKey:      apiKey,
		model:       model,
		temperature: temperature,
		http:        &http.Client{Timeout: 120 * time.Second},
	}
}

// Name implements LLMProvider.
func (c *OpenAIClient) Name() string { return c.model }

// Complete implements LLMProvider.
func (c *OpenAIClient) Complete(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, int, error) {
	body := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"max_tokens":  maxTokens,
		"temperature": c.temperature,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", 0, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("openai HTTP %d: %s", resp.StatusCode, truncateStr(string(rawBody), 300))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rawBody, &out); err != nil {
		return "", 0, fmt.Errorf("parse openai response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", 0, fmt.Errorf("openai returned no choices")
	}
	return out.Choices[0].Message.Content, out.Usage.PromptTokens, nil
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
