package router

import "sort"

// CategoryMatch is one category selected by routeCategories, with its score
// and explanation.
type CategoryMatch struct {
	ID      string   `json:"id"`
	Path    string   `json:"path"`
	Name    string   `json:"name"`
	Score   float64  `json:"score"`
	Reasons []string `json:"reasons,omitempty"`

	node *CategoryNode
}

// routeCategories performs progressive, depth-first category routing: score
// the current frontier (starting at the root's children), descend into the
// children of the best maxCategories candidates at each level, and repeat
// until the frontier is empty (no more children) or maxDepth is reached. A
// candidate is added to the returned selection only once its own score
// clears minScore, independent of whether its ancestors did (a navigational
// parent may share little vocabulary with a query that is really about one
// of its children — descent is not gated on the parent's score). Categories
// from different depths can remain selected simultaneously; a small
// ancestor bonus is added when a child's parent was also selected.
func routeCategories(catalog *Catalog, q Query, maxDepth, maxCategories int, minScore float64) []CategoryMatch {
	var selected []CategoryMatch
	selectedByPath := map[string]bool{}

	frontier := catalog.Root.Children
	for depth := 0; depth < maxDepth && len(frontier) > 0; depth++ {
		type scored struct {
			node  *CategoryNode
			score Score
		}
		var candidates []scored
		for _, node := range frontier {
			s := scoreCategory(q, node)
			if node.Parent != nil && selectedByPath[node.Parent.Path] {
				s.Value += 0.02
				s.Reasons = append(s.Reasons, "parent category also matched")
			}
			candidates = append(candidates, scored{node, s})
		}

		sort.SliceStable(candidates, func(i, j int) bool {
			return candidates[i].score.Value > candidates[j].score.Value
		})

		// Descend into the children of the top maxCategories candidates
		// regardless of whether the parent itself cleared minScore: a
		// navigational parent category (e.g. "software") legitimately may
		// share little vocabulary with a query that is really about one of
		// its children (e.g. "software/databases"). Selection into the
		// final result, below, still requires clearing minScore. Descent
		// stops once the best remaining candidate carries no lexical
		// signal at all, so irrelevant subtrees are not explored forever.
		var nextFrontier []*CategoryNode
		for i, c := range candidates {
			if i >= maxCategories {
				break
			}
			if c.score.Value >= minScore && !selectedByPath[c.node.Path] {
				selected = append(selected, CategoryMatch{
					ID: c.node.Metadata.ID, Path: c.node.Path, Name: c.node.Metadata.Name,
					Score: c.score.Value, Reasons: c.score.Reasons, node: c.node,
				})
				selectedByPath[c.node.Path] = true
			}
			nextFrontier = append(nextFrontier, c.node.Children...)
		}
		frontier = nextFrontier
	}

	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].Score != selected[j].Score {
			return selected[i].Score > selected[j].Score
		}
		return selected[i].Path < selected[j].Path
	})
	if len(selected) > maxCategories && maxCategories > 0 {
		selected = selected[:maxCategories]
	}
	return selected
}

func scoreCategory(q Query, node *CategoryNode) Score {
	return scoreFields(q, Fields{
		Title:         node.Metadata.Name,
		Description:   node.Metadata.Description,
		Summary:       node.Metadata.Summary,
		Keywords:      node.Metadata.Keywords,
		Aliases:       node.Metadata.Aliases,
		CategoryPaths: []string{node.Path},
	})
}
