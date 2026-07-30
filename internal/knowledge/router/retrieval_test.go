package router

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func testDocument(t *testing.T, sourcePath string) *Document {
	t.Helper()
	return &Document{
		Metadata:   DocumentMetadata{ID: "doc", Title: "Doc"},
		SourcePath: sourcePath,
	}
}

func TestRetrieveEvidenceValidLineRange(t *testing.T) {
	doc := testDocument(t, "testdata/knowledge_router/software/databases/postgres-workers.md")
	m := SectionMatch{
		DocumentID: "postgres-workers", SectionID: "claiming-jobs", Title: "Claiming Jobs",
		section: SectionMetadata{ID: "claiming-jobs", StartLine: 20, EndLine: 41}, document: doc,
	}
	cache := newExtractionCache()
	frag, err := retrieveEvidence(cache, m, 8000)
	if err != nil {
		t.Fatalf("retrieveEvidence: %v", err)
	}
	if !strings.Contains(frag.Content, "SKIP LOCKED") {
		t.Fatalf("expected evidence to contain 'SKIP LOCKED', got: %s", frag.Content)
	}
	if frag.StartLine != 20 || frag.EndLine != 41 {
		t.Fatalf("expected line range [20,41], got [%d,%d]", frag.StartLine, frag.EndLine)
	}
}

func TestRetrieveEvidenceOutOfRange(t *testing.T) {
	doc := testDocument(t, "testdata/knowledge_router/software/databases/postgres-workers.md")
	m := SectionMatch{
		document: doc,
		section:  SectionMetadata{ID: "bad", StartLine: 9000, EndLine: 9010},
	}
	cache := newExtractionCache()
	if _, err := retrieveEvidence(cache, m, 8000); err == nil {
		t.Fatal("expected an error for an out-of-range line range")
	}
}

func TestRetrieveEvidenceTruncation(t *testing.T) {
	doc := testDocument(t, "testdata/knowledge_router/software/databases/postgres-workers.md")
	m := SectionMatch{
		document: doc,
		section:  SectionMetadata{ID: "intro", StartLine: 1, EndLine: 19},
	}
	cache := newExtractionCache()
	frag, err := retrieveEvidence(cache, m, 20)
	if err != nil {
		t.Fatalf("retrieveEvidence: %v", err)
	}
	if !frag.Truncated {
		t.Fatal("expected fragment to be marked truncated")
	}
	if len(frag.Content) != 20 {
		t.Fatalf("expected content capped at 20 chars, got %d", len(frag.Content))
	}
}

func TestRetrieveEvidencePerRequestCaching(t *testing.T) {
	doc := testDocument(t, "testdata/knowledge_router/software/databases/postgres-workers.md")
	cache := newExtractionCache()
	m1 := SectionMatch{document: doc, section: SectionMetadata{ID: "a", StartLine: 1, EndLine: 5}}
	m2 := SectionMatch{document: doc, section: SectionMetadata{ID: "b", StartLine: 20, EndLine: 25}}

	if _, err := retrieveEvidence(cache, m1, 8000); err != nil {
		t.Fatalf("retrieveEvidence: %v", err)
	}
	if len(cache.cache) != 1 {
		t.Fatalf("expected 1 cached document, got %d", len(cache.cache))
	}
	if _, err := retrieveEvidence(cache, m2, 8000); err != nil {
		t.Fatalf("retrieveEvidence: %v", err)
	}
	if len(cache.cache) != 1 {
		t.Fatalf("expected extraction to still be cached (1 entry), got %d", len(cache.cache))
	}
}

func TestRetrieveEvidenceUnsupportedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.md")
	doc := testDocument(t, path)
	m := SectionMatch{document: doc, section: SectionMetadata{ID: "x"}}
	cache := newExtractionCache()
	if _, err := retrieveEvidence(cache, m, 8000); err == nil {
		t.Fatal("expected an error for a missing/unsupported source file")
	}
}

func TestRetrieveEvidenceSyntheticDocumentSection(t *testing.T) {
	doc := testDocument(t, "testdata/knowledge_router/software/databases/postgres-workers.md")
	m := SectionMatch{document: doc, section: SectionMetadata{ID: syntheticSectionID}}
	cache := newExtractionCache()
	frag, err := retrieveEvidence(cache, m, 8000)
	if err != nil {
		t.Fatalf("retrieveEvidence: %v", err)
	}
	if frag.Content == "" {
		t.Fatal("expected non-empty synthetic section content")
	}
	if frag.StartLine != 0 || frag.EndLine != 0 {
		t.Fatalf("synthetic section should have no line range, got [%d,%d]", frag.StartLine, frag.EndLine)
	}
}

func TestRetrieveEvidenceContextCancellation(t *testing.T) {
	catalog, _, err := Load(Config{KnowledgePath: "testdata/knowledge_router"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := &Router{cfg: Config{MaxDepth: 8, MaxCategories: 3, MaxDocuments: 5, MaxSections: 5, MaxEvidenceCharacters: 8000}}
	r.catalog.Store(catalog)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := r.SearchDetailed(ctx, "PostgreSQL worker queue claim jobs", SearchOptions{IncludeContent: true})
	if err == nil {
		t.Fatalf("expected context cancellation error, got result: %+v", result)
	}
}
