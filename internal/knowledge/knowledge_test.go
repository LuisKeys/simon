package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeEmbedder maps text deterministically to a 2-D vector based on
// whether it mentions "cat" or "dog", so search ranking is verifiable
// without a real embeddings provider (matching the migration plan's
// "hash-based pseudo-embedding" strategy for knowledge-search parity).
type fakeEmbedder struct{}

func (fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if strings.Contains(strings.ToLower(text), "cat") {
		return []float32{1, 0}, nil
	}
	return []float32{0, 1}, nil
}

func (f fakeEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i], _ = f.Embed(ctx, t)
	}
	return out, nil
}

func TestAddAndSearchRoundTrip(t *testing.T) {
	dir := t.TempDir()
	kb, err := New(fakeEmbedder{}, filepath.Join(dir, ".simon_knowledge"))
	if err != nil {
		t.Fatal(err)
	}

	docPath := filepath.Join(dir, "pets.txt")
	if err := os.WriteFile(docPath, []byte("I have a cat named Whiskers."), 0o644); err != nil {
		t.Fatal(err)
	}

	count, err := kb.Add(context.Background(), docPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("expected at least one chunk indexed")
	}

	hits, err := kb.Search(context.Background(), "tell me about cats", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || !strings.Contains(hits[0].Text, "cat") {
		t.Errorf("expected a cat-related hit first, got %+v", hits)
	}
}

func TestAddSkipsAlreadyIndexedUnlessForced(t *testing.T) {
	dir := t.TempDir()
	kb, _ := New(fakeEmbedder{}, filepath.Join(dir, ".simon_knowledge"))
	docPath := filepath.Join(dir, "doc.txt")
	_ = os.WriteFile(docPath, []byte("original content about dogs"), 0o644)

	first, err := kb.Add(context.Background(), docPath, false)
	if err != nil || first == 0 {
		t.Fatalf("first add: count=%d err=%v", first, err)
	}

	second, err := kb.Add(context.Background(), docPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if second != 0 {
		t.Errorf("expected re-adding without force to index 0 new chunks, got %d", second)
	}

	third, err := kb.Add(context.Background(), docPath, true)
	if err != nil || third == 0 {
		t.Errorf("expected force=true to reindex, count=%d err=%v", third, err)
	}
}

func TestAddIndexesDirectoryRecursively(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	_ = os.MkdirAll(filepath.Join(docsDir, "sub"), 0o755)
	_ = os.WriteFile(filepath.Join(docsDir, "a.txt"), []byte("cats are great pets"), 0o644)
	_ = os.WriteFile(filepath.Join(docsDir, "sub", "b.txt"), []byte("dogs are loyal companions"), 0o644)

	kb, _ := New(fakeEmbedder{}, filepath.Join(dir, ".simon_knowledge"))
	count, err := kb.Add(context.Background(), docsDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("expected chunks from both files")
	}

	hits, _ := kb.Search(context.Background(), "cats", 5)
	sources := map[string]bool{}
	for _, h := range hits {
		sources[filepath.Base(h.Source)] = true
	}
	if !sources["a.txt"] {
		t.Errorf("expected a.txt among search results, got %+v", hits)
	}
}

func TestAddReturnsErrorForMissingPath(t *testing.T) {
	kb, _ := New(fakeEmbedder{}, filepath.Join(t.TempDir(), ".simon_knowledge"))
	if _, err := kb.Add(context.Background(), "/nonexistent/path", false); err == nil {
		t.Error("expected an error for a missing path")
	}
}

func TestSearchReturnsNilWhenStoreDoesNotExist(t *testing.T) {
	kb, _ := New(fakeEmbedder{}, filepath.Join(t.TempDir(), "never-created"))
	// Force the index directory to not exist by removing what New() created.
	_ = os.RemoveAll(kb.storePath)

	hits, err := kb.Search(context.Background(), "anything", 3)
	if err != nil {
		t.Fatal(err)
	}
	if hits != nil {
		t.Errorf("expected nil hits, got %+v", hits)
	}
}

func TestChunkSplitsWithOverlap(t *testing.T) {
	kb := &KnowledgeBase{ChunkSize: 10, Overlap: 2}
	chunks := kb.chunk(strings.Repeat("a", 25))
	if len(chunks) < 2 {
		t.Fatalf("expected multiple overlapping chunks, got %v", chunks)
	}
	for _, c := range chunks {
		if len(c) > 10 {
			t.Errorf("chunk exceeds ChunkSize: %q", c)
		}
	}
}

func TestChunkReturnsNilForBlankText(t *testing.T) {
	kb := &KnowledgeBase{ChunkSize: 10, Overlap: 2}
	if got := kb.chunk("   \n\t  "); got != nil {
		t.Errorf("expected nil chunks for blank text, got %v", got)
	}
}
