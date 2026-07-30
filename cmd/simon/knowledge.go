package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"simon-go/internal/agent"
	"simon-go/internal/config"
	"simon-go/internal/knowledge"
	"simon-go/internal/knowledge/embed"
	"simon-go/internal/knowledge/router"
)

// buildKnowledgeSearcher constructs the configured knowledge backend
// (settings.KnowledgeMode) as an agent.KnowledgeSearcher. It lives in the
// CLI layer rather than internal/agent so that selecting a concrete
// knowledge implementation never becomes the agent core's responsibility.
func buildKnowledgeSearcher(settings config.Settings) (agent.KnowledgeSearcher, error) {
	switch settings.KnowledgeMode {
	case "", "vector":
		embedder, err := embed.Default(settings)
		if err != nil {
			return nil, err
		}
		storePath := settings.KnowledgeStorePath
		if storePath == "" {
			storePath = ".simon_knowledge"
		}
		return knowledge.New(embedder, storePath)
	case "router":
		return router.New(routerConfig(settings))
	case "hybrid":
		return nil, fmt.Errorf("simon: KNOWLEDGE_MODE=hybrid is not implemented yet; use \"vector\" or \"router\"")
	default:
		return nil, fmt.Errorf("simon: unknown KNOWLEDGE_MODE %q (expected \"vector\", \"router\", or \"hybrid\")", settings.KnowledgeMode)
	}
}

func routerConfig(settings config.Settings) router.Config {
	return router.Config{
		KnowledgePath:    settings.KnowledgeRouterPath,
		Strict:           settings.KnowledgeRouterStrict,
		MaxCategories:    settings.KnowledgeRouterMaxCategories,
		MaxDocuments:     settings.KnowledgeRouterMaxDocuments,
		MaxSections:      settings.KnowledgeRouterMaxSections,
		MinCategoryScore: settings.KnowledgeRouterMinCategoryScore,
		MinDocumentScore: settings.KnowledgeRouterMinDocumentScore,
		MinSectionScore:  settings.KnowledgeRouterMinSectionScore,
	}
}

func cmdKnowledge(settings config.Settings, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "simon: knowledge requires a subcommand (build, validate, tree, search)")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "build":
		return cmdKnowledgeBuild(settings, rest)
	case "validate":
		return cmdKnowledgeValidate(settings, rest)
	case "tree":
		return cmdKnowledgeTree(settings, rest)
	case "search":
		return cmdKnowledgeSearch(settings, rest)
	default:
		fmt.Fprintf(os.Stderr, "simon: unknown knowledge subcommand %q\n", sub)
		return 2
	}
}

func cmdKnowledgeBuild(settings config.Settings, args []string) int {
	path := settings.KnowledgeRouterPath
	if len(args) > 0 {
		path = args[0]
	}
	cfg := routerConfig(settings)
	cfg.KnowledgePath = path

	catalog, issues, err := router.Load(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "simon:", err)
		return 1
	}

	sections := 0
	for _, doc := range catalog.Documents {
		if n := len(doc.Metadata.Sections); n > 0 {
			sections += n
		} else {
			sections++
		}
	}
	warnings := 0
	for _, issue := range issues {
		if issue.Severity == router.SeverityWarning {
			warnings++
		}
	}

	fmt.Println("Knowledge Router catalog loaded")
	fmt.Printf("Categories: %d\n", len(catalog.Categories))
	fmt.Printf("Documents: %d\n", len(catalog.Documents))
	fmt.Printf("Sections: %d\n", sections)
	fmt.Printf("Warnings: %d\n", warnings)
	return 0
}

func cmdKnowledgeValidate(settings config.Settings, args []string) int {
	path := settings.KnowledgeRouterPath
	if len(args) > 0 {
		path = args[0]
	}
	cfg := routerConfig(settings)
	cfg.KnowledgePath = path

	catalog, loadIssues, err := router.Load(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "simon:", err)
		return 1
	}
	issues := append(loadIssues, router.Validate(catalog, cfg.Strict)...)

	hasError := false
	for _, issue := range issues {
		fmt.Printf("[%s] %s", issue.Severity, issue.Path)
		if issue.Field != "" {
			fmt.Printf(" (%s)", issue.Field)
		}
		fmt.Printf(": %s\n", issue.Message)
		if issue.Severity == router.SeverityError {
			hasError = true
		}
	}
	if len(issues) == 0 {
		fmt.Println("No validation issues found.")
	}
	if hasError {
		return 1
	}
	return 0
}

func cmdKnowledgeTree(settings config.Settings, args []string) int {
	fs := flag.NewFlagSet("knowledge tree", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "print the tree as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	path := settings.KnowledgeRouterPath
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	cfg := routerConfig(settings)
	cfg.KnowledgePath = path

	catalog, _, err := router.Load(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "simon:", err)
		return 1
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return boolToExit(enc.Encode(treeJSON(catalog.Root)) == nil)
	}

	for _, child := range sortedChildren(catalog.Root) {
		printTree(child, "", true)
	}
	for _, doc := range sortedDocuments(catalog.Root.Documents) {
		fmt.Println(baseName(doc.SourcePath))
	}
	return 0
}

