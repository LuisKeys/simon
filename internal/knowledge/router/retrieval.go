package router

import (
	"strings"

	"github.com/LuisKeys/simon/internal/knowledge/extract"
)

// EvidenceFragment is a bounded region of a document's extracted source
// text, addressed by section and (when available) line range.
type EvidenceFragment struct {
	DocumentID string `json:"document_id"`
	Document   string `json:"document"`
	SectionID  string `json:"section_id"`
	Section    string `json:"section"`

	SourcePath string `json:"source_path"`
	StartLine  int    `json:"start_line,omitempty"`
	EndLine    int    `json:"end_line,omitempty"`

	Content   string  `json:"content"`
	Score     float64 `json:"score"`
	Truncated bool    `json:"truncated,omitempty"`
}

// extractionCache memoizes extract.Text results within a single search
// request so a document referenced by multiple selected sections is only
// extracted once.
type extractionCache struct {
	cache map[string][]string // path -> lines
	errs  map[string]error
}

func newExtractionCache() *extractionCache {
	return &extractionCache{cache: map[string][]string{}, errs: map[string]error{}}
}

func (c *extractionCache) lines(path string) ([]string, error) {
	if lines, ok := c.cache[path]; ok {
		return lines, nil
	}
	if err, ok := c.errs[path]; ok {
		return nil, err
	}
	text, err := extract.Text(path)
	if err != nil {
		c.errs[path] = err
		return nil, err
	}
	lines := strings.Split(text, "\n")
	c.cache[path] = lines
	return lines, nil
}

// retrieveEvidence resolves a single section match into an EvidenceFragment,
// reading only the selected line range (or a bounded prefix of the whole
// document for sections with no line range, e.g. the synthetic
// "__document__" section). maxChars caps the fragment size; content beyond
// it is truncated and flagged.
func retrieveEvidence(cache *extractionCache, m SectionMatch, maxChars int) (EvidenceFragment, error) {
	doc := m.document
	frag := EvidenceFragment{
		DocumentID: m.DocumentID,
		Document:   doc.Metadata.Title,
		SectionID:  m.SectionID,
		Section:    m.Title,
		SourcePath: doc.SourcePath,
		Score:      m.Score,
	}

	lines, err := cache.lines(doc.SourcePath)
	if err != nil {
		return EvidenceFragment{}, wrapf(err, "retrieve %s/%s: extract source", doc.Metadata.ID, m.SectionID)
	}

	var content string
	if m.section.StartLine > 0 && m.section.EndLine > 0 {
		start := m.section.StartLine
		end := m.section.EndLine
		if start < 1 {
			start = 1
		}
		if end > len(lines) {
			end = len(lines)
		}
		if start > end || start > len(lines) {
			return EvidenceFragment{}, wrapf(nil, "retrieve %s/%s: invalid line range [%d,%d] for %d-line document", doc.Metadata.ID, m.SectionID, m.section.StartLine, m.section.EndLine, len(lines))
		}
		content = strings.Join(lines[start-1:end], "\n")
		frag.StartLine = start
		frag.EndLine = end
	} else {
		end := len(lines)
		if end > 200 {
			end = 200
		}
		content = strings.Join(lines[:end], "\n")
	}

	content = strings.Trim(content, "\n\r\t ")
	if content == "" {
		return EvidenceFragment{}, wrapf(nil, "retrieve %s/%s: extracted content is empty", doc.Metadata.ID, m.SectionID)
	}
	if len(content) > maxChars {
		content = content[:maxChars]
		frag.Truncated = true
	}
	frag.Content = content
	return frag, nil
}
