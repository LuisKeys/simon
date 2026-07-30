package router

import "testing"

func loadTestCatalog(t *testing.T) *Catalog {
	t.Helper()
	catalog, issues, err := Load(Config{KnowledgePath: "testdata/knowledge_router"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, issue := range issues {
		if issue.Severity == SeverityError {
			t.Fatalf("unexpected load issue: %+v", issue)
		}
	}
	return catalog
}

func TestRouteCategoriesFindsNestedCategory(t *testing.T) {
	catalog := loadTestCatalog(t)
	q := Tokenize("PostgreSQL database worker queue")
	matches := routeCategories(catalog, q, 8, 3, 0.01)
	found := false
	for _, m := range matches {
		if m.Path == "software/databases" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected software/databases among category matches, got %+v", matches)
	}
}

func TestRouteCategoriesThreshold(t *testing.T) {
	catalog := loadTestCatalog(t)
	q := Tokenize("completely unrelated automotive topic")
	matches := routeCategories(catalog, q, 8, 3, 0.05)
	if len(matches) != 0 {
		t.Fatalf("expected no matches above threshold for unrelated query, got %+v", matches)
	}
}

func TestRouteCategoriesMaxLimit(t *testing.T) {
	catalog := loadTestCatalog(t)
	q := Tokenize("database software engineering")
	matches := routeCategories(catalog, q, 8, 1, 0.0)
	if len(matches) > 1 {
		t.Fatalf("expected at most 1 category, got %d", len(matches))
	}
}

func TestRouteDocumentsDeduplicatesAcrossCategories(t *testing.T) {
	catalog := loadTestCatalog(t)
	cats := []CategoryMatch{
		{Path: "software/databases", node: catalog.Categories["software/databases"]},
		{Path: "software", node: catalog.Categories["software"]},
	}
	// Attach the same document under both fixture categories to exercise
	// dedup, without mutating the shared testdata catalog on disk.
	catalog.Categories["software"].Documents = catalog.Categories["software/databases"].Documents

	q := Tokenize("PostgreSQL worker")
	docs := routeDocuments(cats, q, 5, 0.0)
	seen := map[string]bool{}
	for _, d := range docs {
		if seen[d.ID] {
			t.Fatalf("document %s appeared twice in results", d.ID)
		}
		seen[d.ID] = true
	}
}

func TestRouteSectionsRanksClaimingJobsHighest(t *testing.T) {
	catalog := loadTestCatalog(t)
	q := Tokenize("How do PostgreSQL workers claim jobs safely?")
	cats := routeCategories(catalog, q, 8, 3, 0.01)
	catScores := map[string]float64{}
	for _, c := range cats {
		catScores[c.Path] = c.Score
	}
	docs := routeDocuments(cats, q, 5, 0.0)
	sections := routeSections(docs, catScores, q, 5, 0.0)
	if len(sections) == 0 {
		t.Fatal("expected at least one section match")
	}
	if sections[0].SectionID != "claiming-jobs" {
		t.Fatalf("expected 'claiming-jobs' to rank first, got %q (all: %+v)", sections[0].SectionID, sections)
	}
}

func TestRouteSectionsSyntheticWholeDocument(t *testing.T) {
	docWithoutSections := &Document{
		Metadata:   DocumentMetadata{ID: "no-sections", Title: "No Sections Doc", Summary: "about widgets"},
		SourcePath: "irrelevant.md",
	}
	docs := []DocumentMatch{{ID: "no-sections", doc: docWithoutSections, Score: 1.0}}
	q := Tokenize("widgets")
	sections := routeSections(docs, nil, q, 5, 0.0)
	if len(sections) != 1 || sections[0].SectionID != syntheticSectionID {
		t.Fatalf("expected one synthetic section, got %+v", sections)
	}
}

func TestRouteSectionsMaxLimit(t *testing.T) {
	catalog := loadTestCatalog(t)
	q := Tokenize("PostgreSQL worker queue retry lease")
	cats := routeCategories(catalog, q, 8, 3, 0.0)
	docs := routeDocuments(cats, q, 5, 0.0)
	sections := routeSections(docs, nil, q, 1, 0.0)
	if len(sections) > 1 {
		t.Fatalf("expected at most 1 section, got %d", len(sections))
	}
}
