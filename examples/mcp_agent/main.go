// Command mcp_agent mirrors Python's examples/mcp_agent.py — using tools
// from an MCP server inside a Simon agent. The companion server lives at
// examples/mcp_agent/server/main.go (mirroring
// simon/tools/builtin/mcp_example_server.py) and is launched here with
// `go run`, mirroring Python's `[sys.executable, "mcp_example_server.py"]`.
package main

import (
	"context"
	"fmt"
	"log"

	"simon-go/internal/agent"
	"simon-go/internal/config"
	"simon-go/internal/mcp"
)

func main() {
	client := mcp.New("go", "run", "./examples/mcp_agent/server")

	ctx := context.Background()
	tools, err := client.Tools(ctx)
	if err != nil {
		log.Fatal(err)
	}

	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	fmt.Printf("Tools loaded from MCP server: %v\n\n", names)

	settings := config.Load()
	a := agent.New(settings, agent.WithTools(tools...))

	resp, err := a.Run(ctx, "Use the add_numbers tool to compute 37 + 5, then reverse the result string.")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.Text)
}
