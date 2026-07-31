// Package memory defines the public conversation-history contract consumers
// of the simon SDK implement or use to give a Session persistent history.
// It mirrors internal/memory's shape but adds fields (ToolCallID,
// CreatedAt, Metadata) and a Close method, so it does not alias the
// internal type.
package memory

import "context"
import "time"

// Role identifies who authored a Message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is a single stored conversation turn.
type Message struct {
	Role       Role
	Content    string
	ToolCallID string
	CreatedAt  time.Time
	Metadata   map[string]any
}

// Memory is the conversation-history protocol a Session's backing store
// implements.
type Memory interface {
	Add(ctx context.Context, message Message) error
	List(ctx context.Context) ([]Message, error)
	Clear(ctx context.Context) error
	Close() error
}

// Factory builds a Memory for a given session, letting each Session obtain
// independent storage (e.g. one SQLite row-set or JSON file per session ID).
type Factory interface {
	NewMemory(ctx context.Context, sessionID string) (Memory, error)
}

// FactoryFunc adapts a plain function to Factory.
type FactoryFunc func(ctx context.Context, sessionID string) (Memory, error)

// NewMemory implements Factory.
func (f FactoryFunc) NewMemory(ctx context.Context, sessionID string) (Memory, error) {
	return f(ctx, sessionID)
}
