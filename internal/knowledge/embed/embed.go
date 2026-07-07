// Package embed implements Simon's embedding providers, mirroring Python's
// simon/knowledge/embeddings.py.
package embed

import (
	"context"
	"fmt"
	"math"
	"strings"

	"simon-go/internal/config"
	"simon-go/pkg/simonerr"
)

// Embedder produces normalized dense vectors for text, the Go analogue of
// Python's duck-typed embeddings classes (OpenAIEmbeddings, etc.).
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// normalize L2-normalizes vec in place semantics (returns a new slice),
// mirroring the `_normalize` staticmethod duplicated across every Python
// embeddings class.
func normalize(vec []float32) []float32 {
	var sumSquares float64
	for _, v := range vec {
		sumSquares += float64(v) * float64(v)
	}
	norm := math.Sqrt(sumSquares)
	if norm == 0 {
		norm = 1.0
	}
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = float32(float64(v) / norm)
	}
	return out
}

// Default returns the embeddings provider configured via
// settings.EmbeddingProvider, mirroring default_embeddings().
func Default(settings config.Settings) (Embedder, error) {
	switch strings.ToUpper(settings.EmbeddingProvider) {
	case "OLLAMA":
		return NewOllama(settings.OllamaHost, settings.EmbeddingModel), nil
	case "ANTHROPIC":
		return NewVoyage(settings.AnthropicAPIKey, settings.EmbeddingModel), nil
	case "OPENAI":
		return NewOpenAI(settings.OpenAIAPIKey, settings.EmbeddingModel), nil
	default:
		return nil, simonerr.NewKnowledgeError(
			fmt.Sprintf("unknown EMBEDDING_PROVIDER %q. Valid values: OPENAI, OLLAMA, ANTHROPIC", settings.EmbeddingProvider), nil)
	}
}
