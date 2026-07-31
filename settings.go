package simon

import "github.com/LuisKeys/simon/internal/config"

// Settings is the subset of Simon's environment-backed configuration
// relevant to an embedding application. It is a hand-maintained parallel of
// internal/config.Settings (not a type alias): CLI-only fields (activity
// store path, sensor poll interval, directory-enable flags) stay internal,
// and any internal renumbering/renaming can't silently change this public
// contract. Zero-value fields fall back to config.Load()'s defaults.
type Settings struct {
	DefaultModel string

	OpenAIAPIKey    string
	OpenAIModel     string
	AnthropicAPIKey string
	AnthropicModel  string
	OllamaHost      string
	OllamaModel     string

	KnowledgeStorePath string
	EmbeddingProvider  string
	EmbeddingModel     string

	MaxRetries        int
	RequestTimeout    float64
	RetryBaseDelay    float64
	StructuredRetries int
}

// toInternal maps Settings onto config.Settings, filling any zero-value
// field from the environment/.env defaults in base.
func (s Settings) toInternal(base config.Settings) config.Settings {
	out := base
	if s.DefaultModel != "" {
		out.DefaultModel = s.DefaultModel
	}
	if s.OpenAIAPIKey != "" {
		out.OpenAIAPIKey = s.OpenAIAPIKey
	}
	if s.OpenAIModel != "" {
		out.OpenAIModel = s.OpenAIModel
	}
	if s.AnthropicAPIKey != "" {
		out.AnthropicAPIKey = s.AnthropicAPIKey
	}
	if s.AnthropicModel != "" {
		out.AnthropicModel = s.AnthropicModel
	}
	if s.OllamaHost != "" {
		out.OllamaHost = s.OllamaHost
	}
	if s.OllamaModel != "" {
		out.OllamaModel = s.OllamaModel
	}
	if s.KnowledgeStorePath != "" {
		out.KnowledgeStorePath = s.KnowledgeStorePath
	}
	if s.EmbeddingProvider != "" {
		out.EmbeddingProvider = s.EmbeddingProvider
	}
	if s.EmbeddingModel != "" {
		out.EmbeddingModel = s.EmbeddingModel
	}
	if s.MaxRetries != 0 {
		out.SimonMaxRetries = s.MaxRetries
	}
	if s.RequestTimeout != 0 {
		out.SimonRequestTimeout = s.RequestTimeout
	}
	if s.RetryBaseDelay != 0 {
		out.SimonRetryBaseDelay = s.RetryBaseDelay
	}
	if s.StructuredRetries != 0 {
		out.SimonStructuredRetries = s.StructuredRetries
	}
	return out
}
