package router

// CatalogMetadata is the optional root catalog.yaml describing the whole
// knowledge tree.
type CatalogMetadata struct {
	Version         int    `yaml:"version" json:"version"`
	ID              string `yaml:"id" json:"id"`
	Name            string `yaml:"name" json:"name"`
	Description     string `yaml:"description" json:"description"`
	DefaultLanguage string `yaml:"default_language" json:"default_language"`
}

// CategoryMetadata is the category.yaml found in each taxonomy directory.
type CategoryMetadata struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Summary     string   `yaml:"summary" json:"summary"`
	Keywords    []string `yaml:"keywords" json:"keywords"`
	Aliases     []string `yaml:"aliases" json:"aliases"`
}

// SourceMetadata points a document's metadata at its underlying source file.
type SourceMetadata struct {
	Path   string `yaml:"path" json:"path"`
	Format string `yaml:"format" json:"format"`
}

// SectionMetadata describes one addressable region of a document's source.
type SectionMetadata struct {
	ID        string   `yaml:"id" json:"id"`
	Title     string   `yaml:"title" json:"title"`
	Summary   string   `yaml:"summary" json:"summary"`
	Keywords  []string `yaml:"keywords,omitempty" json:"keywords,omitempty"`
	StartLine int      `yaml:"start_line,omitempty" json:"start_line,omitempty"`
	EndLine   int      `yaml:"end_line,omitempty" json:"end_line,omitempty"`
	Page      int      `yaml:"page,omitempty" json:"page,omitempty"`
}

// DocumentMetadata is the sidecar YAML describing one indexed source
// document, including its section breakdown.
type DocumentMetadata struct {
	Version     int               `yaml:"version" json:"version"`
	ID          string            `yaml:"id" json:"id"`
	Title       string            `yaml:"title" json:"title"`
	Description string            `yaml:"description" json:"description"`
	Summary     string            `yaml:"summary" json:"summary"`
	Categories  []string          `yaml:"categories" json:"categories"`
	Keywords    []string          `yaml:"keywords" json:"keywords"`
	Aliases     []string          `yaml:"aliases" json:"aliases"`
	Source      SourceMetadata    `yaml:"source" json:"source"`
	Sections    []SectionMetadata `yaml:"sections" json:"sections"`
}

// syntheticSectionID marks the single synthetic section used when a
// document declares no explicit sections.
const syntheticSectionID = "__document__"

// supportedMetadataVersion is the only DocumentMetadata/CatalogMetadata
// version understood by this MVP.
const supportedMetadataVersion = 1
