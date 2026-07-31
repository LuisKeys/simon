package simon

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"simon-go/internal/agent"
	"simon-go/internal/agent/response"
	internaltool "simon-go/internal/tool"
	"simon-go/knowledge"
	"simon-go/memory"
	"simon-go/pkg/simonerr"
	"simon-go/tool"
)

// Session represents one independent conversation or task run against a
// Runtime's shared resources: its own history, its own active-run state,
// its own event stream. A Runtime may host many concurrent Sessions;
// within a single Session, only one Run/Stream/RunStructured may be active
// at a time — a second call while one is in flight returns
// simonerr.ErrSessionBusy.
type Session struct {
	id string
	rt *Runtime

	agent  *agent.Agent
	memory memory.Memory

	mu       sync.Mutex
	running  bool
	closed   bool
	cancel   context.CancelFunc
	runCtx   context.Context
	runID    string
	streamCh chan Event

	lastSteps    int
	lastProvider string
}

func newSession(rt *Runtime, id string, opts ...SessionOption) (*Session, error) {
	cfg := sessionConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	sess := &Session{id: id, rt: rt}

	agentOpts := []agent.Option{
		agent.WithName(id),
		agent.WithOnEvent(sess.handleAgentEvent),
	}
	if cfg.systemPrompt != "" {
		agentOpts = append(agentOpts, agent.WithSystemPrompt(cfg.systemPrompt))
	}
	if cfg.maxSteps > 0 {
		agentOpts = append(agentOpts, agent.WithMaxSteps(cfg.maxSteps))
	}

	if rt.memoryFactory != nil {
		mem, err := rt.memoryFactory.NewMemory(context.Background(), id)
		if err != nil {
			return nil, err
		}
		sess.memory = mem
		agentOpts = append(agentOpts, agent.WithMemory(memory.ToInternal(mem)))
	}

	searcher := rt.knowledgeSearcher
	if cfg.knowledge != nil {
		searcher = cfg.knowledge
	}
	if searcher != nil {
		agentOpts = append(agentOpts, agent.WithKnowledge(knowledge.ToAgentSearcher(searcher)))
	}

	if tools := rt.tools.List(); len(tools) > 0 {
		internal := make([]internaltool.Tool, 0, len(tools))
		for _, t := range tools {
			internal = append(internal, tool.ToInternal(t, sess.approve))
		}
		agentOpts = append(agentOpts, agent.WithTools(internal...))
	}

	modelOverride, err := rt.buildModelOverride(context.Background())
	if err != nil {
		return nil, err
	}
	if modelOverride != nil {
		agentOpts = append(agentOpts, agent.WithModelOverride(modelOverride))
	}

	sess.agent = agent.New(rt.settings, agentOpts...)
	return sess, nil
}

// ID returns the session identifier passed to Runtime.NewSession.
func (s *Session) ID() string { return s.id }

// approve checks the Runtime's ApprovalPolicy before a tool call executes.
func (s *Session) approve(ctx context.Context, name string, arguments json.RawMessage) error {
	policy := s.rt.approval
	if policy == nil {
		return nil
	}
	ok, err := policy.Approve(ctx, ApprovalRequest{SessionID: s.id, ToolName: name, Arguments: arguments})
	if err != nil {
		return err
	}
	if !ok {
		return simonerr.NewToolDeniedError(name)
	}
	return nil
}

// translateEventType maps an internal agent.Event.Type string onto a
// public EventType, or reports ok=false for internal-only events that
// aren't forwarded (currently "response_received", whose data is folded
// into the run.completed event Session synthesizes itself).
func translateEventType(t string) (EventType, bool) {
	switch t {
	case "model_selected":
		return EventModelSelected, true
	case "tool_requested":
		return EventToolRequested, true
	case "tool_started":
		return EventToolStarted, true
	case "tool_called":
		return EventToolCompleted, true
	case "retry_attempted":
		return EventRetryAttempted, true
	default:
		return "", false
	}
}

func (s *Session) handleAgentEvent(e agent.Event) {
	if e.Type == "response_received" {
		s.mu.Lock()
		if steps, ok := e.Data["steps"].(int); ok {
			s.lastSteps = steps
		}
		s.mu.Unlock()
		return
	}
	if e.Type == "model_selected" {
		s.mu.Lock()
		if p, ok := e.Data["provider"].(string); ok {
			s.lastProvider = p
		}
		s.mu.Unlock()
	}

	t, ok := translateEventType(e.Type)
	if !ok {
		return
	}

	s.mu.Lock()
	ctx := s.runCtx
	runID := s.runID
	s.mu.Unlock()
	if ctx == nil {
		return
	}
	s.emitEvent(ctx, Event{Type: t, SessionID: s.id, RunID: runID, Timestamp: time.Now(), Data: e.Data})
}

// emitEvent dispatches ev to the Runtime's EventHandler and, if a Stream is
// active, pushes it onto the stream channel.
func (s *Session) emitEvent(ctx context.Context, ev Event) {
	s.rt.dispatch(ctx, ev)
	s.mu.Lock()
	ch := s.streamCh
	s.mu.Unlock()
	if ch != nil {
		sendEvent(ch, ev)
	}
}

