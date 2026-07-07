package router

import (
	"os"
	"testing"

	"simon-go/internal/config"
)

func clearProviderEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"OLLAMA_HOST"} {
		old, existed := os.LookupEnv(key)
		os.Unsetenv(key)
		t.Cleanup(func() {
			if existed {
				os.Setenv(key, old)
			}
		})
	}
}

func TestResolveExplicitProviderNoFallbackWhenUnavailable(t *testing.T) {
	clearProviderEnv(t)
	r := New(config.Settings{DefaultModel: "auto"})

	choice := r.Resolve(ResolveOptions{Model: "openai_model"})

	if choice.Provider != ProviderEcho {
		t.Errorf("expected explicit-but-unavailable provider to fall back to Echo, got %v", choice)
	}
}

func TestResolveExplicitProviderAvailable(t *testing.T) {
	clearProviderEnv(t)
	r := New(config.Settings{OpenAIAPIKey: "sk-test", OpenAIModel: "gpt-5-mini"})

	choice := r.Resolve(ResolveOptions{Model: "openai_model"})

	if choice.Provider != ProviderOpenAI || choice.Model != "gpt-5-mini" {
		t.Errorf("expected openai/gpt-5-mini, got %+v", choice)
	}
}

func TestResolveLocalFirstWhenSimple(t *testing.T) {
	clearProviderEnv(t)
	os.Setenv("OLLAMA_HOST", "http://localhost:11434")
	r := New(config.Settings{
		OpenAIAPIKey: "sk-test", OpenAIModel: "gpt-5",
		OllamaModel: "llama3.1",
	})

	choice := r.Resolve(ResolveOptions{Task: "summarize this email"})

	if choice.Provider != ProviderOllama {
		t.Errorf("expected local-first Ollama for a simple task, got %+v", choice)
	}
}

func TestResolveOnlineFirstWhenComplex(t *testing.T) {
	clearProviderEnv(t)
	os.Setenv("OLLAMA_HOST", "http://localhost:11434")
	r := New(config.Settings{
		OpenAIAPIKey: "sk-test", OpenAIModel: "gpt-5",
		OllamaModel: "llama3.1",
	})

	choice := r.Resolve(ResolveOptions{Task: "do a deep architectural analysis"})

	if choice.Provider != ProviderOpenAI {
		t.Errorf("expected online-first OpenAI for a complex task, got %+v", choice)
	}
}

func TestResolveFallsBackToEchoWhenNothingConfigured(t *testing.T) {
	clearProviderEnv(t)
	r := New(config.Settings{DefaultModel: "auto"})

	choice := r.Resolve(ResolveOptions{})

	if choice.Provider != ProviderEcho {
		t.Errorf("expected Echo fallback, got %+v", choice)
	}
}

func TestResolveSpanishComplexHint(t *testing.T) {
	clearProviderEnv(t)
	r := New(config.Settings{AnthropicAPIKey: "key", AnthropicModel: "claude-3-5-sonnet-latest"})

	choice := r.Resolve(ResolveOptions{Task: "necesito un razonamiento profundo sobre esto"})

	if choice.Provider != ProviderAnthropic {
		t.Errorf("expected anthropic for spanish complex-task hint, got %+v", choice)
	}
}
