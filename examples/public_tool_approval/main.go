// Command public_tool_approval demonstrates ApprovalPolicy: a custom
// policy denies a "delete_file" tool call and allows everything else,
// showing both outcomes without ever touching the filesystem.
package main

import (
	"context"
	"fmt"

	"simon-go/model"
	"simon-go/simon"
	"simon-go/tool"
)

// denyDestructive denies any tool named "delete_file" and allows everything
// else — a stand-in for a real desktop app's "confirm before destructive
// action" prompt.
type denyDestructive struct{}

// Approve is called by the runtime before executing any requested tool
// call; returning false blocks that call without ever invoking the tool
// function.
func (denyDestructive) Approve(_ context.Context, req simon.ApprovalRequest) (bool, error) {
	return req.ToolName != "delete_file", nil
}

// deleteParams is reused as the parameter struct for both tools below since
// they share the same shape (a single file path).
type deleteParams struct {
	Path string `json:"path" jsonschema:"required"`
}

// scriptedModel requests the given tool once, then reports whatever the
// tool call returned (including a denial error's message).
type scriptedModel struct{ toolName string }

func (m scriptedModel) Complete(_ context.Context, req model.Request) (model.Response, error) {
	// If a tool result is already in the conversation, report it and stop.
	for _, msg := range req.Messages {
		if msg.Role == model.RoleTool {
			return model.Response{Text: "tool result: " + msg.Content}, nil
		}
	}
	// Otherwise this is the first call: request m.toolName be run. If the
	// approval policy denies it, the agent loop feeds the denial back as a
	// tool-role message on the next Complete call above, rather than
	// actually running the tool function.
	return model.Response{
		ToolCalls: []model.ToolCall{{ID: "call-1", Name: m.toolName, Arguments: map[string]any{"path": "/tmp/x"}}},
	}, nil
}

func main() {
	// Two tools with the same signature: one destructive, one harmless.
	// denyDestructive blocks the first but not the second.
	deleteFile := tool.New("delete_file", "Delete a file", func(_ context.Context, p deleteParams) (any, error) {
		return "deleted " + p.Path, nil
	})
	readFile := tool.New("read_file", "Read a file", func(_ context.Context, p deleteParams) (any, error) {
		return "contents of " + p.Path, nil
	})

	// run builds a fresh Runtime that always requests the named tool, so
	// both the allowed and denied paths can be demonstrated in isolation.
	run := func(name string) {
		sessRt, _ := simon.New(simon.WithApprovalPolicy(denyDestructive{}), simon.WithModel(scriptedModel{toolName: name}))
		defer sessRt.Close()
		_ = sessRt.RegisterTools(deleteFile, readFile)
		session, _ := sessRt.NewSession("main")
		resp, err := session.Run(context.Background(), "please "+name)
		if err != nil {
			fmt.Println(name, "failed:", err)
			return
		}
		fmt.Println(name, "->", resp.Text)
	}

	run("read_file")   // approved: prints "tool result: contents of /tmp/x"
	run("delete_file") // denied by denyDestructive before the tool ever runs
}
