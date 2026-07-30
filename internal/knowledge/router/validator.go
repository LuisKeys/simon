package router

import "fmt"

// ValidationSeverity classifies a ValidationIssue.
type ValidationSeverity string

const (
	SeverityError   ValidationSeverity = "error"
	SeverityWarning ValidationSeverity = "warning"
)

// ValidationIssue is one structured, inspectable problem found while loading
// or validating a catalog. Validation always collects every issue instead
// of failing on the first one.
type ValidationIssue struct {
	Severity ValidationSeverity `json:"severity"`
	Path     string             `json:"path"`
	Field    string             `json:"field,omitempty"`
	Message  string             `json:"message"`
}

// Validate re-checks an already-loaded catalog for metadata-quality issues
// that Load does not itself detect (missing descriptions, section overlaps,
// invalid line ranges, etc.), in addition to whatever issues Load already
// produced. strict escalates recoverable quality problems (empty
// descriptions/summaries, missing recommended fields) from warnings to
// errors.
func Validate(catalog *Catalog, strict bool) []ValidationIssue {
	var issues []ValidationIssue

	for path, node := range catalog.Categories {
		sev := SeverityWarning
		if strict {
			sev = SeverityError
		}
		if node.Metadata.Name == "" {
			issues = append(issues, ValidationIssue{Severity: sev, Path: path, Field: "name", Message: "empty category name"})
		}
		if node.Metadata.Description == "" {
			issues = append(issues, ValidationIssue{Severity: sev, Path: path, Field: "description", Message: "empty category description"})
		}
	}

	for id, doc := range catalog.Documents {
		issues = append(issues, validateDocument(id, doc, strict)...)
	}

	return issues
}

func validateDocument(id string, doc *Document, strict bool) []ValidationIssue {
	var issues []ValidationIssue
	sev := SeverityWarning
	if strict {
		sev = SeverityError
	}
	meta := doc.Metadata

	if meta.Version != 0 && meta.Version != supportedMetadataVersion {
		issues = append(issues, ValidationIssue{Severity: SeverityError, Path: doc.MetadataPath, Field: "version", Message: fmt.Sprintf("unsupported metadata version %d", meta.Version)})
	}
	if meta.Title == "" {
		issues = append(issues, ValidationIssue{Severity: sev, Path: doc.MetadataPath, Field: "title", Message: "empty document title"})
	}
	if meta.Description == "" {
		issues = append(issues, ValidationIssue{Severity: sev, Path: doc.MetadataPath, Field: "description", Message: "empty description"})
	}
	if meta.Summary == "" {
		issues = append(issues, ValidationIssue{Severity: sev, Path: doc.MetadataPath, Field: "summary", Message: "empty summary"})
	}

	seenSections := map[string]bool{}
	type lineRange struct{ start, end, idx int }
	var ranges []lineRange
	for i, s := range meta.Sections {
		if s.ID == "" {
			issues = append(issues, ValidationIssue{Severity: SeverityError, Path: doc.MetadataPath, Field: "sections", Message: fmt.Sprintf("section %d has no id", i)})
			continue
		}
		if seenSections[s.ID] {
			issues = append(issues, ValidationIssue{Severity: SeverityError, Path: doc.MetadataPath, Field: "sections", Message: "duplicate section id: " + s.ID})
			continue
		}
		seenSections[s.ID] = true
		if s.Summary == "" {
			issues = append(issues, ValidationIssue{Severity: sev, Path: doc.MetadataPath, Field: "sections." + s.ID, Message: "empty section summary"})
		}
		if s.StartLine > 0 || s.EndLine > 0 {
			if s.StartLine <= 0 || s.EndLine <= 0 {
				issues = append(issues, ValidationIssue{Severity: SeverityError, Path: doc.MetadataPath, Field: "sections." + s.ID, Message: "incomplete line range"})
				continue
			}
			if s.StartLine > s.EndLine {
				issues = append(issues, ValidationIssue{Severity: SeverityError, Path: doc.MetadataPath, Field: "sections." + s.ID, Message: "start_line greater than end_line"})
				continue
			}
			ranges = append(ranges, lineRange{s.StartLine, s.EndLine, i})
		}
	}

	for i := 0; i < len(ranges); i++ {
		for j := i + 1; j < len(ranges); j++ {
			a, b := ranges[i], ranges[j]
			if a.start <= b.end && b.start <= a.end {
				issues = append(issues, ValidationIssue{
					Severity: SeverityWarning,
					Path:     doc.MetadataPath,
					Field:    "sections",
					Message:  fmt.Sprintf("overlapping section ranges: %s and %s", meta.Sections[a.idx].ID, meta.Sections[b.idx].ID),
				})
			}
		}
	}

	if meta.Source.Format == "" {
		issues = append(issues, ValidationIssue{Severity: SeverityWarning, Path: doc.MetadataPath, Field: "source.format", Message: "source format not specified"})
	}

	return issues
}
