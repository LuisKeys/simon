package memory

import (
	"context"

	internalmemory "github.com/LuisKeys/simon/internal/memory"
)

// NewInMemory returns an empty, process-local Memory store. ToolCallID,
// CreatedAt, and Metadata are accepted by Add but not persisted — the
// underlying store only keeps role/content, matching internal/memory's
// InMemory today.
func NewInMemory() Memory {
	return &inMemoryWrapper{inner: internalmemory.NewInMemory()}
}

type inMemoryWrapper struct {
	inner *internalmemory.InMemory
}

func (w *inMemoryWrapper) Add(ctx context.Context, message Message) error {
	return w.inner.Add(ctx, string(message.Role), message.Content)
}

func (w *inMemoryWrapper) List(ctx context.Context) ([]Message, error) {
	history, err := w.inner.List(ctx)
	if err != nil {
		return nil, err
	}
	return fromInternal(history), nil
}

func (w *inMemoryWrapper) Clear(ctx context.Context) error {
	return w.inner.Clear(ctx)
}

func (w *inMemoryWrapper) Close() error { return nil }

func fromInternal(history []internalmemory.Message) []Message {
	out := make([]Message, len(history))
	for i, m := range history {
		out[i] = Message{Role: Role(m.Role), Content: m.Content}
	}
	return out
}
