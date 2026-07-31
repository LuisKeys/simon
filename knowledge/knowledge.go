// Package knowledge defines the public retrieval-augmented-generation
// contract consumers of the simon SDK implement or use to attach a
// knowledge base to a Runtime or Session.
package knowledge

import (
	"context"

	"github.com/LuisKeys/simon/pkg/simonerr"
)

// Hit is a single retrieved chunk.
type Hit struct {
	Text     string
	Source   string
	Score    float32
	Metadata map[string]any
}

// Searcher is the read-only contract the agent's ReAct loop needs to
// inject knowledge context into a run.
type Searcher interface {
	Search(ctx context.Context, query string, topK int) ([]Hit, error)
}

// AddOptions configures a single Add call.
type AddOptions struct {
	// Force re-indexes a source even if it was already indexed.
	Force bool
}

// AddResult reports the outcome of a single Add call.
type AddResult struct {
	ChunksAdded int
}

// Store is a full knowledge base: search plus ingestion/removal.
type Store interface {
	Searcher
	Add(ctx context.Context, source string, opts AddOptions) (AddResult, error)
	Remove(ctx context.Context, source string) error
	Close() error
}

// Embedder produces normalized dense vectors for text. Implement this to
// plug a custom embedding provider into Open.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// Option configures a Store at construction time.
type Option func(*options)

type options struct {
	chunkSize int
	overlap   int
}

// WithChunkSize overrides the default chunk size (characters) used when
// splitting a source's text into embeddable pieces.
func WithChunkSize(n int) Option { return func(o *options) { o.chunkSize = n } }

// WithOverlap overrides the default overlap (characters) between
// consecutive chunks.
func WithOverlap(n int) Option { return func(o *options) { o.overlap = n } }

// errNotSupported is returned by Remove until the underlying index format
// supports deletion.
var errNotSupported = simonerr.NewKnowledgeError("knowledge: Remove is not supported by this store yet", nil)