func cmdKnowledgeSearch(settings config.Settings, args []string) int {
	fs := flag.NewFlagSet("knowledge search", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "print results as JSON")
	explain := fs.Bool("explain", false, "print detailed routing explanations")
	topK := fs.Int("top-k", 5, "maximum number of results")
	maxCategories := fs.Int("max-categories", 0, "override MaxCategories")
	maxDocuments := fs.Int("max-documents", 0, "override MaxDocuments")
	maxSections := fs.Int("max-sections", 0, "override MaxSections")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "simon: knowledge search requires a query")
		return 2
	}
	query := fs.Arg(0)

	cfg := routerConfig(settings)
	r, err := router.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "simon:", err)
		return 1
	}

	ctx := context.Background()
	result, err := r.SearchDetailed(ctx, query, router.SearchOptions{
		MaxCategories:  *maxCategories,
		MaxDocuments:   *maxDocuments,
		MaxSections:    *maxSections,
		IncludeContent: true,
		Explain:        *explain,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "simon:", err)
		return 1
	}
	if *topK > 0 && len(result.Evidence) > *topK {
		result.Evidence = result.Evidence[:*topK]
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return boolToExit(enc.Encode(result) == nil)
	}

	if *explain {
		printExplain(result)
		return 0
	}

	for _, ev := range result.Evidence {
		fmt.Printf("%s\n%s\n\n", formatEvidenceSource(ev), ev.Content)
	}
	return 0
}

func printExplain(result *router.SearchResult) {
	fmt.Printf("Query: %s\n\n", result.Query)
	for _, c := range result.Categories {
		fmt.Printf("Category:\n  %s\n  Score: %.2f\n", c.Path, c.Score)
		printReasons(c.Reasons)
	}
	for _, d := range result.Documents {
		fmt.Printf("Document:\n  %s\n  Score: %.2f\n", d.Title, d.Score)
		printReasons(d.Reasons)
	}
	for _, s := range result.Sections {
		fmt.Printf("Section:\n  %s\n  Score: %.2f\n", s.Title, s.Score)
		printReasons(s.Reasons)
	}
	for _, ev := range result.Evidence {
		fmt.Printf("Source:\n  %s\n\n", formatEvidenceSource(ev))
	}
}

func printReasons(reasons []string) {
	if len(reasons) == 0 {
		return
	}
	fmt.Println("  Reasons:")
	for _, r := range reasons {
		fmt.Printf("    - %s\n", r)
	}
	fmt.Println()
}

func formatEvidenceSource(ev router.EvidenceFragment) string {
	if ev.StartLine > 0 && ev.EndLine > 0 {
		return fmt.Sprintf("%s#%s:L%d-L%d", baseName(ev.SourcePath), ev.SectionID, ev.StartLine, ev.EndLine)
	}
	return fmt.Sprintf("%s#%s", baseName(ev.SourcePath), ev.SectionID)
}

func baseName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

func sortedChildren(node *router.CategoryNode) []*router.CategoryNode {
	children := append([]*router.CategoryNode{}, node.Children...)
	sort.Slice(children, func(i, j int) bool { return children[i].Path < children[j].Path })
	return children
}

func sortedDocuments(docs []*router.Document) []*router.Document {
	sorted := append([]*router.Document{}, docs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].SourcePath < sorted[j].SourcePath })
	return sorted
}

func printTree(node *router.CategoryNode, prefix string, _ bool) {
	name := node.Metadata.Name
	if name == "" {
		name = node.Path
	}
	fmt.Println(prefix + name)
	childPrefix := prefix + "  "
	for _, child := range sortedChildren(node) {
		printTree(child, childPrefix, false)
	}
	for _, doc := range sortedDocuments(node.Documents) {
		fmt.Println(childPrefix + baseName(doc.SourcePath))
	}
}

type treeNode struct {
	Name       string     `json:"name"`
	Path       string     `json:"path"`
	Documents  []string   `json:"documents,omitempty"`
	Categories []treeNode `json:"categories,omitempty"`
}

func treeJSON(node *router.CategoryNode) treeNode {
	tn := treeNode{Name: node.Metadata.Name, Path: node.Path}
	for _, doc := range sortedDocuments(node.Documents) {
		tn.Documents = append(tn.Documents, baseName(doc.SourcePath))
	}
	for _, child := range sortedChildren(node) {
		tn.Categories = append(tn.Categories, treeJSON(child))
	}
	return tn
}

func boolToExit(ok bool) int {
	if ok {
		return 0
	}
	return 1
}
