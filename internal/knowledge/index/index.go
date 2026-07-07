// Package index implements Simon's vector index, replacing Python's
// pickle+numpy .npy retrieval.py format (simon/knowledge/retrieval.py
// FileRetriever) with a from-scratch design: no binary compatibility with
// existing Python .simon_knowledge/ data is preserved or required.
//
// Per source (keyed by sha256(source)[:16], matching Python), a directory
// holds:
//
//	<key>.sidx       binary: magic + version + dim + count + float32 vectors
//	<key>.meta.json  JSON: [{text, source}, ...] chunk metadata (replaces pickle)
//	manifest.json    format version + embedding model/dim, for mismatch detection
package index

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"simon-go/pkg/simonerr"
)

// Chunk is one indexed piece of text and the source it came from.
type Chunk struct {
	Text   string `json:"text"`
	Source string `json:"source"`
}

// SearchResult is one search hit, ranked by cosine/dot-product Score
// (vectors are pre-normalized by the embed package, so a plain dot product
// is cosine similarity).
type SearchResult struct {
	Text   string
	Source string
	Score  float32
}

// Index is Simon's vector store contract (the Go analogue of Python's
// FileRetriever, generalized behind an interface so a future ANN backend
// can replace brute-force search without changing callers).
type Index interface {
	AddSource(ctx context.Context, source string, chunks []Chunk, vectors [][]float32, force bool) error
	HasSource(source string) bool
	Search(ctx context.Context, query []float32, topK int) ([]SearchResult, error)
}

const (
	sidxMagic    = "SIDX"
	sidxVersion  = uint16(1)
	manifestFile = "manifest.json"
)

// manifest tracks the embedding model/dimension used to build the index,
// so a later Add with a different embedding model/dim fails loudly instead
// of corrupting search results by mixing incompatible vector spaces.
type manifest struct {
	FormatVersion  int            `json:"format_version"`
	EmbeddingModel string         `json:"embedding_model,omitempty"`
	Dim            int            `json:"dim"`
	Sources        map[string]any `json:"sources"`
}

// FileIndex is a directory-backed Index: one .sidx + .meta.json file pair
// per source, all loaded into memory on open.
type FileIndex struct {
	mu       sync.Mutex
	dir      string
	dim      int
	matrices map[string][][]float32 // key -> vectors
	meta     map[string][]Chunk     // key -> chunk metadata, same order as matrices
	sources  map[string]string      // key -> original source path/URL
}

// Open loads (or creates) a FileIndex rooted at dir.
func Open(dir string) (*FileIndex, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	fi := &FileIndex{
		dir:      dir,
		matrices: map[string][][]float32{},
		meta:     map[string][]Chunk{},
		sources:  map[string]string{},
	}
	if err := fi.loadExisting(); err != nil {
		return nil, err
	}
	return fi, nil
}

func keyFor(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])[:16]
}

func (fi *FileIndex) loadExisting() error {
	entries, err := os.ReadDir(fi.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sidx" {
			continue
		}
		key := entry.Name()[:len(entry.Name())-len(".sidx")]
		vectors, err := readSIDX(filepath.Join(fi.dir, entry.Name()))
		if err != nil {
			return fmt.Errorf("index: reading %s: %w", entry.Name(), err)
		}
		chunks, err := readMeta(filepath.Join(fi.dir, key+".meta.json"))
		if err != nil {
			return fmt.Errorf("index: reading %s.meta.json: %w", key, err)
		}
		fi.matrices[key] = vectors
		fi.meta[key] = chunks
		if len(chunks) > 0 {
			fi.sources[key] = chunks[0].Source
		}
		if len(vectors) > 0 {
			fi.dim = len(vectors[0])
		}
	}
	return nil
}

// HasSource reports whether source has already been indexed.
func (fi *FileIndex) HasSource(source string) bool {
	fi.mu.Lock()
	defer fi.mu.Unlock()
	_, ok := fi.matrices[keyFor(source)]
	return ok
}

