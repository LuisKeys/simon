// Package memory implements pluggable conversation history, mirroring
// Python's simon/memory package (BaseMemory ABC, InMemoryMemory,
// JSONFileMemory).
package memory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Message is a stored {role, content} turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Memory is the minimal conversation-history protocol (the Go analogue of
// Python's BaseMemory ABC).
type Memory interface {
	Add(ctx context.Context, role, content string) error
	List(ctx context.Context) ([]Message, error)
	Clear(ctx context.Context) error
}

// InMemory is simple in-process storage, designed to be swapped out later.
// A mutex guards the slice because, unlike Python's single-threaded asyncio
// event loop, Go callers (e.g. internal/multi's goroutine-based AgentPool)
// may call Add/List/Clear concurrently.
type InMemory struct {
	mu       sync.Mutex
	messages []Message
}

// NewInMemory returns an empty InMemory store.
func NewInMemory() *InMemory {
	return &InMemory{}
}

func (m *InMemory) Add(_ context.Context, role, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, Message{Role: role, Content: content})
	return nil
}

func (m *InMemory) List(_ context.Context) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Message, len(m.messages))
	copy(out, m.messages)
	return out, nil
}

func (m *InMemory) Clear(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = nil
	return nil
}

// chatsDir mirrors Python's `_CHATS_DIR = Path(".simon_chats")`.
const chatsDir = ".simon_chats"

// JSONFile is persistent conversation history backed by a single
// human-readable JSON file under .simon_chats/. Only the base filename of
// name is used (mirroring Python's `Path(name).name`), so callers cannot
// escape the chats directory via path traversal.
type JSONFile struct {
	mu       sync.Mutex
	path     string
	messages []Message // nil until first load, mirroring Python's lazy _messages
	loaded   bool
}

// NewJSONFile returns a JSONFile store rooted at .simon_chats/<basename of name>.
// An empty name defaults to "conversation.json", matching Python's default.
func NewJSONFile(name string) *JSONFile {
	if name == "" {
		name = "conversation.json"
	}
	return &JSONFile{path: filepath.Join(chatsDir, filepath.Base(name))}
}

func (m *JSONFile) load() error {
	if m.loaded {
		return nil
	}
	data, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		m.messages = []Message{}
		m.loaded = true
		return nil
	}
	if err != nil {
		return err
	}
	var messages []Message
	if err := json.Unmarshal(data, &messages); err != nil {
		return err
	}
	m.messages = messages
	m.loaded = true
	return nil
}

func (m *JSONFile) save() error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	messages := m.messages
	if messages == nil {
		messages = []Message{}
	}
	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0o644)
}

func (m *JSONFile) Add(_ context.Context, role, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.load(); err != nil {
		return err
	}
	m.messages = append(m.messages, Message{Role: role, Content: content})
	return m.save()
}

func (m *JSONFile) List(_ context.Context) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.load(); err != nil {
		return nil, err
	}
	out := make([]Message, len(m.messages))
	copy(out, m.messages)
	return out, nil
}

func (m *JSONFile) Clear(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = []Message{}
	m.loaded = true
	return m.save()
}
