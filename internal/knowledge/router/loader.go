package router

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	catalogFileName  = "catalog.yaml"
	categoryFileName = "category.yaml"
)

// rawDocument is a document sidecar YAML file paired with its filesystem
// location, before it has been resolved into a canonical *Document.
type rawDocument struct {
	metadata     DocumentMetadata
	metadataPath string
	dir          string // directory containing the sidecar file, relative to root
}

// Load walks knowledgePath and builds a Catalog plus the list of validation
// issues discovered along the way. Load never fails solely because of
// metadata quality problems (missing fields, unknown categories, etc.) —
// those are reported as ValidationIssue values so callers can decide how to
// react (Config.Strict escalates recoverable issues to errors upstream via
// Validate/Router construction). Load only returns a non-nil error for
// unrecoverable filesystem/YAML-syntax failures.
func Load(cfg Config) (*Catalog, []ValidationIssue, error) {
	root, err := filepath.Abs(cfg.KnowledgePath)
	if err != nil {
		return nil, nil, wrapf(err, "resolve knowledge root %q", cfg.KnowledgePath)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, nil, wrapf(err, "resolve knowledge root %q", cfg.KnowledgePath)
	}

	var issues []ValidationIssue

	catalogMeta := CatalogMetadata{Version: supportedMetadataVersion, Name: filepath.Base(root)}
	catalogPath := filepath.Join(root, catalogFileName)
	if data, err := os.ReadFile(catalogPath); err == nil {
		var m CatalogMetadata
		if err := yaml.Unmarshal(data, &m); err != nil {
			issues = append(issues, ValidationIssue{Severity: SeverityError, Path: catalogPath, Message: "parse catalog.yaml: " + err.Error()})
		} else {
			catalogMeta = m
		}
	}

	catalog := &Catalog{
		Metadata:   catalogMeta,
		RootPath:   root,
		Root:       &CategoryNode{Path: ""},
		Categories: map[string]*CategoryNode{},
		Documents:  map[string]*Document{},
	}

	var rawDocs []rawDocument

	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == catalogFileName {
			return nil
		}
		if strings.EqualFold(filepath.Ext(name), ".yaml") || strings.EqualFold(filepath.Ext(name), ".yml") {
			if name == categoryFileName {
				if err := loadCategoryNode(catalog, root, path, &issues); err != nil {
					issues = append(issues, ValidationIssue{Severity: SeverityError, Path: path, Message: err.Error()})
				}
				return nil
			}
			var meta DocumentMetadata
			data, err := os.ReadFile(path)
			if err != nil {
				issues = append(issues, ValidationIssue{Severity: SeverityError, Path: path, Message: "read document metadata: " + err.Error()})
				return nil
			}
			if err := yaml.Unmarshal(data, &meta); err != nil {
				issues = append(issues, ValidationIssue{Severity: SeverityError, Path: path, Message: "parse document metadata: " + err.Error()})
				return nil
			}
			rel, _ := filepath.Rel(root, filepath.Dir(path))
			rawDocs = append(rawDocs, rawDocument{metadata: meta, metadataPath: path, dir: normalizeCategoryPath(rel)})
		}
		return nil
	})
	if walkErr != nil {
		return nil, nil, wrapf(walkErr, "walk knowledge root %q", root)
	}

	ensureAncestorCategories(catalog)
	linkCategoryTree(catalog)

	sort.Slice(rawDocs, func(i, j int) bool { return rawDocs[i].metadataPath < rawDocs[j].metadataPath })

	for _, rd := range rawDocs {
		resolveDocument(catalog, root, rd, cfg.Strict, &issues)
	}

	return catalog, issues, nil
}

// loadCategoryNode parses one category.yaml and registers the corresponding
// CategoryNode (creating it if a raw walk hasn't touched this path yet).
func loadCategoryNode(catalog *Catalog, root, path string, issues *[]ValidationIssue) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var meta CategoryMetadata
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return err
	}
	rel, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil {
		return err
	}
	catPath := normalizeCategoryPath(rel)
	if catPath == "." {
		// category.yaml at the knowledge root describes the root itself;
		// there is no addressable category path for it.
		return nil
	}
	if existing, ok := catalog.Categories[catPath]; ok {
		*issues = append(*issues, ValidationIssue{Severity: SeverityError, Path: path, Field: "path", Message: "duplicate category path: " + catPath})
		existing.Metadata = meta
		return nil
	}
	catalog.Categories[catPath] = &CategoryNode{Metadata: meta, Path: catPath}
	return nil
}