// sendEvent pushes ev onto ch without blocking. Non-terminal events are
// dropped if the buffer is full; terminal events (run.completed/failed/
// cancelled) always get through, evicting the oldest buffered event if
// necessary, so a slow/absent consumer can never lose the final outcome
// nor stall the run.
func sendEvent(ch chan Event, ev Event) {
	select {
	case ch <- ev:
		return
	default:
	}
	if !ev.Type.isTerminal() {
		return
	}
	for {
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- ev:
			return
		default:
		}
	}
}

// beginRun enforces the single-active-run-per-session rule and prepares
// per-run bookkeeping (cancellation, run ID). Callers must invoke the
// returned end func exactly once when the run finishes.
func (s *Session) beginRun(ctx context.Context) (context.Context, func(), error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, nil, simonerr.NewSessionClosedError()
	}
	if s.running {
		s.mu.Unlock()
		return nil, nil, simonerr.NewSessionBusyError()
	}
	s.running = true
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.runCtx = runCtx
	s.runID = newID("run")
	s.lastSteps = 0
	s.lastProvider = ""
	s.mu.Unlock()

	if s.rt.sem != nil {
		select {
		case s.rt.sem <- struct{}{}:
		case <-runCtx.Done():
			s.mu.Lock()
			s.running = false
			s.cancel = nil
			s.runCtx = nil
			s.runID = ""
			s.mu.Unlock()
			cancel()
			return nil, nil, runCtx.Err()
		}
	}

	end := func() {
		if s.rt.sem != nil {
			<-s.rt.sem
		}
		s.mu.Lock()
		s.running = false
		s.cancel = nil
		s.runCtx = nil
		s.runID = ""
		s.streamCh = nil
		s.mu.Unlock()
		cancel()
	}
	return runCtx, end, nil
}

// finish builds the public Response (or terminal error) from an agent run
// and emits the corresponding terminal event.
func (s *Session) finish(ctx context.Context, runID string, resp response.AgentResponse, err error) (Response, error) {
	s.mu.Lock()
	steps := s.lastSteps
	provider := s.lastProvider
	s.mu.Unlock()

	if err != nil {
		evType := EventRunFailed
		if errors.Is(err, context.Canceled) {
			evType = EventRunCancelled
		}
		s.emitEvent(ctx, Event{Type: evType, SessionID: s.id, RunID: runID, Timestamp: time.Now(), Data: map[string]any{"error": err.Error()}})
		return Response{}, err
	}

	out := fromAgentResponse(resp, s.agent.ModelName(), provider, steps)
	s.emitEvent(ctx, Event{Type: EventRunCompleted, SessionID: s.id, RunID: runID, Timestamp: time.Now(), Data: out})
	return out, nil
}

// Run executes prompt through the ReAct loop and returns once it
// completes, fails, or is cancelled.
func (s *Session) Run(ctx context.Context, prompt string) (Response, error) {
	runCtx, end, err := s.beginRun(ctx)
	if err != nil {
		return Response{}, err
	}
	defer end()

	s.mu.Lock()
	runID := s.runID
	s.mu.Unlock()
	s.emitEvent(runCtx, Event{Type: EventRunStarted, SessionID: s.id, RunID: runID, Timestamp: time.Now()})

	resp, runErr := s.agent.Run(runCtx, prompt)
	return s.finish(runCtx, runID, resp, runErr)
}

// Stream executes prompt like Run, but returns immediately with a
// read-only channel of Events instead of waiting for the final Response.
// The channel closes once the run completes, fails, or is cancelled; the
// final event is always delivered even if earlier events were dropped for
// buffer space.
func (s *Session) Stream(ctx context.Context, prompt string) (<-chan Event, error) {
	runCtx, end, err := s.beginRun(ctx)
	if err != nil {
		return nil, err
	}

	ch := make(chan Event, 64)
	s.mu.Lock()
	s.streamCh = ch
	runID := s.runID
	s.mu.Unlock()

	s.emitEvent(runCtx, Event{Type: EventRunStarted, SessionID: s.id, RunID: runID, Timestamp: time.Now()})

	go func() {
		defer close(ch)
		defer end()
		resp, runErr := s.agent.Run(runCtx, prompt)
		s.finish(runCtx, runID, resp, runErr)
	}()

	return ch, nil
}

// Cancel cancels this session's active run, if any. It is a no-op if no
// run is active.
func (s *Session) Cancel() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Clear erases this session's memory, if it has any.
func (s *Session) Clear(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return simonerr.NewSessionClosedError()
	}
	mem := s.memory
	s.mu.Unlock()
	if mem != nil {
		return mem.Clear(ctx)
	}
	return nil
}

// Close cancels any active run, closes this session's exclusive memory (if
// any), and marks it closed. Idempotent.
func (s *Session) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	cancel := s.cancel
	mem := s.memory
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if mem != nil {
		return mem.Close()
	}
	return nil
}
