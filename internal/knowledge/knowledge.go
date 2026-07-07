// Package knowledge implements document ingestion + retrieval, mirroring
// Python's simon/knowledge/knowledge.py KnowledgeBase. Chunking, file
// extension dispatch, and the add/search contract match Python; the
// on-disk index format is the from-scratch SIDX design in
// internal/knowledge/index (see that package's doc comment) rather than
// pickle+numpy.
package knowledge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"simon-go/internal/agent/response"
	"simon-go/internal/knowledge/embed"
	"simon-go/internal/knowledge/extract"
	"simon-go/internal/knowledge/index"
)

// KnowledgeBase chunks and indexes documents, then serves similarity search
// over them.
type KnowledgeBase struct {
	ChunkSize int
	Overlap   int

	storePath string
	embedder  embed.Embedder
	idx       index.Index
}

// Option configures a KnowledgeBase at construction time.
type Option func(*KnowledgeBase)

func WithChunkSize(n int) Option { return func(kb *KnowledgeBase) { kb.ChunkSize = n } }
func WithOverlap(n int) Option   { return func(kb *KnowledgeBase) { kb.Overlap = n } }

// New builds a KnowledgeBase backed by embedder and storePath (an
// index.FileIndex directory), matching Python's KnowledgeBase(chunk_size,
// overlap, store_path) plus its lazily-constructed FileRetriever.
func New(embedder embed.Embedder, storePath string, opts ...Option) (*KnowledgeBase, error) {
	idx, err := index.Open(storePath)
	if err != nil {
		return nil, err
	}
	kb := &KnowledgeBase{ChunkSize: 500, Overlap: 50, storePath: storePath, embedder: embedder, idx: idx}
	for _, opt := range opts {
		opt(kb)
	}
	return kb, nil
}

// Add indexes path (a file or, recursively, every file under a directory),
// returning the number of chunks newly indexed. Files already indexed are
// skipped unless force is set, matching Python's `add(path, force=False)`.
func (kb *KnowledgeBase) Add(ctx context.Context, path string, force bool) (int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("knowledge: path not found: %s: %w", path, err)
	}

	var files []string
	if info.IsDir() {
		if err := filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				files = append(files, p)
			}
			return nil
		}); err != nil {
			return 0, err
		}
	} else {
		files = []string{path}
	}

	total := 0
	for _, file := range files {
		if kb.idx.HasSource(file) && !force {
			continue
		}
		text, err := extract.Text(file)
		if err != nil {
			return total, err
		}
		chunks := kb.chunk(text)
		if len(chunks) == 0 {
			continue
		}
		vectors, err := kb.embedder.EmbedBatch(ctx, chunks)
		if err != nil {
			return total, err
		}
		indexChunks := make([]index.Chunk, len(chunks))
		for i, c := range chunks {
			indexChunks[i] = index.Chunk{Text: c, Source: file}
		}
		if err := kb.idx.AddSource(ctx, file, indexChunks, vectors, force); err != nil {
			return total, err
		}
		total += len(chunks)
	}
	return total, nil
}

// Search returns the topK chunks most similar to query. Matching Python,
// Search returns no results (rather than erroring) if the store directory
// doesn't exist yet.
func (kb *KnowledgeBase) Search(ctx context.Context, query string, topK int) ([]response.KnowledgeHit, error) {
	if _, err := os.Stat(kb.storePath); os.IsNotExist(err) {
		return nil, nil
	}

	qvec, err := kb.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	results, err := kb.idx.Search(ctx, qvec, topK)
	if err != nil {
		return nil, err
	}

	hits := make([]response.KnowledgeHit, len(results))
	for i, r := range results {
		hits[i] = response.KnowledgeHit{Text: r.Text, Source: r.Source, Score: r.Score}
	}
	return hits, nil
}

// chunk splits text into overlapping fixed-size windows, mirroring
// Python's character-based (not token-based) sliding-window _chunk.
func (kb *KnowledgeBase) chunk(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	runes := []rune(text)
	step := kb.ChunkSize - kb.Overlap
	if step < 1 {
		step = 1
	}

	var chunks []string
	for i := 0; i < len(runes); i += step {
		end := i + kb.ChunkSize
		if end > len(runes) {
			end = len(runes)
		}
		if c := strings.TrimSpace(string(runes[i:end])); c != "" {
			chunks = append(chunks, c)
		}
	}
	return chunks
}
