// Package mcp connects to an MCP server over stdio and exposes its tools
// as Simon tool.Tool values, mirroring Python's simon/tools/mcp_client.py
// MCPClient.
package mcp

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"simon-go/internal/tool"
	"simon-go/pkg/simonerr"
)

// Client connects to an MCP server via stdio, launched by command.
type Client struct {
	command []string
	impl    *sdk.Implementation
}

// New builds a Client that launches the MCP server with command (argv[0]
// is the executable, the rest are its arguments).
func New(command ...string) *Client {
	return &Client{
		command: command,
		impl:    &sdk.Implementation{Name: "simon", Version: "0.1.0"},
	}
}

func (c *Client) newSession(ctx context.Context) (*sdk.ClientSession, error) {
	cmd := exec.Command(c.command[0], c.command[1:]...)
	client := sdk.NewClient(c.impl, nil)
	session, err := client.Connect(ctx, &sdk.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return nil, simonerr.NewProviderError("mcp: connecting to server failed", err)
	}
	return session, nil
}

// Tools connects to the server, lists its tools, and wraps each as a
// tool.Tool. Like Python's MCPClient, each subsequent Call opens a fresh
// stdio connection rather than keeping this one open.
func (c *Client) Tools(ctx context.Context) ([]tool.Tool, error) {
	session, err := c.newSession(ctx)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, simonerr.NewProviderError("mcp: listing tools failed", err)
	}

	tools := make([]tool.Tool, 0, len(result.Tools))
	for _, mcpTool := range result.Tools {
		tools = append(tools, c.wrap(mcpTool))
	}
	return tools, nil
}

func (c *Client) wrap(mcpTool *sdk.Tool) tool.Tool {
	name := mcpTool.Name
	description := mcpTool.Description
	if description == "" {
		description = "Tool " + name
	}
	schema, _ := mcpTool.InputSchema.(map[string]any)

	return tool.NewRaw(name, description, schema, func(ctx context.Context, raw json.RawMessage) (string, error) {
		return c.call(ctx, name, raw)
	})
}

func (c *Client) call(ctx context.Context, name string, rawArgs json.RawMessage) (string, error) {
	session, err := c.newSession(ctx)
	if err != nil {
		return "", err
	}
	defer session.Close()

	var args map[string]any
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return "", simonerr.NewToolError("mcp: invalid arguments", err)
		}
	}

	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return "", simonerr.NewProviderError("mcp: tool call failed", err)
	}

	var parts []string
	for _, content := range result.Content {
		if text, ok := content.(*sdk.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n"), nil
}
