package mcp

import (
	"context"
	"os"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMain re-executes this test binary as a standalone MCP stdio server
// when GO_MCP_HELPER_PROCESS is set, the standard Go pattern for testing
// code that spawns subprocesses (see os/exec's own tests) — Client always
// launches a real child process, so there is no way to test it without one.
func TestMain(m *testing.M) {
	if os.Getenv("GO_MCP_HELPER_PROCESS") == "1" {
		runHelperServer()
		return
	}
	os.Exit(m.Run())
}

type greetArgs struct {
	Name string `json:"name"`
}

func runHelperServer() {
	server := sdk.NewServer(&sdk.Implementation{Name: "test-server", Version: "0.0.1"}, nil)
	sdk.AddTool(server, &sdk.Tool{Name: "greet", Description: "Greets someone"},
		func(ctx context.Context, req *sdk.CallToolRequest, args greetArgs) (*sdk.CallToolResult, any, error) {
			return &sdk.CallToolResult{
				Content: []sdk.Content{&sdk.TextContent{Text: "hello, " + args.Name}},
			}, nil, nil
		})
	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func helperCommand(t *testing.T) []string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return []string{exe, "-test.run=TestMain"}
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	os.Setenv("GO_MCP_HELPER_PROCESS", "1")
	t.Cleanup(func() { os.Unsetenv("GO_MCP_HELPER_PROCESS") })
	return New(helperCommand(t)...)
}

func TestToolsListsAndDescribesServerTools(t *testing.T) {
	c := newTestClient(t)

	tools, err := c.Tools(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "greet" {
		t.Fatalf("expected 1 tool named greet, got %+v", tools)
	}
	if tools[0].Description != "Greets someone" {
		t.Errorf("Description = %q", tools[0].Description)
	}
}

func TestToolCallInvokesRemoteServer(t *testing.T) {
	c := newTestClient(t)

	tools, err := c.Tools(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err := tools[0].Call(context.Background(), []byte(`{"name":"Ada"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello, Ada" {
		t.Errorf("got %q", out)
	}
}
