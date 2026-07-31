package simon

import (
	"context"
	"errors"
	"time"

	"simon-go/internal/agent"
)

// RunStructured runs prompt like Session.Run, but parses the model's reply
// into T (via a JSON-schema instruction and retries on invalid JSON,
// matching internal/agent.RunStructured's behavior). On exhaustion, the
// returned error is a *simonerr.StructuredOutputError (recoverable via
// errors.As), carrying the raw text and attempt count.
func RunStructured[T any](ctx context.Context, s *Session, prompt string) (T, error) {
	var zero T

	runCtx, end, err := s.beginRun(ctx)
	if err != nil {
		return zero, err
	}
	defer end()

	s.mu.Lock()
	runID := s.runID
	s.mu.Unlock()
	s.emitEvent(runCtx, Event{Type: EventRunStarted, SessionID: s.id, RunID: runID, Timestamp: time.Now()})

	result, resp, runErr := agent.RunStructured[T](runCtx, s.agent, prompt)
	if runErr != nil {
		evType := EventRunFailed
		if errors.Is(runErr, context.Canceled) {
			evType = EventRunCancelled
		}
		s.emitEvent(runCtx, Event{Type: evType, SessionID: s.id, RunID: runID, Timestamp: time.Now(), Data: map[string]any{"error": runErr.Error()}})
		return zero, runErr
	}

	s.mu.Lock()
	steps := s.lastSteps
	provider := s.lastProvider
	s.mu.Unlock()
	out := fromAgentResponse(resp, s.agent.ModelName(), provider, steps)
	s.emitEvent(runCtx, Event{Type: EventRunCompleted, SessionID: s.id, RunID: runID, Timestamp: time.Now(), Data: out})

	return result, nil
}
