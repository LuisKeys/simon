// Package semantic classifies raw sensor observations into activity
// labels, mirroring Python's simon/semantic/extractor.py
// SemanticEventExtractor.
//
// Deliberately bypasses internal/router: window titles and clipboard
// metadata are private activity data, and the router's complexity
// heuristic could pick a cloud provider if API keys happen to be
// configured for other parts of the app. This extractor talks directly to
// a model.Model that defaults to a local Ollama model, so classification
// never leaves the device regardless of what else is configured.
package semantic

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"simon-go/internal/events"
	"simon-go/internal/model"
	"simon-go/internal/model/ollama"
)

// DefaultCategories mirrors Python's DEFAULT_CATEGORIES.
var DefaultCategories = []string{
	"programming", "terminal", "reading_docs", "chat_messaging", "email",
	"web_browsing", "reading_news", "meeting_video", "watching_media",
	"file_management", "design", "other",
}

const systemPromptTemplate = "You classify what a computer user is doing right now, from the app name and " +
	`window title alone. Reply with ONLY a JSON object: {"category": "<one of: %s>", "label": "<short human-readable description>"}. ` +
	`No other text. If unsure, use category "other".`

// Extractor subscribes to raw sensor events and publishes
// SemanticActivityInferred events. Only text signals are used (app name,
// window title, clipboard metadata) — no screenshots.
type Extractor struct {
	bus        *events.EventBus
	model      model.Model
	categories []string

	mu      sync.Mutex
	context map[string]string
}

// Option configures an Extractor at construction time.
type Option func(*Extractor)

// WithModel overrides the default local-only Ollama model.
func WithModel(m model.Model) Option { return func(e *Extractor) { e.model = m } }

// WithCategories overrides DefaultCategories.
func WithCategories(categories []string) Option {
	return func(e *Extractor) { e.categories = categories }
}

// New builds an Extractor bound to bus. Without WithModel, it defaults to
// a local Ollama model (ollama.New("", "") — empty host resolves via
// OLLAMA_HOST/the client's environment default, empty model defaults to
// "llama3.1"), matching Python's `model or OllamaModel()`.
func New(bus *events.EventBus, opts ...Option) (*Extractor, error) {
	e := &Extractor{bus: bus, categories: DefaultCategories, context: map[string]string{}}
	for _, opt := range opts {
		opt(e)
	}
	if e.model == nil {
		m, err := ollama.New("", "")
		if err != nil {
			return nil, err
		}
		e.model = m
	}
	return e, nil
}

// Attach subscribes to the raw sensor event types this extractor consumes.
func (e *Extractor) Attach() {
	e.bus.Subscribe(events.WindowFocusChanged, e.onWindowEvent)
	e.bus.Subscribe(events.ClipboardChanged, e.onClipboardEvent)
}

func (e *Extractor) onWindowEvent(ctx context.Context, event events.ActivityEvent) error {
	e.mu.Lock()
	e.context["app_name"] = stringOr(event.Data, "app_name")
	e.context["window_title"] = stringOr(event.Data, "window_title")
	e.mu.Unlock()
	return e.classify(ctx)
}

func (e *Extractor) onClipboardEvent(ctx context.Context, event events.ActivityEvent) error {
	e.mu.Lock()
	_, hasAppName := e.context["app_name"]
	if !hasAppName {
		e.mu.Unlock()
		// Clipboard metadata alone rarely justifies a new classification —
		// it only refines the activity we already have window context for.
		return nil
	}
	e.context["clipboard_kind"] = stringOr(event.Data, "kind")
	e.mu.Unlock()
	return e.classify(ctx)
}

func (e *Extractor) classify(ctx context.Context) error {
	e.mu.Lock()
	snapshot := make(map[string]string, len(e.context))
	for k, v := range e.context {
		snapshot[k] = v
	}
	e.mu.Unlock()

	contextJSON, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}

	resp, err := e.model.Complete(ctx, []model.Message{
		{Role: model.RoleSystem, Content: systemPrompt(e.categories)},
		{Role: model.RoleUser, Content: string(contextJSON)},
	}, nil)
	if err != nil {
		slog.Error("SemanticEventExtractor classification failed", "err", err)
		return nil
	}

	parsed := e.parse(resp.Text)
	if parsed == nil {
		slog.Warn("SemanticEventExtractor got unparseable output", "text", resp.Text)
		return nil
	}

	data := make(map[string]any, len(snapshot)+2)
	for k, v := range snapshot {
		data[k] = v
	}
	data["category"] = parsed["category"]
	data["label"] = parsed["label"]

	return e.bus.Publish(ctx, events.New(events.SemanticActivityInferred, "SemanticEventExtractor", data))
}

func (e *Extractor) parse(text string) map[string]string {
	data := extractJSONObject(text)
	if data == nil {
		return nil
	}
	label, _ := data["label"].(string)
	category, _ := data["category"].(string)
	if label == "" || !contains(e.categories, category) {
		return nil
	}
	return map[string]string{"label": label, "category": category}
}

func systemPrompt(categories []string) string {
	return fmt.Sprintf(systemPromptTemplate, strings.Join(categories, ", "))
}

func stringOr(data map[string]any, key string) string {
	if v, ok := data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// extractJSONObject mirrors Python's _extract_json_object: try a direct
// JSON decode first, then fall back to slicing from the first '{' to the
// last '}' to tolerate prose wrapping around the object.
func extractJSONObject(text string) map[string]any {
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err == nil {
		return data
	}

	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start == -1 || end == -1 || end < start {
		return nil
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &data); err != nil {
		return nil
	}
	return data
}
