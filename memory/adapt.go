package memory

import (
	"context"

	internalmemory "simon-go/internal/memory"
)

// ToInternal adapts any public Memory implementation into
// internal/memory.Memory, so it can be passed to agent.WithMemory. Used
// when a Runtime's MemoryFactory returns a custom Memory rather than one of
// this package's own wrappers.
func ToInternal(m Memory) internalmemory.Memory {
	return &internalAdapter{m: m}
}

type internalAdapter struct {
	m Memory
}

func (a *internalAdapter) Add(ctx context.Context, role, content string) error {
	return a.m.Add(ctx, Message{Role: Role(role), Content: content})
}

func (a *internalAdapter) List(ctx context.Context) ([]internalmemory.Message, error) {
	messages, err := a.m.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]internalmemory.Message, len(messages))
	for i, msg := range messages {
		out[i] = internalmemory.Message{Role: string(msg.Role), Content: msg.Content}
	}
	return out, nil
}

func (a *internalAdapter) Clear(ctx context.Context) error {
	return a.m.Clear(ctx)
}
