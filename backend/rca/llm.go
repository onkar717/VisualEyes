package rca

import "context"

// LLMProvider is the single interface all LLM backends must implement.
// Each stage in the pipeline calls Complete with a system + user prompt.
type LLMProvider interface {
	// Name returns a human-readable label (e.g. "claude-sonnet-4-6", "gpt-4o").
	Name() string
	// Complete sends a chat request and returns (response text, input tokens, error).
	Complete(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, int, error)
}
