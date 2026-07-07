// Package privacy implements deny-by-default, auditable access control for
// sensors, mirroring Python's simon/privacy package (PermissionScope,
// PermissionManager, PermissionStore/SQLitePermissionStore).
package privacy

import (
	"context"
	"log/slog"
	"sync"

	"simon-go/internal/events"
)

// Scope is a granular capability a Sensor can request, split intentionally
// fine-grained (e.g. clipboard metadata vs content) so a user can allow
// "clipboard activity happened" without allowing the system to ever see
// what was copied.
type Scope string

const (
	WindowFocus       Scope = "window_focus"
	ClipboardMetadata Scope = "clipboard_metadata"
	ClipboardContent  Scope = "clipboard_content"
	ScreenText        Scope = "screen_text"
)

// Manager is a deny-by-default permission gate for sensors. Every grant,
// revoke, and denial is published on Bus (if configured) as an
// ActivityEvent, so "what did the system observe and why" stays answerable
// from the same event log the rest of the pipeline reads.
type Manager struct {
	store  Store
	bus    *events.EventBus
	mu     sync.RWMutex
	grants map[Scope]bool
	loaded bool
}

// NewManager builds a Manager backed by store, optionally auditing to bus
// (pass nil for no auditing).
func NewManager(store Store, bus *events.EventBus) *Manager {
	return &Manager{store: store, bus: bus, grants: map[Scope]bool{}}
}

// Initialize loads persisted grants into memory. Safe to call more than once.
func (m *Manager) Initialize(ctx context.Context) error {
	grants, err := m.store.LoadAll(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.grants = make(map[Scope]bool, len(grants))
	for k, v := range grants {
		m.grants[Scope(k)] = v
	}
	m.loaded = true
	m.mu.Unlock()
	return nil
}

// Close releases the underlying store's connection.
func (m *Manager) Close() error { return m.store.Close() }

// IsGranted checks the in-memory cache. Denies (returns false) if
// Initialize hasn't been called yet or the scope was never granted.
func (m *Manager) IsGranted(scope Scope) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.loaded {
		slog.Warn("IsGranted called before Initialize; denying", "scope", scope)
	}
	return m.grants[scope]
}

// Grant persists and audits granting scope.
func (m *Manager) Grant(ctx context.Context, scope Scope) error {
	if err := m.set(ctx, scope, true); err != nil {
		return err
	}
	return m.audit(ctx, events.PermissionGranted, scope, nil)
}

// Revoke persists and audits revoking scope.
func (m *Manager) Revoke(ctx context.Context, scope Scope) error {
	if err := m.set(ctx, scope, false); err != nil {
		return err
	}
	return m.audit(ctx, events.PermissionRevoked, scope, nil)
}

// Deny records that sensor attempted to run without a granted scope.
func (m *Manager) Deny(ctx context.Context, scope Scope, sensor string) error {
	return m.audit(ctx, events.PermissionDenied, scope, map[string]any{"sensor": sensor})
}

func (m *Manager) set(ctx context.Context, scope Scope, granted bool) error {
	if err := m.store.Set(ctx, string(scope), granted); err != nil {
		return err
	}
	m.mu.Lock()
	m.grants[scope] = granted
	m.mu.Unlock()
	return nil
}

func (m *Manager) audit(ctx context.Context, eventType string, scope Scope, extra map[string]any) error {
	if m.bus == nil {
		return nil
	}
	data := map[string]any{"scope": string(scope)}
	for k, v := range extra {
		data[k] = v
	}
	return m.bus.Publish(ctx, events.New(eventType, "privacy", data))
}
