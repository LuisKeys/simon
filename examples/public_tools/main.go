// Command public_tools demonstrates registering a typed tool and driving a
// full tool-call round trip: a scripted Model requests the tool on its
// first reply, then produces a final answer once given the tool's result.
package main

import (
	"context"
	"fmt"
	"log"

	"simon-go/model"
	"simon-go/simon"
	"simon-go/tool"
)

// AddParams is the parameter struct for the "add" tool — its json/jsonschema
// tags drive the generated JSON schema.
type AddParams struct {
	A int `json:"a" jsonschema:"required"`
	B int `json:"b" jsonschema:"required"`
}

// scriptedModel is a tiny hand-rolled model.Model standing in for a real
// LLM: it requests the "add" tool once, then answers using the tool's
// result — enough to exercise the tool-call loop deterministically.
type scriptedModel struct{}

func (scriptedModel) Complete(_ context.Context, req model.Request) (model.Response, error) {
	// Second call in the loop: the agent has already executed the tool and
	// appended its result as a tool-role message, so wrap it up.
	for _, msg := range req.Messages {
		if msg.Role == model.RoleTool {
			return model.Response{Text: "The sum is " + msg.Content}, nil
		}
	}
	// First call in the loop: no tool result yet, so request the "add"
	// tool with fixed arguments instead of answering directly.
	return model.Response{
		ToolCalls: []model.ToolCall{
			{ID: "call-1", Name: "add", Arguments: map[string]any{"a": 2, "b": 3}},
		},
	}, nil
}

func main() {
	// tool.New reflects AddParams' json/jsonschema tags to build the tool's
	// JSON schema, then wraps the function as a tool.Tool the agent loop
	// can dispatch to by name.
	addTool := tool.New("add", "Add two integers", func(_ context.Context, p AddParams) (any, error) {
		return p.A + p.B, nil
	})

	rt, err := simon.New(simon.WithModel(scriptedModel{}))
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	// RegisterTool makes the tool available to every session created from
	// this Runtime.
	if err := rt.RegisterTool(addTool); err != nil {
		log.Fatal(err)
	}

	session, err := rt.NewSession("main")
	if err != nil {
		log.Fatal(err)
	}

	// Run drives the full loop: model requests "add" -> runtime executes
	// addTool -> result fed back to the model -> final answer returned.
	resp, err := session.Run(context.Background(), "What is 2 + 3?")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.Text)
}
