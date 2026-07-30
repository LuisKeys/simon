package router

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync/atomic"

	"simon-go/internal/agent/response"
)

// Router is Knowledge Router's public entry point. It satisfies
// agent.KnowledgeSearcher (via Search) without importing internal/agent, so
// it can be attached with agent.WithKnowledge(r) from CLI/example code that
// does import both packages.
type Router struct {
	cfg     Config
	catalog atomic.Pointer[Catalog]
}

// New loads the knowledge directory at cfg.KnowledgePath and returns a
// ready-to-use Router. Metadata-quality problems are not fatal here; call
// Validate afterward to inspect them, or check them via SearchDetailed's
// Explain output.
func New(cfg Config) (*Router, error) {
	cfg = cfg.withDefaults()
	if cfg.KnowledgePath == "" {
		return nil, wrapf(nil, "config.KnowledgePath is required")
	}
	catalog, _, err := Load(cfg)
	if err != nil {
		return nil, err
	}
	r := &Router{cfg: cfg}
	r.catalog.Store(catalog)
	return r, nil
}

// Reload rebuilds the catalog from disk and atomically replaces the one in
// use, so in-flight searches never observe a partially-loaded catalog.
func (r *Router) Reload(ctx context.Context) error {
	catalog, _, err := Load(r.cfg)
	if err != nil {
		return err
	}
	r.catalog.Store(catalog)
	return nil
}

// Validate re-runs full validation (load-time issues plus metadata-quality
// checks) against the currently loaded catalog.
func (r *Router) Validate(ctx context.Context) []ValidationIssue {
	_, loadIssues, err := Load(r.cfg)
	if err != nil {
		return []ValidationIssue{{Severity: SeverityError, Message: err.Error()}}
	}
	issues := append([]ValidationIssue{}, loadIssues...)
	issues = append(issues, Validate(r.catalog.Load(), r.cfg.Strict)...)
	return issues
}

// Catalog returns the currently loaded catalog.
func (r *Router) Catalog() *Catalog {
	return r.catalog.Load()
}

// Search implements agent.KnowledgeSearcher. A non-positive topK or an
// empty/whitespace-only query returns an empty result without error.
func (r *Router) Search(ctx context.Context, query string, topK int) ([]response.KnowledgeHit, error) {
	if topK <= 0 {
		return nil, nil
	}
	result, err := r.SearchDetailed(ctx, query, SearchOptions{IncludeContent: true})
	if err != nil {
		return nil, err
	}
	hits := make([]response.KnowledgeHit, 0, len(result.Evidence))
	for _, ev := range result.Evidence {
		score := float32(ev.Score)
		if math.IsNaN(float64(score)) || math.IsInf(float64(score), 0) {
			score = 0
		}
		hits = append(hits, response.KnowledgeHit{
			Text:   ev.Content,
			Source: formatSource(ev),
			Score:  score,
		})
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if topK < len(hits) {
		hits = hits[:topK]
	}
	return hits, nil
}

// SearchDetailed runs the full category -> document -> section -> evidence
// pipeline and returns every stage's ranked candidates, powering CLI
// explain mode, debugging, and tests.
func (r *Router) SearchDetailed(ctx context.Context, query string, options SearchOptions) (*SearchResult, error) {
	q := Tokenize(query)
	result := &SearchResult{Query: query}
	if len(q.Tokens) == 0 {
		return result, nil
	}

	opts := options.resolved(r.cfg)
	catalog := r.catalog.Load()

	categories := routeCategories(catalog, q, r.cfg.MaxDepth, opts.MaxCategories, opts.MinCategoryScore)
	result.Categories = categories

	categoryScoreByPath := make(map[string]float64, len(categories))
	for _, c := range categories {
		categoryScoreByPath[c.Path] = c.Score
	}

	documents := routeDocuments(categories, q, opts.MaxDocuments, opts.MinDocumentScore)
	result.Documents = documents

	sections := routeSections(documents, categoryScoreByPath, q, opts.MaxSections, opts.MinSectionScore)
	result.Sections = sections

	if !opts.IncludeContent {
		return result, nil
	}

	cache := newExtractionCache()
	evidence := make([]EvidenceFragment, 0, len(sections))
	for _, sec := range sections {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		frag, err := retrieveEvidence(cache, sec, r.cfg.MaxEvidenceCharacters)
		if err != nil {
			continue
		}
		evidence = append(evidence, frag)
	}
	sort.SliceStable(evidence, func(i, j int) bool { return evidence[i].Score > evidence[j].Score })
	result.Evidence = evidence

	return result, nil
}

// formatSource renders an evidence fragment's source reference as
// "/path/document.md#section-id:L21-L55" (line suffix omitted when the
// section has no line range).
func formatSource(ev EvidenceFragment) string {
	base := fmt.Sprintf("%s#%s", ev.SourcePath, ev.SectionID)
	if ev.StartLine > 0 && ev.EndLine > 0 {
		return fmt.Sprintf("%s:L%d-L%d", base, ev.StartLine, ev.EndLine)
	}
	return base
}
