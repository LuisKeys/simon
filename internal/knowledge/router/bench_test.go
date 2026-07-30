package router

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// syntheticCorpus builds n one-category, one-section documents under dir so
// benchmarks can exercise catalog loading and search at scale without
// committing thousands of fixture files to the repo.
func syntheticCorpus(b *testing.B, dir string, n int) {
	b.Helper()
	if err := os.WriteFile(filepath.Join(dir, "category.yaml"), []byte("id: bench\nname: Bench\ndescription: Synthetic benchmark category.\nsummary: Synthetic documents for benchmarking.\n"), 0o644); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < n; i++ {
		base := fmt.Sprintf("doc-%d", i)
		content := fmt.Sprintf("# Document %d\n\nThis document is about topic number %d and covers benchmarking details.\n", i, i)
		if err := os.WriteFile(filepath.Join(dir, base+".md"), []byte(content), 0o644); err != nil {
			b.Fatal(err)
		}
		meta := fmt.Sprintf(`id: %s
title: Document %d
description: Synthetic document %d for benchmarking search over a large catalog.
summary: Covers topic %d.
source:
  path: %s.md
  format: markdown
`, base, i, i, i, base)
		if err := os.WriteFile(filepath.Join(dir, base+".yaml"), []byte(meta), 0o644); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCatalogLoading(b *testing.B) {
	dir := b.TempDir()
	syntheticCorpus(b, dir, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := Load(Config{KnowledgePath: dir}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDocumentScoring(b *testing.B) {
	dir := b.TempDir()
	syntheticCorpus(b, dir, 1000)
	catalog, _, err := Load(Config{KnowledgePath: dir})
	if err != nil {
		b.Fatal(err)
	}
	docs := make([]*Document, 0, len(catalog.Documents))
	for _, d := range catalog.Documents {
		docs = append(docs, d)
	}
	q := Tokenize("topic number benchmarking")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, d := range docs {
			scoreDocument(q, d)
		}
	}
}

func BenchmarkSectionScoring(b *testing.B) {
	doc := &Document{Metadata: DocumentMetadata{Title: "Doc", Summary: "Covers benchmarking topics."}}
	sec := SectionMetadata{Title: "Section", Summary: "Discusses benchmarking in detail."}
	q := Tokenize("benchmarking topics detail")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scoreSection(q, doc, sec)
	}
}

func BenchmarkSearchOver1000SyntheticDocuments(b *testing.B) {
	dir := b.TempDir()
	syntheticCorpus(b, dir, 1000)
	r, err := New(Config{KnowledgePath: dir})
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.Search(ctx, "topic number benchmarking", 5); err != nil {
			b.Fatal(err)
		}
	}
}
