package tool

import (
	"context"
	"encoding/json"

	internaltool "simon-go/internal/tool"
)

// ToInternal adapts a public Tool into internal/tool.Tool so it can be
// registered with an internal ReAct loop (agent.WithTools). approve, if
// non-nil, is called before Execute and can veto the call by returning a
// non-nil error (e.g. an ApprovalPolicy denial) — this is the hook the
// simon facade uses to enforce tool approval, since the internal ReAct loop
// has no per-call interception point of its own.
func ToInternal(t Tool, approve func(ctx context.Context, name string, arguments json.RawMessage) error) internaltool.Tool {
	return internaltool.NewRaw(t.Name(), t.Description(), t.Schema(),
		func(ctx context.Context, raw json.RawMessage) (string, error) {
			if approve != nil {
				if err := approve(ctx, t.Name(), raw); err != nil {
					return "", err
				}
			}
			result, err := t.Execute(ctx, raw)
			if err != nil {
				return "", err
			}
			return stringify(result)
		})
}

// stringify converts a tool's Execute result into the text the internal
// ReAct loop feeds back to the model. Strings pass through unchanged;
// everything else is JSON-encoded.
func stringify(result any) (string, error) {
	if s, ok := result.(string); ok {
		return s, nil
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
