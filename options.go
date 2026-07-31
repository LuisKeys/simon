package simon

import (
	"context"
	"log/slog"

	"github.com/LuisKeys/simon/internal/config"
	"github.com/LuisKeys/simon/knowledge"
	"github.com/LuisKeys/simon/memory"
	"github.com/LuisKeys/simon/model"
	"github.com/LuisKeys/simon/tool"
)

// Option configures a Runtime at construction time.
type Option func(*Runtime) error

// WithEnvironment loads settings from the process environment (and a
// ".env" file in the working directory, if present) — the same source
// cmd/simon uses. This is the default even with no options at all; passing
// it explicitly documents intent and lets it be combined with WithSettings
// (which takes precedence field-by-field).
func WithEnvironment() Option {
	return func(rt *Runtime) error {
		rt.settings = config.Load()
		return nil
	}
}

// WithSettings applies explicit settings on top of whatever base settings
// are already loaded (environment by default). Fields left at their zero
// value keep the base's value, so WithSettings can be used to override just
// one or two fields.
func WithSettings(settings Settings) Option {
	return func(rt *Runtime) error {
		rt.settings = settings.toInternal(rt.settings)
		return nil
	}
}

// WithModel pins every session created by this Runtime to a single custom
// Model implementation, bypassing router-based provider selection entirely.
func WithModel(m model.Model) Option {
	return func(rt *Runtime) error {
		rt.modelOverride = m
		return nil
	}
}

// ModelRouter selects a provider/model pair for a run. Implement this to
// customize provider selection instead of using Simon's built-in
// heuristics (env-configured providers + task-complexity keywords).
//
// Routing decisions from a custom ModelRouter are resolved once per
// Session (at NewSession), not per prompt — Simon's own default router
// already resolves per-prompt when no custom Model/ModelRouter is set, so
// this only affects the advanced case of a caller-supplied router.
type ModelRouter interface {
	Resolve(ctx context.Context, modelLabel, task string) (provider, modelName string)
}

// WithRouter replaces Simon's default provider-selection heuristics with a
// custom ModelRouter.
func WithRouter(router ModelRouter) Option {
	return func(rt *Runtime) error {
		rt.customRouter = router
		return nil
	}
}

// MemoryFactory builds a Memory for a given session, letting each Session
// obtain independent storage.
type MemoryFactory = memory.Factory

// WithMemoryFactory attaches a MemoryFactory so every Session gets its own
// Memory instance at construction time.
func WithMemoryFactory(factory MemoryFactory) Option {
	return func(rt *Runtime) error {
		rt.memoryFactory = factory
		return nil
	}
}

// WithKnowledgeBase attaches a knowledge base every Session searches by
// default (a Session can override it via WithSessionKnowledge).
func WithKnowledgeBase(kb knowledge.Searcher) Option {
	return func(rt *Runtime) error {
		rt.knowledgeSearcher = kb
		return nil
	}
}

// WithToolRegistry replaces the Runtime's tool registry outright, instead
// of registering tools one at a time via RegisterTool/RegisterTools.
func WithToolRegistry(registry *tool.Registry) Option {
	return func(rt *Runtime) error {
		if registry == nil {
			registry = tool.NewRegistry()
		}
		rt.tools = registry
		return nil
	}
}

// WithEventHandler attaches a handler invoked for every Event emitted by
// any Session this Runtime creates. Handler panics/errors are recovered
// and never affect the run that triggered them.
func WithEventHandler(handler EventHandler) Option {
	return func(rt *Runtime) error {
		rt.eventHandler = handler
		return nil
	}
}

// WithLogger attaches a structured logger. Without one, Runtime uses a
// silent (slog.DiscardHandler) logger — errors are still returned
// normally, nothing is printed.
func WithLogger(logger *slog.Logger) Option {
	return func(rt *Runtime) error {
		rt.logger = logger
		return nil
	}
}

// WithMaxConcurrentRuns caps how many Session runs may execute
// concurrently across the whole Runtime. Zero (the default) means
// unlimited.
func WithMaxConcurrentRuns(limit int) Option {
	return func(rt *Runtime) error {
		rt.maxConcurrent = limit
		return nil
	}
}

// WithApprovalPolicy attaches a policy every registered tool call is
// checked against before it executes. The default policy (AllowAll)
// permits every call.
func WithApprovalPolicy(policy ApprovalPolicy) Option {
	return func(rt *Runtime) error {
		rt.approval = policy
		return nil
	}
}

// SessionOption configures a Session at construction time.
type SessionOption func(*sessionConfig)

type sessionConfig struct {
	systemPrompt string
	maxSteps     int
	knowledge    knowledge.Searcher
}

// WithSystemPrompt sets the session's system prompt.
func WithSystemPrompt(prompt string) SessionOption {
	return func(c *sessionConfig) { c.systemPrompt = prompt }
}

// WithMaxSteps overrides the maximum number of ReAct tool-call steps for
// this session.
func WithMaxSteps(n int) SessionOption {
	return func(c *sessionConfig) { c.maxSteps = n }
}

// WithSessionKnowledge overrides the Runtime's default knowledge base for
// this session only.
func WithSessionKnowledge(kb knowledge.Searcher) SessionOption {
	return func(c *sessionConfig) { c.knowledge = kb }
}
