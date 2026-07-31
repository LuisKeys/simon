// Package tool defines the public tool contract consumers of the simon SDK
// implement to give an agent new capabilities. It mirrors internal/tool's
// shape but as an interface rather than a concrete struct, and Execute
// returns (any, error) instead of (string, error) so handlers can return
// structured results without having to pre-serialize them to text.
package tool

import (
	"context"
	"encoding/json"

	internaltool "github.com/LuisKeys/simon/internal/tool"
)

// Handler executes a tool call given its raw JSON arguments.
type Handler func(ctx context.Context, arguments json.RawMessage) (any, error)

// Tool is a single callable tool: its wire-level identity (name,
// description, JSON schema) plus the logic that executes it.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Execute(ctx context.Context, arguments json.RawMessage) (any, error)
}

type simonTool struct {
	name        string
	description string
	schema      map[string]any
	handler     Handler
}

func (t *simonTool) Name() string           { return t.name }
func (t *simonTool) Description() string    { return t.description }
func (t *simonTool) Schema() map[string]any { return t.schema }
func (t *simonTool) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	return t.handler(ctx, arguments)
}

// New declares a tool whose parameters are described by the struct type P.
// P's fields determine the generated JSON schema via `json`/`jsonschema`
// struct tags, the same tags used to unmarshal a model's tool-call
// arguments before calling fn.
func New[P any](name, description string, fn func(ctx context.Context, params P) (any, error)) Tool {
	schema := internaltool.SchemaFor[P]()
	return &simonTool{
		name:        name,
		description: description,
		schema:      schema,
		handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var params P
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &params); err != nil {
					return nil, err
				}
			}
			return fn(ctx, params)
		},
	}
}

// NewRaw builds a Tool from an already-known JSON schema and a raw
// JSON-in handler, bypassing New's struct-reflection schema generation.
// Intended for tools whose schema is only known at runtime — notably MCP
// tools, whose input schema comes from the remote server, not a Go type.
func NewRaw(name, description string, schema map[string]any, handler Handler) Tool {
	if schema == nil {
		schema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return &simonTool{name: name, description: description, schema: schema, handler: handler}
}
