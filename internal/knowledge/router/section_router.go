package router

import "sort"

// bonus weights combining a section's own score with its parent document's
// and categories' scores, per the spec's suggested combination formula.
const (
	sectionDocumentWeight = 0.35
	sectionCategoryWeight = 0.15
)

// SectionMatch is one section selected by routeSections.
type SectionMatch struct {
	DocumentID string   `json:"document_id"`
	SectionID  string   `json:"section_id"`
	Title      string   `json:"title"`
	Score      float64  `json:"score"`
	Reasons    []string `json:"reasons,omitempty"`

	section  SectionMetadata
	document *Document
}

// routeSections scores every section (or, for documents with no declared
// sections, one synthetic whole-document section) belonging to the
// candidate documents, combining each section's own lexical score with a
// weighted share of its document's and categories' scores.
func routeSections(documents []DocumentMatch, categoryScoreByPath map[string]float64, q Query, maxSections int, minScore float64) []SectionMatch {
	var matches []SectionMatch

	for _, docMatch := range documents {
		doc := docMatch.doc
		categoryBonus := 0.0
		for _, catPath := range doc.Metadata.Categories {
			if s, ok := categoryScoreByPath[catPath]; ok && s > categoryBonus {
				categoryBonus = s
			}
		}

		sections := doc.Metadata.Sections
		if len(sections) == 0 {
			sections = []SectionMetadata{{ID: syntheticSectionID, Title: doc.Metadata.Title, Summary: doc.Metadata.Summary}}
		}

		for _, sec := range sections {
			s := scoreSection(q, doc, sec)
			final := s.Value + docMatch.Score*sectionDocumentWeight + categoryBonus*sectionCategoryWeight
			if final < minScore {
				continue
			}
			matches = append(matches, SectionMatch{
				DocumentID: doc.Metadata.ID, SectionID: sec.ID, Title: sec.Title,
				Score: final, Reasons: s.Reasons, section: sec, document: doc,
			})
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		if matches[i].DocumentID != matches[j].DocumentID {
			return matches[i].DocumentID < matches[j].DocumentID
		}
		return matches[i].SectionID < matches[j].SectionID
	})
	if maxSections > 0 && len(matches) > maxSections {
		matches = matches[:maxSections]
	}
	return matches
}

func scoreSection(q Query, doc *Document, sec SectionMetadata) Score {
	return scoreFields(q, Fields{
		Title:       sec.Title,
		Summary:     sec.Summary,
		Keywords:    sec.Keywords,
		Description: doc.Metadata.Title + " " + doc.Metadata.Summary,
	})
}
