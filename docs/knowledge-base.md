# Knowledge base

Simon has two independent, coexisting retrieval backends, both wired into
`internal/agent` purely through the `agent.KnowledgeSearcher` interface:

- **Vector `KnowledgeBase`** (`internal/knowledge`) — embeddings + a
  brute-force vector index (SIDX). Documented in this file, below.
- **Knowledge Router** (`internal/knowledge/router`) — hierarchical,
  lexical, embeddings-free routing (category → document → section →
  evidence). Documented in [Knowledge Router](#knowledge-router), below.

Select which one `simon knowledge`/example code builds via
`KNOWLEDGE_MODE` (`vector` [default] | `router` | `hybrid` [reserved, not
implemented]) — see [configuration.md](configuration.md). Neither backend
imports the other, and `simon index`/the vector `KnowledgeBase` behave
exactly as before this feature was added.

| Capability | Vector `KnowledgeBase` | Knowledge Router |
|---|---|---|
| Embeddings | Required | Not required |
| Vector index | SIDX | None |
| Metadata | Minimal (source path only) | Structured YAML (category + document + section) |
| Retrieval | Chunk similarity | Hierarchical routing |
| Hardware | Provider-dependent | Low-resource, fully offline |
| Explainability | Similarity score | Field-level score reasons |
| Setup | Automatic indexing (`simon index`) | Curated metadata (`simon knowledge build/validate`) |
| Best fit | Unstructured corpora | Controlled, curated knowledge bases |

## Vector `KnowledgeBase`

Packages: `internal/knowledge` (orchestrator), `internal/knowledge/embed`,
`internal/knowledge/extract`, `internal/knowledge/index`. All three leaf
packages are independent of each other; `knowledge.go` is the only file
that imports all three.

```
internal/knowledge  (KnowledgeBase: Add, Search, chunk)
 ├── internal/knowledge/embed    (Embedder: OpenAI / Ollama / Voyage)
 ├── internal/knowledge/extract  (Text(path): pdf/docx/xlsx/pptx/plain)
 └── internal/knowledge/index    (FileIndex: SIDX binary format)
```

Wired into `internal/agent` only through the `agent.KnowledgeSearcher`
interface (`Search(ctx, query, topK) ([]response.KnowledgeHit, error)`) —
the agent package never imports `internal/knowledge` directly, so building
a bare agent doesn't pull in embeddings/index/extraction dependencies.
Attach with `agent.WithKnowledge(kb)`.

## `internal/knowledge.KnowledgeBase`

```go
type KnowledgeBase struct { ChunkSize, Overlap int /* + unexported */ }

func New(embedder embed.Embedder, storePath string, opts ...Option) (*KnowledgeBase, error)
func WithChunkSize(n int) Option   // default 500
func WithOverlap(n int) Option     // default 50

func (kb *KnowledgeBase) Add(ctx context.Context, path string, force bool) (int, error)
func (kb *KnowledgeBase) Search(ctx context.Context, query string, topK int) ([]response.KnowledgeHit, error)
```

### `Add`

1. If `path` is a directory, `filepath.WalkDir` collects every non-dir file
   recursively; otherwise it's a single-file list.
2. Per file: skip if `idx.HasSource(file)` is already true and `force` is
   false.
3. `extract.Text(file)` → raw text → `kb.chunk(text)` → overlapping
   character windows (skip the file if zero chunks result).
4. `embedder.EmbedBatch(ctx, chunks)` → one vector per chunk.
5. `idx.AddSource(ctx, file, chunks, vectors, force)` persists it.
6. Returns the total chunk count added across all files.

### `chunk` — sliding window

Character- (rune-) based, not token-based, mirroring the Python original
exactly:

- Empty (after `TrimSpace`) input → `nil`.
- `step := ChunkSize - Overlap`, floored at 1.
- Windows of `[i, min(i+ChunkSize, len(runes)))` for `i` stepping by `step`
  across the rune slice; each window is trimmed, and empty results after
  trimming are dropped.

### `Search`

1. If `storePath` doesn't exist on disk yet, returns `(nil, nil)` — no
   error — matching Python's behavior for an as-yet-unindexed knowledge
   base.
2. Embeds the query string.
3. Delegates to `idx.Search(ctx, queryVector, topK)`.
4. Maps `index.SearchResult{Text, Source, Score}` →
   `response.KnowledgeHit{Text, Source, Score}`.

## `internal/knowledge/embed` — embedding providers

```go
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}
func Default(settings config.Settings) (Embedder, error)
```

`Default` dispatches on `strings.ToUpper(settings.EmbeddingProvider)`:

| `EMBEDDING_PROVIDER` | Implementation | Notes |
|---|---|---|
| `OLLAMA` | `NewOllama(settings.OllamaHost, settings.EmbeddingModel)` | |
| `ANTHROPIC` | `NewVoyage(settings.AnthropicAPIKey, settings.EmbeddingModel)` | Anthropic has no first-party embeddings API; Voyage AI is Anthropic's recommended provider |
| `OPENAI` (default) | `NewOpenAI(settings.OpenAIAPIKey, settings.EmbeddingModel)` | default model `text-embedding-3-small` |
| anything else | — | `simonerr.NewKnowledgeError("unknown EMBEDDING_PROVIDER %q...")` |

**Every vector returned by every provider is L2-normalized** via a shared
`normalize` helper before being handed back (dividing by 1.0 instead of a
zero norm, to avoid `NaN`). This is why `internal/knowledge/index` can use
a plain dot product as cosine similarity — the normalization already
happened upstream.

- **`OpenAI`** — wraps `github.com/openai/openai-go/v2`'s
  `Embeddings.New`, converting `[]float64` responses to `[]float32`.
- **`Ollama`** — wraps `/api/embed` via `github.com/ollama/ollama/api`. A
  malformed `host` doesn't fail construction; the client is left `nil` and
  the error surfaces on first `Embed`/`EmbedBatch` call, mirroring Python's
  lazy `_get_client()`.
- **`Voyage`** — hand-rolled minimal HTTP client against
  `https://api.voyageai.com/v1/embeddings` (no official Go SDK exists,
  unlike Python's `voyageai` package). Default model `voyage-2`. Results
  are placed at `out[item.Index]` per the API's own returned ordering
  index, not assumed to come back in request order.

## `internal/knowledge/extract` — text extraction

```go
func Text(path string) (string, error)
```

The sole exported symbol; dispatches on lowercased file extension:

| Extension | Handler | Approach |
|---|---|---|
| `.pdf` | `pdfText` | `github.com/ledongthuc/pdf` → `GetPlainText()` |
| `.docx` | `docxText` | unzips, walks `word/document.xml`'s `<w:p>/<w:t>` |
| `.pptx` | `pptxText` | unzips, walks each `ppt/slides/slideN.xml`'s `<a:p>/<a:t>`, **numerically** sorted by slide number (regex `slide(\d+)\.xml$`) to avoid `slide10` sorting before `slide2` |
| `.xlsx` | `xlsxText` | `github.com/xuri/excelize/v2`; every sheet, every row, cells tab-joined, blank rows skipped |
| anything else | `plainText` | raw file bytes, `strings.ToValidUTF8(data, "")` to drop invalid bytes — mirrors Python's `read_text(errors='ignore')` |

`docx`/`pptx` share one XML-streaming paragraph walker
(`extractParagraphText`/`paragraphsOf` in `ooxml.go`) since Word's
`<w:p>/<w:t>` and PowerPoint's DrawingML `<a:p>/<a:t>` share the same
paragraph/text-run shape.

## `internal/knowledge/index` — the SIDX vector index

> Replaces Python's pickle+numpy `.npy` retrieval format
> (`simon/knowledge/retrieval.py FileRetriever`) with a from-scratch
> design: **no binary compatibility with an existing Python
> `.simon_knowledge/` directory is preserved or required.**

```go
type Chunk struct { Text, Source string }
type SearchResult struct { Text, Source string; Score float32 }

type Index interface {
    AddSource(ctx context.Context, source string, chunks []Chunk, vectors [][]float32, force bool) error
    HasSource(source string) bool
    Search(ctx context.Context, query []float32, topK int) ([]SearchResult, error)
}

func Open(dir string) (*FileIndex, error) // creates dir if needed
```

`Index` is an interface — "the Go analogue of Python's `FileRetriever`,
generalized ... so a future ANN backend can replace brute-force search
without changing callers" — with `FileIndex` as the only current
implementation.

### Key derivation

`key := hex.EncodeToString(sha256.Sum256([]byte(source)))[:16]` — the first
16 hex characters (8 bytes) of the SHA-256 hash of the source path/URL
string. Matches Python's keying scheme.

### On-disk layout

Per source, directly under the `FileIndex` root directory:

```
<key>.sidx        binary: magic + version + dim + count + float32 vectors
<key>.meta.json   JSON array of {"text": "...", "source": "..."} (replaces pickle)
manifest.json     one shared file per directory: format version + dim + sources map
```

### The `.sidx` binary format

All integers little-endian:

| Offset | Size | Field | Notes |
|---|---|---|---|
| 0 | 4 bytes | Magic | ASCII `"SIDX"` |
| 4 | 2 bytes | Version | `uint16`, currently `1` |
| 6 | 2 bytes | Dim | `uint16`, vector dimensionality (`0` if no vectors) |
| 8 | 4 bytes | Count | `uint32`, number of vectors |
| 12 | `count * dim * 4` bytes | Vectors | `count` vectors of `dim` consecutive `float32`s each, row-major |

12-byte header. Read-side validation rejects a bad magic, an unsupported
version, or a file truncated relative to its own declared `count`/`dim`.

### `manifest.json`

```go
type manifest struct {
    FormatVersion  int            `json:"format_version"`
    EmbeddingModel string         `json:"embedding_model,omitempty"`
    Dim            int            `json:"dim"`
    Sources        map[string]any `json:"sources"` // key -> {"source": path, "chunk_count": n}
}
```

Rewritten in full on every successful `AddSource`. **`EmbeddingModel` is
never populated anywhere in the code** (always empty, thus omitted) and
**there is no code path that reads `manifest.json` back** — it is a
write-only, human/tooling-readable summary, not consulted by `Open` or
`loadExisting`.

### The mismatch guard that *is* enforced

Dimension mismatch detection lives entirely in memory, re-derived from
`.sidx` file headers on `Open` → `loadExisting` (not from
`manifest.json`). On every `AddSource` call with vectors, if
`fi.dim != 0 && dim != fi.dim`, it fails with:

> `index: embedding dimension mismatch: index is %d-dim, got %d-dim
> (mixing embedding models/providers is not supported)`

If an index directory happens to hold multiple sources with genuinely
different dimensions from prior writes, `loadExisting`'s in-memory `fi.dim`
ends up set from whichever `.sidx` file `os.ReadDir` happens to enumerate
last (no explicit ordering) — in practice, one index directory is expected
to hold one embedding space.

### `AddSource` flow

1. Validate `len(chunks) == len(vectors)`.
2. If `key` already exists: no-op unless `force`, in which case the old
   `<key>.sidx`/`<key>.meta.json` are deleted first (`removeLocked`).
3. Dimension check (above); update `fi.dim`.
4. Write `.sidx` then `.meta.json`.
5. Update in-memory `matrices`/`meta`/`sources` maps.
6. Rewrite `manifest.json`.

### `Search` flow — brute force, explicitly

1. Empty index → `(nil, nil)`.
2. For every source, for every vector, `dot(vec, query)` (min-length-safe,
   doesn't error on ragged vectors) — vectors are pre-normalized by
   `embed`, so a plain dot product is cosine similarity.
3. Sort all results (across every source) descending by score.
4. Truncate to `topK`.

The package doc comment notes this brute-force approach is "appropriate
for this SDK's scale" — there is no ANN index, and none is currently
planned beyond the `Index` interface leaving room for one.

## Knowledge Router

`internal/knowledge/router` — a second retrieval strategy, not a
replacement for the vector `KnowledgeBase` or a RAG competitor. Rather than
embedding and searching every chunk, it routes a query through a curated
taxonomy: `category → subcategory → document → section`, then reads only
the selected source regions via the existing `extract.Text`. No embeddings,
vector index, database, or LLM call is involved in retrieval itself.

```
internal/knowledge/router
 ├── metadata.go, loader.go, catalog.go, validator.go   (YAML -> in-memory Catalog)
 ├── tokenizer.go, scorer.go                             (deterministic lexical scoring)
 ├── category_router.go, document_router.go, section_router.go
 ├── retrieval.go                                        (bounded extract.Text-backed reads)
 ├── result.go, router.go                                (SearchResult, public Router API)
 └── errors.go
```

It depends only on `internal/knowledge/extract`, `internal/agent/response`,
`pkg/simonerr`, the standard library, and `gopkg.in/yaml.v3` — it does not
import `internal/agent`, `internal/knowledge/embed`, or
`internal/knowledge/index`. Like the vector `KnowledgeBase`, it satisfies
`agent.KnowledgeSearcher` and attaches with `agent.WithKnowledge(r)`.

### Knowledge directory layout

Directories are categories. Each category directory holds a `category.yaml`;
each indexed document has a sidecar YAML file (normally matching the source
file's basename) describing it. An optional root `catalog.yaml` names the
whole tree. See `internal/knowledge/router/testdata/knowledge_router/` and
`examples/knowledge_router_agent/knowledge/` for worked examples:

```
knowledge/
├── catalog.yaml
└── software/
    ├── category.yaml
    └── databases/
        ├── category.yaml
        ├── postgres-workers.md
        └── postgres-workers.yaml     # source: {path, format} + sections: [{id, start_line, end_line, summary}, ...]
```

A document's `categories:` list may name more than one category path
(`software/databases`, `software/distributed-systems`, ...) — it is not
physically duplicated, just referenced from each category's `Documents`
slice around one canonical `*Document`. If a document declares no
categories, non-strict mode attaches it to its physical parent directory;
strict mode (`KNOWLEDGE_ROUTER_STRICT=true`) treats that as a validation
error instead.

### Search flow

`Router.SearchDetailed` (which `Search` calls internally with
`topK`-truncation) runs:

1. **Category routing** — score the root's children, descend into the
   children of the best `MaxCategories` candidates at each level (descent
   is not gated on the parent's own score — a broad parent like "software"
   may share little vocabulary with a query about one specific child), up
   to `MaxDepth`. A candidate is only added to the result once its own
   score clears `MinCategoryScore`.
2. **Document routing** — collect every document referenced by a selected
   category (deduplicated), score each one's metadata independently (no
   source content is read here), add a small per-matching-category bonus,
   keep the best `MaxDocuments` above `MinDocumentScore`.
3. **Section routing** — score every section of every candidate document
   (or one synthetic `__document__` section for documents with no declared
   sections), combine with a weighted share of the parent document's and
   category's scores (`final = sectionScore + documentScore*0.35 +
   categoryScore*0.15`), keep the best `MaxSections` above
   `MinSectionScore`.
4. **Evidence retrieval** — for each selected section, `extract.Text` the
   document (cached per search request, so a document referenced by
   multiple sections is only extracted once), slice out the declared
   `start_line`/`end_line` range (or a bounded prefix for the synthetic
   whole-document section), and truncate to `MaxEvidenceCharacters`
   (default 8000).

Scoring is a deterministic weighted sum over exact-phrase, title/keyword/
alias/description/summary/category-path matches (see the constants in
`scorer.go`), normalized by `10 * max(1, tokenCount)` so short and long
queries land in a comparable range; every match records a human-readable
reason string, surfaced end-to-end through `SearchResult` and `simon
knowledge search --explain`.

### Configuration

See [configuration.md](configuration.md) for the full `KNOWLEDGE_ROUTER_*`
env var table. `router.Config`/`SearchOptions` mirror those settings
1:1 (`cmd/simon/knowledge.go`'s `routerConfig` is the mapping function).

### CLI

```bash
simon knowledge build <path>       # load + print catalog counts
simon knowledge validate [path]    # print every ValidationIssue; exit 1 on any error
simon knowledge tree [path] [--json]
simon knowledge search "<query>" [--json] [--explain] [--top-k N] \
  [--max-categories N] [--max-documents N] [--max-sections N]
```

`simon index <path>` is unchanged and continues to build the vector
`KnowledgeBase` — Knowledge Router is exclusively reached through `simon
knowledge ...`.

### Limitations (MVP)

- No embeddings/vector fallback, BM25/SQLite FTS index, LLM-assisted
  routing, or query rewriting — pure deterministic lexical scoring.
- No stemming: `"workers"` will not match a keyword written as
  `"worker"` — write plural/singular keyword variants explicitly if a
  query is expected to use either form.
- No automatic metadata generation or taxonomy discovery — every category
  and document requires a curated YAML sidecar.
- No filesystem watching, OCR, HTTP/MCP exposure, or cross-document answer
  synthesis; `Router.Reload` must be called explicitly to pick up changes.
- Line-range sections are canonical; `SectionMetadata.Page` is accepted and
  stored but not yet used for page-aware extraction.
