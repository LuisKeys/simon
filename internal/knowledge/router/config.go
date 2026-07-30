// Package router implements Knowledge Router: a hierarchical, lexical,
// embeddings-free retrieval backend that coexists with Simon's vector
// KnowledgeBase (internal/knowledge). Rather than searching every document
// fragment directly, it routes a query through categories -> documents ->
// sections before reading only the selected source regions. This package
// intentionally depends only on internal/knowledge/extract,
// internal/agent/response, pkg/simonerr, and the standard library (plus
// gopkg.in/yaml.v3) so it can be attached to the agent core purely through
// the agent.KnowledgeSearcher interface, without internal/agent ever
// importing this package.
package router

// Config configures a Router's construction: where the knowledge directory
// lives, and the default limits/thresholds applied to every search unless
// overridden per-call via SearchOptions.
type Config struct {
	KnowledgePath string
	Strict        bool
	MaxDepth      int

	MaxCategories int
	MaxDocuments  int
	MaxSections   int

	MinCategoryScore float64
	MinDocumentScore float64
	MinSectionScore  float64

	// MaxEvidenceCharacters bounds how much source text a single evidence
	// fragment may contain. Zero uses the default (8000).
	MaxEvidenceCharacters int
}

// withDefaults returns a copy of c with zero-valued fields replaced by the
// recommended MVP defaults.
func (c Config) withDefaults() Config {
	if c.MaxDepth <= 0 {
		c.MaxDepth = 8
	}
	if c.MaxCategories <= 0 {
		c.MaxCategories = 3
	}
	if c.MaxDocuments <= 0 {
		c.MaxDocuments = 5
	}
	if c.MaxSections <= 0 {
		c.MaxSections = 5
	}
	if c.MinCategoryScore <= 0 {
		c.MinCategoryScore = 0.05
	}
	if c.MinDocumentScore <= 0 {
		c.MinDocumentScore = 0.05
	}
	if c.MinSectionScore <= 0 {
		c.MinSectionScore = 0.05
	}
	if c.MaxEvidenceCharacters <= 0 {
		c.MaxEvidenceCharacters = 8000
	}
	return c
}
