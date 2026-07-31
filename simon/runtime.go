// Package simon is the public facade for embedding Simon in another Go
// application: a Runtime holds shared resources (settings, provider
// selection, tool registry, memory/knowledge attachments, event dispatch),
// and each Session is one independent conversation or task run against it.
//
// Public types here (Response, Event, Usage, ...) intentionally do not
// alias internal/agent/response's types even where the shapes currently
// match: internal packages are free to change shape without that silently
// changing this package's contract. See internal/agent's package doc for
// why the agent loop itself doesn't offer an async variant — Runtime and
// Session follow the same rule (callers wanting concurrency use goroutines,
// not a parallel API).
package simon

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"simon-go/internal/config"
	"simon-go/internal/router"
	"simon-go/knowledge"
	"simon-go/memory"
	"simon-go/model"
	"simon-go/pkg/simonerr"
	"simon-go/tool"
)

var idCounter atomic.Uint64

// newID returns a process-unique, monotonically increasing identifier
// prefixed with kind (e.g. "rt-1", "run-2"). Not globally unique across
// process restarts, which is fine: these IDs only need to disambiguate
// concurrent runtimes/sessions/runs within one process.
func newID(kind string) string {
	n := idCounter.Add(1)
	return kind + "-" + itoa(n)
}

func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// Runtime holds resources shared across every Session it creates: settings,
// provider/router selection, the tool registry, the approval policy, event
// dispatch, and lifecycle. Safe for concurrent use.
type Runtime struct {
	mu sync.Mutex

	id       string
	settings config.Settings
	router   *router.Router

	customRouter  ModelRouter
	modelOverride model.Model

	tools    *tool.Registry
	approval ApprovalPolicy

	eventHandler      EventHandler
	logger            *slog.Logger
	memoryFactory     memory.Factory
	knowledgeSearcher knowledge.Searcher

	maxConcurrent int
	sem           chan struct{}

	sessions map[string]*Session
	closed   bool
}

// New builds a Runtime. With no options, settings are loaded from the
// environment (equivalent to WithEnvironment()).
func New(opts ...Option) (*Runtime, error) {
	rt := &Runtime{
		id:       newID("rt"),
		settings: config.Load(),
		tools:    tool.NewRegistry(),
		approval: AllowAll{},
		sessions: make(map[string]*Session),
	}
	for _, opt := range opts {
		if err := opt(rt); err != nil {
			return nil, err
		}
	}
	if rt.logger == nil {
		rt.logger = slog.New(slog.DiscardHandler)
	}
	rt.router = router.New(rt.settings)
	if rt.maxConcurrent > 0 {
		rt.sem = make(chan struct{}, rt.maxConcurrent)
	}
	return rt, nil
}

// RegisterTool adds a single tool to the Runtime's shared registry. Tools
// registered here are available to every Session created afterward;
// Sessions already created keep the tool set they were built with.
func (rt *Runtime) RegisterTool(t tool.Tool) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.closed {
		return simonerr.NewRuntimeClosedError()
	}
	rt.tools.Add(t)
	return nil
}

// RegisterTools registers multiple tools; see RegisterTool.
func (rt *Runtime) RegisterTools(tools ...tool.Tool) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.closed {
		return simonerr.NewRuntimeClosedError()
	}
	for _, t := range tools {
		rt.tools.Add(t)
	}
	return nil
}

// NewSession creates an independent Session bound to this Runtime's shared
// resources. Multiple Sessions may run concurrently.
func (rt *Runtime) NewSession(id string, opts ...SessionOption) (*Session, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.closed {
		return nil, simonerr.NewRuntimeClosedError()
	}
	sess, err := newSession(rt, id, opts...)
	if err != nil {
		return nil, err
	}
	rt.sessions[id] = sess
	return sess, nil
}

// Close cancels every active run and closes every Session this Runtime
// created. Idempotent: calling it more than once is a no-op after the
// first call.
func (rt *Runtime) Close() error {
	rt.mu.Lock()
	if rt.closed {
		rt.mu.Unlock()
		return nil
	}
	rt.closed = true
	sessions := make([]*Session, 0, len(rt.sessions))
	for _, s := range rt.sessions {
		sessions = append(sessions, s)
	}
	rt.sessions = nil
	rt.mu.Unlock()

	for _, s := range sessions {
		_ = s.Close()
	}
	return nil
}

// dispatch delivers ev (with RuntimeID stamped) to the configured
// EventHandler, if any, isolating the caller from handler panics.
func (rt *Runtime) dispatch(ctx context.Context, ev Event) {
	ev.RuntimeID = rt.id
	safeInvoke(rt.eventHandler, ctx, ev)
}
