// Command public_structured_output demonstrates simon.RunStructured: a
// scripted Model replies with raw JSON (inside markdown fences, to show
// that fences are stripped), which RunStructured parses into a typed
// Go struct.
package main

import (
	"context"
	"fmt"
	"log"

	"simon-go/model"
	"simon-go/simon"
)

// ProjectPlan is the target shape for structured output.
type ProjectPlan struct {
	Title string   `json:"title"`
	Steps []string `json:"steps"`
}

type scriptedModel struct{}

// Complete always replies with the same JSON wrapped in a markdown code
// fence, mimicking how a real LLM often formats JSON output. RunStructured
// is expected to strip the fence before unmarshaling.
func (scriptedModel) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	return model.Response{Text: "```json\n" +
		`{"title": "Ship the SDK", "steps": ["design", "implement", "test"]}` +
		"\n```"}, nil
}

func main() {
	rt, err := simon.New(simon.WithModel(scriptedModel{}))
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	session, err := rt.NewSession("main")
	if err != nil {
		log.Fatal(err)
	}

	// RunStructured is a generic helper: it runs the prompt like Run, then
	// unmarshals the model's JSON reply directly into the given type
	// parameter instead of returning raw text.
	plan, err := simon.RunStructured[ProjectPlan](context.Background(), session, "Plan the SDK rollout")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%s: %v\n", plan.Title, plan.Steps)
}
