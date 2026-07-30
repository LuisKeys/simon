// Command knowledge_router_agent demonstrates Knowledge Router: a
// hierarchical, lexical, embeddings-free alternative to Simon's vector
// KnowledgeBase. It loads a small curated catalog (category.yaml + document
// sidecar YAML files under ./knowledge), validates it, attaches it to an
// agent via agent.WithKnowledge exactly like the vector-backed
// knowledge_agent example does, and asks a couple of questions that can
// only be answered from the routed evidence.
//
// Retrieval can also be exercised without any LLM at all — pass -no-llm to
// print each question's SearchDetailed results (selected category,
// document, section, and evidence) instead of running the agent.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"simon-go/internal/agent"
	"simon-go/internal/config"
	"simon-go/internal/knowledge/router"
)

var questions = []string{
	"How do PostgreSQL workers claim jobs safely without colliding with each other?",
	"What happens to a job's lease if the worker that claimed it crashes?",
}

func main() {
	noLLM := flag.Bool("no-llm", false, "skip the agent/LLM call and only print SearchDetailed results")
	flag.Parse()

	r, err := router.New(router.Config{KnowledgePath: "examples/knowledge_router_agent/knowledge"})
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	if issues := r.Validate(ctx); len(issues) > 0 {
		fmt.Println("Validation issues:")
		for _, issue := range issues {
			fmt.Printf("  [%s] %s: %s\n", issue.Severity, issue.Path, issue.Message)
		}
		fmt.Println()
	}

	if *noLLM {
		runRetrievalOnly(ctx, r)
		return
	}

	settings := config.Load()
	a := agent.New(settings, agent.WithKnowledge(r))

	for _, q := range questions {
		fmt.Printf("Q: %s\n", q)
		resp, err := a.Run(ctx, q)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("A: %s\n\n", resp.Text)

		result, err := r.SearchDetailed(ctx, q, router.SearchOptions{IncludeContent: false})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Sources:")
		for _, s := range result.Sections {
			fmt.Printf("  - %s/%s\n", s.DocumentID, s.SectionID)
		}
		fmt.Println()
	}
}

func runRetrievalOnly(ctx context.Context, r *router.Router) {
	for _, q := range questions {
		fmt.Printf("Q: %s\n", q)
		result, err := r.SearchDetailed(ctx, q, router.SearchOptions{IncludeContent: true, Explain: true})
		if err != nil {
			log.Fatal(err)
		}
		for _, c := range result.Categories {
			fmt.Printf("  category  %-24s score=%.2f\n", c.Path, c.Score)
		}
		for _, d := range result.Documents {
			fmt.Printf("  document  %-24s score=%.2f\n", d.Title, d.Score)
		}
		for _, ev := range result.Evidence {
			fmt.Printf("  evidence  %s#%s (lines %d-%d)\n", ev.SourcePath, ev.SectionID, ev.StartLine, ev.EndLine)
		}
		fmt.Println()
	}
}
