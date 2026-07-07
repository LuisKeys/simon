// Command knowledge_agent mirrors Python's examples/knowledge_agent.py —
// index a PDF into the knowledge base, then ask the agent questions that
// can only be answered from that document.
package main

import (
	"context"
	"fmt"
	"log"

	"simon-go/internal/agent"
	"simon-go/internal/config"
	"simon-go/internal/knowledge"
	"simon-go/internal/knowledge/embed"
)

const prompt = `
You are a friendly professor explaining AI concepts to a curious 20-year-old.
Using only the document you have been given, explain the following topics.
Keep each answer under 3 sentences. Avoid all math and jargon — use simple
real-world analogies wherever possible.

1. What is a Transformer model and what problem does it solve?
2. What does "attention" mean in this context? Give a simple real-world analogy.
3. What do the encoder and decoder do? (one sentence each)
4. What is multi-head attention, in plain English?
5. Why is positional encoding needed and how can we think about it simply?
`

func main() {
	const paperPath = "examples/knowledge_agent/docs/attention_paper.pdf"

	settings := config.Load()

	embedder, err := embed.Default(settings)
	if err != nil {
		log.Fatal(err)
	}
	storePath := settings.KnowledgeStorePath
	if storePath == "" {
		storePath = ".simon_knowledge"
	}
	kb, err := knowledge.New(embedder, storePath)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	chunks, err := kb.Add(ctx, paperPath, false)
	if err != nil {
		log.Fatal(err)
	}
	if chunks > 0 {
		fmt.Printf("Indexed %d chunks from %s\n\n", chunks, paperPath)
	} else {
		fmt.Printf("%s already indexed — skipping.\n\n", paperPath)
	}
	// To force re-indexing: kb.Add(ctx, paperPath, true)

	a := agent.New(settings, agent.WithKnowledge(kb))
	resp, err := a.Run(ctx, prompt)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.Text)
}
