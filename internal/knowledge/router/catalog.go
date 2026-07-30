package router

// Catalog is the immutable, in-memory runtime model built from a knowledge
// directory tree. Reload builds a new Catalog and callers atomically swap
// it in (see Router.Reload) rather than mutating one in place.
type Catalog struct {
	Metadata CatalogMetadata
	RootPath string
	Root     *CategoryNode

	// Categories is keyed by normalized, "/"-separated category path
	// (e.g. "software/databases").
	Categories map[string]*CategoryNode

	// Documents is keyed by document ID. There is exactly one canonical
	// *Document per ID even when it is referenced from multiple
	// categories.
	Documents map[string]*Document
}

// CategoryNode is one node of the category tree. The synthetic root node
// (Path == "") has no metadata of its own and is never returned as a match.
type CategoryNode struct {
	Metadata CategoryMetadata
	Path     string
	Parent   *CategoryNode
	Children []*CategoryNode

	// Documents lists canonical documents assigned to this category,
	// pointing into Catalog.Documents rather than owning copies.
	Documents []*Document
}

// Document is the canonical, catalog-wide record for one indexed source
// document.
type Document struct {
	Metadata     DocumentMetadata
	MetadataPath string
	SourcePath   string
}
