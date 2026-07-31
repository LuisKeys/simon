package agent

import (
	"simon-go/internal/config"
	"simon-go/internal/model"
	"simon-go/internal/model/anthropic"
	"simon-go/internal/model/ollama"
	"simon-go/internal/model/openai"
	"simon-go/internal/router"
)

// BuildProviderModel constructs the live model.Model client for choice,
// mirroring the provider constructors Python calls inline in
// ModelRouter.resolve call sites. Exported so callers outside this package
// (notably the public simon facade) can resolve a provider from a
// router.Choice without duplicating this switch.
func BuildProviderModel(settings config.Settings, choice router.Choice) (model.Model, error) {
	switch choice.Provider {
	case router.ProviderOpenAI:
		return openai.New(settings.OpenAIAPIKey, choice.Model), nil
	case router.ProviderAnthropic:
		return anthropic.New(settings.AnthropicAPIKey, choice.Model), nil
	case router.ProviderOllama:
		return ollama.New(settings.OllamaHost, choice.Model)
	default:
		return model.EchoModel{}, nil
	}
}
