// Command structured_output_agent mirrors Python's
// examples/structured_output_agent.py — structured output parsed into a
// typed Recipe struct via agent.RunStructured.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"simon-go/internal/agent"
	"simon-go/internal/config"
)

// Recipe mirrors Python's Pydantic Recipe model.
type Recipe struct {
	Title           string   `json:"title"`
	Ingredients     []string `json:"ingredients"`
	Steps           []string `json:"steps"`
	PrepTimeMinutes int      `json:"prep_time_minutes"`
}

func main() {
	settings := config.Load()
	a := agent.New(settings)

	recipe, _, err := agent.RunStructured[Recipe](context.Background(), a, "Give me a simple pancake recipe")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Recipe: %s\n", recipe.Title)
	fmt.Printf("Prep time: %d minutes\n", recipe.PrepTimeMinutes)
	fmt.Printf("Ingredients: %s\n", strings.Join(recipe.Ingredients, ", "))
	fmt.Printf("Steps: %d steps\n", len(recipe.Steps))
}
