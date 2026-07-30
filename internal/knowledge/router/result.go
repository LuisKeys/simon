package router

// SearchResult is the full, explainable output of SearchDetailed: every
// stage's ranked candidates plus the retrieved evidence.
type SearchResult struct {
	Query      string             `json:"query"`
	Categories []CategoryMatch    `json:"categories"`
	Documents  []DocumentMatch    `json:"documents"`
	Sections   []SectionMatch     `json:"sections"`
	Evidence   []EvidenceFragment `json:"evidence"`
}

// SearchOptions overrides a Router's constructor-level defaults for a
// single call to SearchDetailed.
type SearchOptions struct {
	MaxCategories int
	MaxDocuments  int
	MaxSections   int

	MinCategoryScore float64
	MinDocumentScore float64
	MinSectionScore  float64

	IncludeContent bool
	Explain        bool
}

// resolved merges non-zero fields of o onto cfg's defaults.
func (o SearchOptions) resolved(cfg Config) SearchOptions {
	r := SearchOptions{
		MaxCategories:    cfg.MaxCategories,
		MaxDocuments:     cfg.MaxDocuments,
		MaxSections:      cfg.MaxSections,
		MinCategoryScore: cfg.MinCategoryScore,
		MinDocumentScore: cfg.MinDocumentScore,
		MinSectionScore:  cfg.MinSectionScore,
		IncludeContent:   true,
	}
	if o.MaxCategories > 0 {
		r.MaxCategories = o.MaxCategories
	}
	if o.MaxDocuments > 0 {
		r.MaxDocuments = o.MaxDocuments
	}
	if o.MaxSections > 0 {
		r.MaxSections = o.MaxSections
	}
	if o.MinCategoryScore > 0 {
		r.MinCategoryScore = o.MinCategoryScore
	}
	if o.MinDocumentScore > 0 {
		r.MinDocumentScore = o.MinDocumentScore
	}
	if o.MinSectionScore > 0 {
		r.MinSectionScore = o.MinSectionScore
	}
	r.Explain = o.Explain
	return r
}
