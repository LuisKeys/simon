// Command server is a standalone MCP stdio server used by
// examples/mcp_agent, mirroring Python's
// simon/tools/builtin/mcp_example_server.py. It exposes two trivial tools
// (add_numbers, reverse_string) modeled on internal/mcp/mcp_test.go's
// runHelperServer pattern.
package main

import (
	"context"
	"os"
	"strconv"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type addNumbersArgs struct {
	A int `json:"a"`
	B int `json:"b"`
}

type reverseStringArgs struct {
	Text string `json:"text"`
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

func main() {
	server := sdk.NewServer(&sdk.Implementation{Name: "simon-example", Version: "0.1.0"}, nil)

	sdk.AddTool(server, &sdk.Tool{Name: "add_numbers", Description: "Add two integers and return their sum."},
		func(_ context.Context, _ *sdk.CallToolRequest, args addNumbersArgs) (*sdk.CallToolResult, any, error) {
			sum := args.A + args.B
			return &sdk.CallToolResult{
				Content: []sdk.Content{&sdk.TextContent{Text: strconv.Itoa(sum)}},
			}, nil, nil
		})

	sdk.AddTool(server, &sdk.Tool{Name: "reverse_string", Description: "Reverse the characters in a string."},
		func(_ context.Context, _ *sdk.CallToolRequest, args reverseStringArgs) (*sdk.CallToolResult, any, error) {
			return &sdk.CallToolResult{
				Content: []sdk.Content{&sdk.TextContent{Text: reverse(args.Text)}},
			}, nil, nil
		})

	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		os.Exit(1)
	}
}
