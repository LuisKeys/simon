package simon

import (
	"context"
	"encoding/json"
)

// ApprovalRequest describes a tool call awaiting an approval decision.
type ApprovalRequest struct {
	SessionID string
	ToolName  string
	Arguments json.RawMessage
}

// ApprovalPolicy gates tool execution, giving desktop/interactive
// applications a hook to require human confirmation before a sensitive
// tool call runs.
type ApprovalPolicy interface {
	// Approve returns true to allow the call, false (with a nil error) to
	// deny it silently, or a non-nil error to deny it and surface why.
	Approve(ctx context.Context, request ApprovalRequest) (bool, error)
}

// AllowAll is the default ApprovalPolicy: every tool call is permitted
// without prompting.
type AllowAll struct{}

// Approve implements ApprovalPolicy.
func (AllowAll) Approve(context.Context, ApprovalRequest) (bool, error) {
	return true, nil
}
