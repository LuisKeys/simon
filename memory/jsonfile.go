package memory

import (
	"context"

	internalmemory "github.com/LuisKeys/simon/internal/memory"
)

// NewJSONFile returns a Memory store persisted as a single JSON file under
// .simon_chats/<basename of path>. Like NewInMemory, ToolCallID, CreatedAt,
// and Metadata are accepted but not persisted in this phase.
func NewJSONFile(path string) Memory {
	return &jsonFileWrapper{inner: internalmemory.NewJSONFile(path)}
}

// NewJSONFileIn returns a Memory store persisted as a single JSON file under
// dir/<basename of name>, instead of the default .simon_chats/. Use this
// when the process needs its chat history under a specific directory rather
// than relative to the working directory.
func NewJSONFileIn(dir, name string) Memory {
	return &jsonFileWrapper{inner: internalmemory.NewJSONFileIn(dir, name)}
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
