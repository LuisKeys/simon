package router

import (
	"context"
	"strings"
	"testing"

	"github.com/LuisKeys/simon/internal/agent/response"
)

// knowledgeSearcher mirrors agent.KnowledgeSearcher's exact method set. It
// is duplicated here, rather than importing internal/agent, to keep this
// package's dependency graph free of any agent-core import while still
// verifying Router satisfies the contract at compile time.
type knowledgeSearcher interface {
	Search(ctx context.Context, query string, topK int) ([]response.KnowledgeHit, error)
}

var _ knowledgeSearcher = (*Router)(nil)

func newTestRouter(t *testing.T) *Router {
	t.Helper()
	r, err := New(Config{KnowledgePath: "testdata/knowledge_router"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

func TestRouterSearchIntegration(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()

	result, err := r.SearchDetailed(ctx, "How can workers claim PostgreSQL jobs without collisions?", SearchOptions{IncludeContent: true, Explain: true})
	if err != nil {
		t.Fatalf("SearchDetailed: %v", err)
	}

	if !containsPath(result.Categories, "software/databases") {
		t.Fatalf("expected software/databases category, got %+v", result.Categories)
	}
	if !containsDocID(result.Documents, "postgres-workers") {
		t.Fatalf("expected postgres-workers document, got %+v", result.Documents)
	}
	if !containsSectionID(result.Sections, "claiming-jobs") {
		t.Fatalf("expected claiming-jobs section, got %+v", result.Sections)
	}

	var claimingJobs *EvidenceFragment
	for i := range result.Evidence {
		if result.Evidence[i].SectionID == "claiming-jobs" {
			claimingJobs = &result.Evidence[i]
		}
	}
	if claimingJobs == nil {
		t.Fatalf("expected evidence for the claiming-jobs section, got %+v", result.Evidence)
	}
	if !strings.Contains(claimingJobs.Content, "SKIP LOCKED") {
		t.Fatalf("expected claiming-jobs evidence to contain 'SKIP LOCKED', got: %s", claimingJobs.Content)
	}
	if claimingJobs.StartLine == 0 || claimingJobs.EndLine == 0 {
		t.Fatalf("expected evidence to carry a line range, got %+v", claimingJobs)
	}
}

func TestRouterSearchSatisfiesKnowledgeSearcher(t *testing.T) {
	r := newTestRouter(t)
	hits, err := r.Search(context.Background(), "How do PostgreSQL workers claim jobs safely?", 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least one knowledge hit")
	}
	for _, h := range hits {
		if h.Text == "" {
			t.Fatal("hit text must never be empty")
		}
		if !strings.Contains(h.Source, "postgres-workers.md#") {
			t.Fatalf("expected source reference with section anchor, got %q", h.Source)
		}
	}
	for i := 1; i < len(hits); i++ {
		if hits[i].Score > hits[i-1].Score {
			t.Fatalf("hits must be sorted descending by score: %+v", hits)
		}
	}
}

func TestRouterSearchNonPositiveTopK(t *testing.T) {
	r := newTestRouter(t)
	hits, err := r.Search(context.Background(), "PostgreSQL worker queue", 0)
	if err != nil || hits != nil {
		t.Fatalf("expected (nil, nil) for non-positive topK, got (%v, %v)", hits, err)
	}
}

func TestRouterSearchEmptyQuery(t *testing.T) {
	r := newTestRouter(t)
	hits, err := r.Search(context.Background(), "   ", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected no hits for a blank query, got %+v", hits)
	}
}

func TestRouterReload(t *testing.T) {
	r := newTestRouter(t)
	before := r.Catalog()
	if err := r.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	after := r.Catalog()
	if before == after {
		t.Fatal("expected Reload to atomically replace the catalog pointer")
	}
	if len(after.Documents) != len(before.Documents) {
		t.Fatalf("expected the same document count after reload, got %d vs %d", len(after.Documents), len(before.Documents))
	}
}

func TestRouterValidate(t *testing.T) {
	r := newTestRouter(t)
	issues := r.Validate(context.Background())
	for _, issue := range issues {
		if issue.Severity == SeverityError {
			t.Fatalf("unexpected validation error on clean fixture: %+v", issue)
		}
	}
}

func containsPath(matches []CategoryMatch, path string) bool {
	for _, m := range matches {
		if m.Path == path {
			return true
		}
	}
	return false
}

func containsDocID(matches []DocumentMatch, id string) bool {
	for _, m := range matches {
		if m.ID == id {
			return true
		}
	}
	return false
}

func containsSectionID(matches []SectionMatch, id string) bool {
	for _, m := range matches {
		if m.SectionID == id {
			return true
		}
	}
	return false
}
