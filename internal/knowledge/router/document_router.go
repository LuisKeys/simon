package router

import (
	"path/filepath"
	"sort"
)

// DocumentMatch is one document selected by routeDocuments.
type DocumentMatch struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	SourcePath string   `json:"source_path"`
	Score      float64  `json:"score"`
	Reasons    []string `json:"reasons,omitempty"`

	doc *Document
}

// routeDocuments builds the document candidate set from the selected
// categories (deduplicated across categories), scores each document's
// metadata independently, adds a small bonus per matching selected
// category, and keeps the best maxDocuments above minScore. Source content
// is never read here.
func routeDocuments(categories []CategoryMatch, q Query, maxDocuments int, minScore float64) []DocumentMatch {
	catBonus := map[string][]string{} // documentID -> matched category paths
	docs := map[string]*Document{}
	for _, cat := range categories {
		if cat.node == nil {
			continue
		}
		for _, doc := range cat.node.Documents {
			docs[doc.Metadata.ID] = doc
			catBonus[doc.Metadata.ID] = append(catBonus[doc.Metadata.ID], cat.Path)
		}
	}

	var matches []DocumentMatch
	for id, doc := range docs {
		s := scoreDocument(q, doc)
		for range catBonus[id] {
			s.Value += weightCategoryPath * 0.1
		}
		if len(catBonus[id]) > 0 {
			s.Reasons = append(s.Reasons, "belongs to a matched category")
		}
		if s.Value < minScore {
			continue
		}
		matches = append(matches, DocumentMatch{
			ID: doc.Metadata.ID, Title: doc.Metadata.Title, SourcePath: doc.SourcePath,
			Score: s.Value, Reasons: s.Reasons, doc: doc,
		})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].ID < matches[j].ID
	})
	if maxDocuments > 0 && len(matches) > maxDocuments {
		matches = matches[:maxDocuments]
	}
	return matches
}

func scoreDocument(q Query, doc *Document) Score {
	m := doc.Metadata
	return scoreFields(q, Fields{
		Title:         m.Title,
		Description:   m.Description,
		Summary:       m.Summary,
		Keywords:      m.Keywords,
		Aliases:       m.Aliases,
		CategoryPaths: m.Categories,
		General:       []string{filepath.Base(doc.SourcePath)},
	})
}
