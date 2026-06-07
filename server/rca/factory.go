package rca

import (
	"log/slog"
	"strings"

	"github.com/onkar717/visual-eyes/backend/config"
)

// BuildLLMProvider selects and constructs the appropriate LLM backend from config.
// Returns nil when RCA is disabled or no API key is configured.
func BuildLLMProvider(cfg *config.Config) LLMProvider {
	if !cfg.RCA.Enabled {
		return nil
	}

	provider := strings.ToLower(cfg.RCA.Provider)

	switch provider {
	case "claude", "":
		if cfg.RCA.APIKey == "" {
			slog.Warn("rca.provider=claude but ANTHROPIC_API_KEY not set — rca disabled")
			return nil
		}
		slog.Info("rca provider: claude", "model", cfg.RCA.Model)
		return NewClaudeClient(cfg.RCA.APIKey, cfg.RCA.Model, cfg.RCA.MaxTokens, cfg.RCA.Temperature)

	case "openai":
		if cfg.RCA.OpenAIAPIKey == "" {
			slog.Warn("rca.provider=openai but OPENAI_API_KEY not set — rca disabled")
			return nil
		}
		slog.Info("rca provider: openai", "model", cfg.RCA.Model)
		return NewOpenAIClient("openai", cfg.RCA.OpenAIAPIKey, cfg.RCA.Model, cfg.RCA.BaseURL, cfg.RCA.Temperature)

	case "groq":
		if cfg.RCA.GroqAPIKey == "" {
			slog.Warn("rca.provider=groq but GROQ_API_KEY not set — rca disabled")
			return nil
		}
		model := cfg.RCA.Model
		if model == "" || model == "claude-sonnet-4-6" {
			model = "llama-3.3-70b-versatile"
		}
		slog.Info("rca provider: groq", "model", model)
		return NewOpenAIClient("groq", cfg.RCA.GroqAPIKey, model, cfg.RCA.BaseURL, cfg.RCA.Temperature)

	case "mistral":
		if cfg.RCA.MistralAPIKey == "" {
			slog.Warn("rca.provider=mistral but MISTRAL_API_KEY not set — rca disabled")
			return nil
		}
		model := cfg.RCA.Model
		if model == "" || model == "claude-sonnet-4-6" {
			model = "mistral-large-latest"
		}
		slog.Info("rca provider: mistral", "model", model)
		return NewOpenAIClient("mistral", cfg.RCA.MistralAPIKey, model, cfg.RCA.BaseURL, cfg.RCA.Temperature)

	default:
		// Custom base URL (e.g. local Ollama, Azure OpenAI, LM Studio)
		apiKey := cfg.RCA.APIKey
		if apiKey == "" {
			apiKey = cfg.RCA.OpenAIAPIKey
		}
		if apiKey == "" || cfg.RCA.BaseURL == "" {
			slog.Warn("rca.provider unknown and no base_url/api_key set — rca disabled", "provider", provider)
			return nil
		}
		slog.Info("rca provider: custom openai-compatible", "base_url", cfg.RCA.BaseURL, "model", cfg.RCA.Model)
		return NewOpenAIClient("custom", apiKey, cfg.RCA.Model, cfg.RCA.BaseURL, cfg.RCA.Temperature)
	}
}
