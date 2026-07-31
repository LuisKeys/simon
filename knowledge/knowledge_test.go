package knowledge_test

import (
	"context"
	"os"
	"testing"

	"github.com/LuisKeys/simon/knowledge"
)

// fakeEmbedder returns a fixed-dimension vector derived from text length,
// just distinct enough to exercise Open/Add/Search without a real
// embedding provider or network access.
type fakeEmbedder struct{}

func (fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	return []float32{float32(len(text)), 1, 0}, nil
}

func (fakeEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i], _ = fakeEmbedder{}.Embed(ctx, t)
	}
	return out, nil
}

func TestOpenAddSearchRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := knowledge.Open(fakeEmbedder{}, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	docPath := dir + "/doc.txt"
	if err := os.WriteFile(docPath, []byte("Simon is a Go SDK for building agents."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := store.Add(context.Background(), docPath, knowledge.AddOptions{})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if result.ChunksAdded == 0 {
		t.Fatal("expected at least one chunk added")
	}

	hits, err := store.Search(context.Background(), "Simon SDK", 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least one search hit")
	}
}

func TestRemoveReturnsNotSupportedError(t *testing.T) {
	dir := t.TempDir()
	store, err := knowledge.Open(fakeEmbedder{}, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	if err := store.Remove(context.Background(), "anything"); err == nil {
		t.Error("expected Remove to return a not-supported error")
	}
}
