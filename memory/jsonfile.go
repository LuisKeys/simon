package memory

import (
	"context"

	internalmemory "simon-go/internal/memory"
)

// NewJSONFile returns a Memory store persisted as a single JSON file under
// .simon_chats/<basename of path>. Like NewInMemory, ToolCallID, CreatedAt,
// and Metadata are accepted but not persisted in this phase.
func NewJSONFile(path string) Memory {
	return &jsonFileWrapper{inner: internalmemory.NewJSONFile(path)}
}

type jsonFileWrapper struct {
	inner *internalmemory.JSONFile
}

func (w *jsonFileWrapper) Add(ctx context.Context, message Message) error {
	return w.inner.Add(ctx, string(message.Role), message.Content)
}

func (w *jsonFileWrapper) List(ctx context.Context) ([]Message, error) {
	history, err := w.inner.List(ctx)
	if err != nil {
		return nil, err
	}
	return fromInternal(history), nil
}

func (w *jsonFileWrapper) Clear(ctx context.Context) error {
	return w.inner.Clear(ctx)
}

func (w *jsonFileWrapper) Close() error { return nil }
