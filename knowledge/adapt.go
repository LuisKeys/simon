package knowledge

import (
	"context"

	"github.com/LuisKeys/simon/internal/agent/response"
	"github.com/LuisKeys/simon/internal/config"
	internalknowledge "github.com/LuisKeys/simon/internal/knowledge"
	internalembed "github.com/LuisKeys/simon/internal/knowledge/embed"
)

// Open builds a Store backed by embedder and storePath (an on-disk index
// directory).
func Open(embedder Embedder, storePath string, opts ...Option) (Store, error) {
	cfg := options{chunkSize: 500, overlap: 50}
	for _, opt := range opts {
		opt(&cfg)
	}
	kb, err := internalknowledge.New(&embedderAdapter{e: embedder}, storePath,
		internalknowledge.WithChunkSize(cfg.chunkSize),
		internalknowledge.WithOverlap(cfg.overlap),
	)
	if err != nil {
		return nil, err
	}
	return &storeAdapter{kb: kb}, nil
}

// OpenFromEnv builds a Store backed by storePath, selecting an embedding
// provider from environment variables/.env the same way the simon CLI
// does (EMBEDDING_PROVIDER, EMBEDDING_MODEL, and provider API keys). Use
// Open directly to supply a custom Embedder instead.
func OpenFromEnv(storePath string, opts ...Option) (Store, error) {
	settings := config.Load()
	embedder, err := internalembed.Default(settings)
	if err != nil {
		return nil, err
	}
	cfg := options{chunkSize: 500, overlap: 50}
	for _, opt := range opts {
		opt(&cfg)
	}
	kb, err := internalknowledge.New(embedder, storePath,
		internalknowledge.WithChunkSize(cfg.chunkSize),
		internalknowledge.WithOverlap(cfg.overlap),
	)
	if err != nil {
		return nil, err
	}
	return &storeAdapter{kb: kb}, nil
}

// embedderAdapter adapts a public Embedder into internal/knowledge/embed.Embedder.
type embedderAdapter struct {
	e Embedder
}

func (a *embedderAdapter) Embed(ctx context.Context, text string) ([]float32, error) {
	return a.e.Embed(ctx, text)
}

func (a *embedderAdapter) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return a.e.EmbedBatch(ctx, texts)
}

// storeAdapter adapts *internal/knowledge.KnowledgeBase into the public Store
// interface.
type storeAdapter struct {
	kb *internalknowledge.KnowledgeBase
}

func (s *storeAdapter) Search(ctx context.Context, query string, topK int) ([]Hit, error) {
	hits, err := s.kb.Search(ctx, query, topK)
	if err != nil {
		return nil, err
	}
	out := make([]Hit, len(hits))
	for i, h := range hits {
		out[i] = Hit{Text: h.Text, Source: h.Source, Score: h.Score}
	}
	return out, nil
}

func (s *storeAdapter) Add(ctx context.Context, source string, opts AddOptions) (AddResult, error) {
	n, err := s.kb.Add(ctx, source, opts.Force)
	if err != nil {
		return AddResult{}, err
	}
	return AddResult{ChunksAdded: n}, nil
}

func (s *storeAdapter) Remove(ctx context.Context, source string) error {
	return errNotSupported
}

func (s *storeAdapter) Close() error { return nil }

// searcherAdapter adapts a public Searcher into the KnowledgeSearcher shape
// internal/agent expects (Search returning internal response.KnowledgeHit).
// The concrete type is returned (not an interface) since agent.KnowledgeSearcher
// is unexported from internal/agent — Go's structural typing lets callers in
// the simon package pass this value anywhere that interface is expected.
type searcherAdapter struct {
	s Searcher
}

func (a *searcherAdapter) Search(ctx context.Context, query string, topK int) ([]response.KnowledgeHit, error) {
	hits, err := a.s.Search(ctx, query, topK)
	if err != nil {
		return nil, err
	}
	out := make([]response.KnowledgeHit, len(hits))
	for i, h := range hits {
		out[i] = response.KnowledgeHit{Text: h.Text, Source: h.Source, Score: h.Score}
	}
	return out, nil
}

// ToAgentSearcher adapts a public Searcher so it can be passed to
// agent.WithKnowledge, translating Hit into response.KnowledgeHit.
func ToAgentSearcher(s Searcher) *searcherAdapter {
	return &searcherAdapter{s: s}
}
