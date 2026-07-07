package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAddSourceAndSearchRanksByScore(t *testing.T) {
	dir := t.TempDir()
	idx, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	chunks := []Chunk{{Text: "cats are great", Source: "a.txt"}, {Text: "dogs are great", Source: "a.txt"}}
	vectors := [][]float32{{1, 0}, {0, 1}}
	if err := idx.AddSource(context.Background(), "a.txt", chunks, vectors, false); err != nil {
		t.Fatal(err)
	}

	results, err := idx.Search(context.Background(), []float32{1, 0}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Text != "cats are great" {
		t.Errorf("expected the closer vector ranked first, got %+v", results)
	}
}

func TestAddSourceSkipsExistingUnlessForced(t *testing.T) {
	dir := t.TempDir()
	idx, _ := Open(dir)

	_ = idx.AddSource(context.Background(), "a.txt", []Chunk{{Text: "v1", Source: "a.txt"}}, [][]float32{{1, 0}}, false)
	if !idx.HasSource("a.txt") {
		t.Fatal("expected source to be indexed")
	}

	// No-op without force.
	_ = idx.AddSource(context.Background(), "a.txt", []Chunk{{Text: "v2", Source: "a.txt"}}, [][]float32{{0, 1}}, false)
	results, _ := idx.Search(context.Background(), []float32{1, 0}, 1)
	if results[0].Text != "v1" {
		t.Errorf("expected original content preserved without force, got %+v", results)
	}

	// Replaces with force.
	_ = idx.AddSource(context.Background(), "a.txt", []Chunk{{Text: "v2", Source: "a.txt"}}, [][]float32{{0, 1}}, true)
	results, _ = idx.Search(context.Background(), []float32{0, 1}, 1)
	if results[0].Text != "v2" {
		t.Errorf("expected forced replacement, got %+v", results)
	}
}

func TestAddSourceRejectsDimensionMismatch(t *testing.T) {
	dir := t.TempDir()
	idx, _ := Open(dir)
	_ = idx.AddSource(context.Background(), "a.txt", []Chunk{{Text: "x", Source: "a.txt"}}, [][]float32{{1, 0, 0}}, false)

	err := idx.AddSource(context.Background(), "b.txt", []Chunk{{Text: "y", Source: "b.txt"}}, [][]float32{{1, 0}}, false)
	if err == nil {
		t.Fatal("expected a dimension mismatch error")
	}
}

func TestPersistsAndReloadsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	idx1, _ := Open(dir)
	_ = idx1.AddSource(context.Background(), "a.txt", []Chunk{{Text: "hello", Source: "a.txt"}}, [][]float32{{0.6, 0.8}}, false)

	if _, err := os.Stat(filepath.Join(dir, manifestFile)); err != nil {
		t.Fatalf("expected manifest.json to be written, got: %v", err)
	}

	idx2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !idx2.HasSource("a.txt") {
		t.Fatal("expected source to survive reload")
	}
	results, _ := idx2.Search(context.Background(), []float32{0.6, 0.8}, 1)
	if len(results) != 1 || results[0].Text != "hello" {
		t.Errorf("expected reloaded chunk data, got %+v", results)
	}
}

func TestSearchReturnsNilOnEmptyIndex(t *testing.T) {
	idx, _ := Open(t.TempDir())
	results, err := idx.Search(context.Background(), []float32{1, 0}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Errorf("expected nil results on empty index, got %+v", results)
	}
}

func TestSIDXRoundTripsFloat32Precisely(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sidx")
	vectors := [][]float32{{0.123456, -0.987654, 3.14159}, {1e10, -1e-10, 0}}

	if err := writeSIDX(path, vectors); err != nil {
		t.Fatal(err)
	}
	got, err := readSIDX(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0][0] != vectors[0][0] || got[1][0] != vectors[1][0] {
		t.Errorf("round trip mismatch: got %v, want %v", got, vectors)
	}
}
