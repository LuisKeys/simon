// Package router implements lightweight model/provider selection with
// sensible defaults, mirroring Python's simon/router/router.py ModelRouter.
//
// Resolve returns a Choice (provider + model name) rather than a live
// model.Model instance: internal/model (Phase 1) depends on this package to
// avoid a router->model->router import cycle, and turns a Choice into an
// actual provider client once the model package exists.
package router

import (
	"os"
	"strings"

	"simon-go/internal/config"
)

// Provider identifies which backend a Choice should be built from.
type Provider string

const (
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderOllama    Provider = "ollama"
	ProviderEcho      Provider = "echo"
)

// Choice is the resolved provider + model name pair.
type Choice struct {
	Provider Provider
	Model    string
}

var labelMap = map[string]string{
	"auto":            "auto",
	"openai_model":    "openai",
	"anthropic_model": "anthropic",
	"ollama_model":    "ollama",
}

var complexTaskHints = []string{
	"complex", "difficult", "hard", "multi-step", "reasoning", "analyze", "analysis",
	"arquitectura", "arquitectónico", "complej", "razonamiento", "profundo",
}

// Router selects a provider/model pair given the current settings.
type Router struct {
	Settings config.Settings
}

// New builds a Router bound to the given settings.
func New(settings config.Settings) *Router {
	return &Router{Settings: settings}
}

func (r *Router) hasOpenAI() bool {
	return strings.TrimSpace(r.Settings.OpenAIAPIKey) != "" && strings.TrimSpace(r.Settings.OpenAIModel) != ""
}

func (r *Router) hasAnthropic() bool {
	return strings.TrimSpace(r.Settings.AnthropicAPIKey) != "" && strings.TrimSpace(r.Settings.AnthropicModel) != ""
}

// hasOllama mirrors Python's check of the raw OLLAMA_HOST env var (not the
// defaulted settings field) so Ollama only counts as "configured" when the
// user explicitly set a host, not merely because of the built-in default.
func (r *Router) hasOllama() bool {
	host := os.Getenv("OLLAMA_HOST")
	return strings.TrimSpace(host) != "" && strings.TrimSpace(r.Settings.OllamaModel) != ""
}

func isComplexTask(task string) bool {
	if task == "" {
		return false
	}
	text := strings.ToLower(task)
	for _, hint := range complexTaskHints {
		if strings.Contains(text, hint) {
			return true
		}
	}
	return false
}

// ResolveOptions mirrors resolve()'s optional (model, task, complex_task)
// parameters. A nil/empty Model means "use the configured default"; a nil
// ComplexTask means "infer from Task's keywords".
type ResolveOptions struct {
	Model       string
	Task        string
	ComplexTask *bool
}

// Resolve picks a provider/model Choice using the same priority rules as
// Python's ModelRouter.resolve: an explicit provider label wins outright (no
// fallback if unavailable); otherwise task complexity picks between
// online-first (complex) and local-first (simple) provider ordering; Echo is
// the ultimate fallback.
func (r *Router) Resolve(opts ResolveOptions) Choice {
	defaultLower := strings.ToLower(orDefault(r.Settings.DefaultModel, "auto"))
	defaultSelected := labelMapGet(defaultLower)

	rawSelected := opts.Model
	if rawSelected == "" {
		rawSelected = defaultSelected
	}
	rawSelected = strings.ToLower(rawSelected)
	selected := labelMapGet(rawSelected)

	isComplex := isComplexTask(opts.Task)
	if opts.ComplexTask != nil {
		isComplex = *opts.ComplexTask
	}

	if selected == "openai" && r.hasOpenAI() {
		return Choice{ProviderOpenAI, orDefault(r.Settings.OpenAIModel, "gpt-5")}
	}
	if selected == "anthropic" && r.hasAnthropic() {
		return Choice{ProviderAnthropic, orDefault(r.Settings.AnthropicModel, "claude-3-5-sonnet-latest")}
	}
	if selected == "ollama" && r.hasOllama() {
		return Choice{ProviderOllama, orDefault(r.Settings.OllamaModel, "llama3.1")}
	}

	// An explicit provider label was requested but is unavailable: no fallback.
	if selected == "openai" || selected == "anthropic" || selected == "ollama" {
		return Choice{ProviderEcho, ""}
	}

	if isComplex {
		if r.hasOpenAI() {
			return Choice{ProviderOpenAI, orDefault(r.Settings.OpenAIModel, "gpt-5")}
		}
		if r.hasAnthropic() {
			return Choice{ProviderAnthropic, orDefault(r.Settings.AnthropicModel, "claude-3-5-sonnet-latest")}
		}
		if r.hasOllama() {
			return Choice{ProviderOllama, orDefault(r.Settings.OllamaModel, "llama3.1")}
		}
	} else {
		if r.hasOllama() {
			return Choice{ProviderOllama, orDefault(r.Settings.OllamaModel, "llama3.1")}
		}
		if r.hasOpenAI() {
			return Choice{ProviderOpenAI, orDefault(r.Settings.OpenAIModel, "gpt-5")}
		}
		if r.hasAnthropic() {
			return Choice{ProviderAnthropic, orDefault(r.Settings.AnthropicModel, "claude-3-5-sonnet-latest")}
		}
	}

	return Choice{ProviderEcho, ""}
}

func labelMapGet(key string) string {
	if v, ok := labelMap[key]; ok {
		return v
	}
	return key
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