// AddSource indexes chunks (with their pre-computed vectors) under source.
// If source was already indexed and force is false, AddSource is a no-op;
// with force=true the previous entry is replaced.
func (fi *FileIndex) AddSource(_ context.Context, source string, chunks []Chunk, vectors [][]float32, force bool) error {
	if len(chunks) != len(vectors) {
		return simonerr.NewKnowledgeError("index: chunks and vectors must have the same length", nil)
	}
	fi.mu.Lock()
	defer fi.mu.Unlock()

	key := keyFor(source)
	if _, exists := fi.matrices[key]; exists {
		if !force {
			return nil
		}
		if err := fi.removeLocked(key); err != nil {
			return err
		}
	}

	if len(vectors) > 0 {
		dim := len(vectors[0])
		if fi.dim != 0 && dim != fi.dim {
			return simonerr.NewKnowledgeError(
				fmt.Sprintf("index: embedding dimension mismatch: index is %d-dim, got %d-dim (mixing embedding models/providers is not supported)", fi.dim, dim), nil)
		}
		fi.dim = dim
	}

	if err := writeSIDX(filepath.Join(fi.dir, key+".sidx"), vectors); err != nil {
		return err
	}
	if err := writeMeta(filepath.Join(fi.dir, key+".meta.json"), chunks); err != nil {
		return err
	}

	fi.matrices[key] = vectors
	fi.meta[key] = chunks
	fi.sources[key] = source
	return fi.writeManifestLocked()
}

func (fi *FileIndex) removeLocked(key string) error {
	delete(fi.matrices, key)
	delete(fi.meta, key)
	delete(fi.sources, key)
	if err := os.Remove(filepath.Join(fi.dir, key+".sidx")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(filepath.Join(fi.dir, key+".meta.json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (fi *FileIndex) writeManifestLocked() error {
	sources := make(map[string]any, len(fi.sources))
	for key, source := range fi.sources {
		sources[key] = map[string]any{"source": source, "chunk_count": len(fi.meta[key])}
	}
	m := manifest{FormatVersion: 1, Dim: fi.dim, Sources: sources}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(fi.dir, manifestFile), data, 0o644)
}

// Search returns the topK chunks whose vectors have the highest dot product
// with query, across every indexed source (brute force — appropriate for
// this SDK's scale; index.Index stays swappable for an ANN backend later).
func (fi *FileIndex) Search(_ context.Context, query []float32, topK int) ([]SearchResult, error) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	if len(fi.matrices) == 0 {
		return nil, nil
	}

	var results []SearchResult
	for key, matrix := range fi.matrices {
		chunks := fi.meta[key]
		for i, vec := range matrix {
			score := dot(vec, query)
			results = append(results, SearchResult{Text: chunks[i].Text, Source: chunks[i].Source, Score: score})
		}
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if topK < len(results) {
		results = results[:topK]
	}
	return results, nil
}

func dot(a, b []float32) float32 {
	var sum float32
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		sum += a[i] * b[i]
	}
	return sum
}

func writeSIDX(path string, vectors [][]float32) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	dim := 0
	if len(vectors) > 0 {
		dim = len(vectors[0])
	}

	header := make([]byte, 0, 12)
	header = append(header, sidxMagic...)
	header = binary.LittleEndian.AppendUint16(header, sidxVersion)
	header = binary.LittleEndian.AppendUint16(header, uint16(dim))
	header = binary.LittleEndian.AppendUint32(header, uint32(len(vectors)))
	if _, err := f.Write(header); err != nil {
		return err
	}

	buf := make([]byte, 4)
	for _, vec := range vectors {
		for _, v := range vec {
			binary.LittleEndian.PutUint32(buf, math.Float32bits(v))
			if _, err := f.Write(buf); err != nil {
				return err
			}
		}
	}
	return nil
}

func readSIDX(path string) ([][]float32, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 12 || string(data[:4]) != sidxMagic {
		return nil, simonerr.NewKnowledgeError("index: not a valid .sidx file (bad magic)", nil)
	}
	version := binary.LittleEndian.Uint16(data[4:6])
	if version != sidxVersion {
		return nil, simonerr.NewKnowledgeError(fmt.Sprintf("index: unsupported .sidx format version %d", version), nil)
	}
	dim := int(binary.LittleEndian.Uint16(data[6:8]))
	count := int(binary.LittleEndian.Uint32(data[8:12]))

	expected := 12 + count*dim*4
	if len(data) < expected {
		return nil, simonerr.NewKnowledgeError("index: truncated .sidx file", nil)
	}

	vectors := make([][]float32, count)
	offset := 12
	for i := 0; i < count; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
			offset += 4
		}
		vectors[i] = vec
	}
	return vectors, nil
}

func writeMeta(path string, chunks []Chunk) error {
	data, err := json.Marshal(chunks)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func readMeta(path string) ([]Chunk, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var chunks []Chunk
	if err := json.Unmarshal(data, &chunks); err != nil {
		return nil, err
	}
	return chunks, nil
}
