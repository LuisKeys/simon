package extract

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestPlainTextFallbackForUnknownExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("hello\nworld"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Text(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello\nworld" {
		t.Errorf("got %q", got)
	}
}

func TestXlsxTextJoinsRowsTabSeparated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sheet.xlsx")

	f := excelize.NewFile()
	defer f.Close()
	_ = f.SetSheetRow("Sheet1", "A1", &[]any{"name", "age"})
	_ = f.SetSheetRow("Sheet1", "A2", &[]any{"Ada", 36})
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}

	got, err := Text(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "name\tage") || !strings.Contains(got, "Ada\t36") {
		t.Errorf("got %q", got)
	}
}

func TestDocxTextExtractsParagraphs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.docx")
	writeMinimalDocx(t, path, []string{"Hello there", "Second paragraph"})

	got, err := Text(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hello there\nSecond paragraph" {
		t.Errorf("got %q", got)
	}
}

func TestPptxTextExtractsSlidesInOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deck.pptx")
	writeMinimalPptx(t, path, []string{"Slide one text", "Slide two text"})

	got, err := Text(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Slide one text\nSlide two text" {
		t.Errorf("got %q", got)
	}
}

// writeMinimalDocx builds a bare-bones .docx (a zip containing just
// word/document.xml) with one <w:p><w:r><w:t> run per paragraph — enough to
// exercise docxText without depending on a real Word installation.
func writeMinimalDocx(t *testing.T, path string, paragraphs []string) {
	t.Helper()
	var body strings.Builder
	body.WriteString(`<?xml version="1.0"?><w:document xmlns:w="ns"><w:body>`)
	for _, p := range paragraphs {
		body.WriteString(`<w:p><w:r><w:t>` + p + `</w:t></w:r></w:p>`)
	}
	body.WriteString(`</w:body></w:document>`)
	writeZip(t, path, map[string]string{"word/document.xml": body.String()})
}

// writeMinimalPptx builds a bare-bones .pptx with one slideN.xml per entry
// in texts, each containing a single <a:p><a:r><a:t> run.
func writeMinimalPptx(t *testing.T, path string, texts []string) {
	t.Helper()
	files := map[string]string{}
	for i, text := range texts {
		xml := `<?xml version="1.0"?><p:sld xmlns:a="ns" xmlns:p="ns"><p:cSld><p:spTree>` +
			`<p:sp><p:txBody><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></p:txBody></p:sp>` +
			`</p:spTree></p:cSld></p:sld>`
		files["ppt/slides/slide"+strconv.Itoa(i+1)+".xml"] = xml
	}
	writeZip(t, path, files)
}

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}
