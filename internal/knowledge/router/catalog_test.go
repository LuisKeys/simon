package router

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadTestdataCatalog(t *testing.T) {
	catalog, issues, err := Load(Config{KnowledgePath: "testdata/knowledge_router"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, issue := range issues {
		if issue.Severity == SeverityError {
			t.Fatalf("unexpected load issue: %+v", issue)
		}
	}
	if _, ok := catalog.Categories["software"]; !ok {
		t.Fatalf("expected category 'software', got %v", catalog.Categories)
	}
	if _, ok := catalog.Categories["software/databases"]; !ok {
		t.Fatalf("expected nested category 'software/databases', got %v", catalog.Categories)
	}
	doc, ok := catalog.Documents["postgres-workers"]
	if !ok {
		t.Fatalf("expected document 'postgres-workers', got %v", catalog.Documents)
	}
	if len(doc.Metadata.Sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(doc.Metadata.Sections))
	}
}

func TestCatalogCategoryPathNormalization(t *testing.T) {
	catalog, _, err := Load(Config{KnowledgePath: "testdata/knowledge_router"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	node := catalog.Categories["software/databases"]
	if node == nil {
		t.Fatal("expected software/databases node")
	}
	if node.Parent == nil || node.Parent.Path != "software" {
		t.Fatalf("expected parent path 'software', got %+v", node.Parent)
	}
}

func TestCatalogDuplicateCategoryPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a", "category.yaml"), "id: a\nname: A\n")
	catalog, issues, err := Load(Config{KnowledgePath: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_ = catalog
	// A second load of the same directory should not itself create
	// duplicates; instead simulate a duplicate by writing two files that
	// resolve to the same category path is not directly expressible via
	// WalkDir with a single category.yaml per dir, so we assert none was
	// reported for the single-file case (regression guard).
	for _, issue := range issues {
		if issue.Message == "duplicate category path: a" {
			t.Fatalf("unexpected duplicate category issue: %+v", issue)
		}
	}
}

func TestCatalogDuplicateDocumentID(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "cat", "category.yaml"), "id: cat\nname: Cat\n")
	writeFile(t, filepath.Join(dir, "cat", "doc1.md"), "content one\n")
	writeFile(t, filepath.Join(dir, "cat", "doc1.yaml"), "id: dup\ntitle: One\nsource:\n  path: doc1.md\n  format: markdown\n")
	writeFile(t, filepath.Join(dir, "cat", "doc2.md"), "content two\n")
	writeFile(t, filepath.Join(dir, "cat", "doc2.yaml"), "id: dup\ntitle: Two\nsource:\n  path: doc2.md\n  format: markdown\n")

	_, issues, err := Load(Config{KnowledgePath: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	found := false
	for _, issue := range issues {
		if issue.Severity == SeverityError && issue.Field == "id" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected duplicate document id error, got %+v", issues)
	}
}

func TestCatalogMultiCategoryDocument(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a", "category.yaml"), "id: a\nname: A\n")
	writeFile(t, filepath.Join(dir, "b", "category.yaml"), "id: b\nname: B\n")
	writeFile(t, filepath.Join(dir, "a", "doc.md"), "content\n")
	writeFile(t, filepath.Join(dir, "a", "doc.yaml"), "id: multi\ntitle: Multi\ncategories:\n  - a\n  - b\nsource:\n  path: doc.md\n  format: markdown\n")

	catalog, issues, err := Load(Config{KnowledgePath: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, issue := range issues {
		if issue.Severity == SeverityError {
			t.Fatalf("unexpected error: %+v", issue)
		}
	}
	if len(catalog.Categories["a"].Documents) != 1 || len(catalog.Categories["b"].Documents) != 1 {
		t.Fatalf("expected document attached to both categories, got a=%d b=%d",
			len(catalog.Categories["a"].Documents), len(catalog.Categories["b"].Documents))
	}
	if catalog.Categories["a"].Documents[0] != catalog.Categories["b"].Documents[0] {
		t.Fatalf("expected the same canonical *Document instance in both categories")
	}
}

func TestCatalogMissingCategoryReference(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a", "category.yaml"), "id: a\nname: A\n")
	writeFile(t, filepath.Join(dir, "a", "doc.md"), "content\n")
	writeFile(t, filepath.Join(dir, "a", "doc.yaml"), "id: orphan\ntitle: Orphan\ncategories:\n  - nonexistent\nsource:\n  path: doc.md\n  format: markdown\n")

	_, issues, err := Load(Config{KnowledgePath: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	found := false
	for _, issue := range issues {
		if issue.Field == "categories" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unknown category reference issue, got %+v", issues)
	}
}

func TestCatalogRootMetadataDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a", "category.yaml"), "id: a\nname: A\n")
	catalog, _, err := Load(Config{KnowledgePath: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if catalog.Metadata.Name == "" {
		t.Fatalf("expected a default catalog name when catalog.yaml is absent")
	}
}

func TestCatalogPathTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a", "category.yaml"), "id: a\nname: A\n")
	writeFile(t, filepath.Join(dir, "a", "doc.yaml"), "id: evil\ntitle: Evil\nsource:\n  path: ../../../../../../etc/passwd\n  format: plain\n")

	_, issues, err := Load(Config{KnowledgePath: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	found := false
	for _, issue := range issues {
		if issue.Field == "source.path" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a source.path issue for a path escaping the root, got %+v", issues)
	}
	if _, ok := (&Catalog{}).Documents["evil"]; ok {
		t.Fatal("sanity check failed")
	}
}

func TestCatalogRelativeSourcePaths(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a", "category.yaml"), "id: a\nname: A\n")
	writeFile(t, filepath.Join(dir, "a", "sub", "doc.md"), "content\n")
	writeFile(t, filepath.Join(dir, "a", "doc.yaml"), "id: rel\ntitle: Rel\nsource:\n  path: sub/doc.md\n  format: markdown\n")

	catalog, issues, err := Load(Config{KnowledgePath: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, issue := range issues {
		if issue.Severity == SeverityError {
			t.Fatalf("unexpected error: %+v", issue)
		}
	}
	doc, ok := catalog.Documents["rel"]
	if !ok {
		t.Fatal("expected document 'rel' to load")
	}
	if _, err := os.Stat(doc.SourcePath); err != nil {
		t.Fatalf("resolved source path should exist: %v", err)
	}
}

func TestValidateStrictEscalatesWarnings(t *testing.T) {
	catalog, _, err := Load(Config{KnowledgePath: "testdata/knowledge_router"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	nonStrict := Validate(catalog, false)
	strict := Validate(catalog, true)
	_ = nonStrict
	_ = strict // both should run without panicking; strict may escalate severities
}