// ensureAncestorCategories synthesizes placeholder CategoryNodes for any
// directory that sits between the root and a known category but has no
// category.yaml of its own, so the tree stays connected.
func ensureAncestorCategories(catalog *Catalog) {
	for path := range catalog.Categories {
		parts := strings.Split(path, "/")
		for i := 1; i < len(parts); i++ {
			ancestor := strings.Join(parts[:i], "/")
			if _, ok := catalog.Categories[ancestor]; !ok {
				catalog.Categories[ancestor] = &CategoryNode{
					Metadata: CategoryMetadata{ID: parts[i-1], Name: parts[i-1]},
					Path:     ancestor,
				}
			}
		}
	}
}

// linkCategoryTree wires Parent/Children pointers once every category node
// (real and synthesized) is known.
func linkCategoryTree(catalog *Catalog) {
	paths := make([]string, 0, len(catalog.Categories))
	for p := range catalog.Categories {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		node := catalog.Categories[p]
		parentPath := parentCategoryPath(p)
		var parent *CategoryNode
		if parentPath == "" {
			parent = catalog.Root
		} else {
			parent = catalog.Categories[parentPath]
		}
		node.Parent = parent
		parent.Children = append(parent.Children, node)
	}
}

func parentCategoryPath(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return ""
	}
	return path[:idx]
}

// normalizeCategoryPath converts an OS-specific relative path into the
// canonical "/"-separated category path form.
func normalizeCategoryPath(rel string) string {
	return filepath.ToSlash(filepath.Clean(rel))
}

// resolveDocument turns one rawDocument into a canonical *Document,
// resolves its source path safely within root, links it into its declared
// (or inferred) categories, and records validation issues.
func resolveDocument(catalog *Catalog, root string, rd rawDocument, strict bool, issues *[]ValidationIssue) {
	meta := rd.metadata
	if meta.ID == "" {
		meta.ID = strings.TrimSuffix(filepath.Base(rd.metadataPath), filepath.Ext(rd.metadataPath))
	}
	if _, exists := catalog.Documents[meta.ID]; exists {
		*issues = append(*issues, ValidationIssue{Severity: SeverityError, Path: rd.metadataPath, Field: "id", Message: "duplicate document id: " + meta.ID})
		return
	}

	sourceRel := meta.Source.Path
	if sourceRel == "" {
		*issues = append(*issues, ValidationIssue{Severity: SeverityError, Path: rd.metadataPath, Field: "source.path", Message: "document has no source.path"})
		return
	}

	docDirAbs := filepath.Dir(rd.metadataPath)
	sourceAbs := filepath.Join(docDirAbs, sourceRel)
	safeSource, err := safeJoin(root, sourceAbs)
	if err != nil {
		*issues = append(*issues, ValidationIssue{Severity: SeverityError, Path: rd.metadataPath, Field: "source.path", Message: "unsafe source path: " + err.Error()})
		return
	}
	if _, err := os.Stat(safeSource); err != nil {
		*issues = append(*issues, ValidationIssue{Severity: SeverityError, Path: rd.metadataPath, Field: "source.path", Message: "source file not found: " + sourceRel})
		return
	}

	doc := &Document{Metadata: meta, MetadataPath: rd.metadataPath, SourcePath: safeSource}
	catalog.Documents[meta.ID] = doc

	categories := meta.Categories
	if len(categories) == 0 {
		if strict {
			*issues = append(*issues, ValidationIssue{Severity: SeverityError, Path: rd.metadataPath, Field: "categories", Message: "document has no categories"})
		} else if rd.dir != "." {
			categories = []string{rd.dir}
		}
	}

	seen := map[string]bool{}
	for _, catPath := range categories {
		catPath = normalizeCategoryPath(catPath)
		if seen[catPath] {
			continue
		}
		seen[catPath] = true
		node, ok := catalog.Categories[catPath]
		if !ok {
			sev := SeverityWarning
			if strict {
				sev = SeverityError
			}
			*issues = append(*issues, ValidationIssue{Severity: sev, Path: rd.metadataPath, Field: "categories", Message: "unknown category reference: " + catPath})
			continue
		}
		node.Documents = append(node.Documents, doc)
	}
}

// safeJoin resolves candidate (an absolute path derived from user-controlled
// metadata) and verifies the result stays within root, following symlinks.
func safeJoin(root, candidate string) (string, error) {
	cleaned := filepath.Clean(candidate)
	resolved := cleaned
	if evaled, err := filepath.EvalSymlinks(cleaned); err == nil {
		resolved = evaled
	}
	relRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		relRoot = root
	}
	rel, err := filepath.Rel(relRoot, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", wrapf(nil, "path %q escapes knowledge root", candidate)
	}
	return cleaned, nil
}
