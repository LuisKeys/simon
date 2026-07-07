// Package extract reads plain text out of pdf/docx/xlsx/pptx/plain-text
// files, mirroring KnowledgeBase._read_file in Python's
// simon/knowledge/knowledge.py.
package extract

import (
	"os"
	"path/filepath"
	"strings"
)

// Text extracts the textual content of path, dispatching on its extension.
// Unknown extensions fall back to reading the file as plain UTF-8 text,
// dropping invalid bytes rather than erroring — mirroring Python's
// `path.read_text(encoding="utf-8", errors="ignore")`.
func Text(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return pdfText(path)
	case ".docx":
		return docxText(path)
	case ".xlsx":
		return xlsxText(path)
	case ".pptx":
		return pptxText(path)
	default:
		return plainText(path)
	}
}

func plainText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.ToValidUTF8(string(data), ""), nil
}
