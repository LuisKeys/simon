package extract

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"simon-go/pkg/simonerr"
)

// extractParagraphText reads xmlPath out of the OOXML zip archive at
// zipPath and returns one line per paragraph element (local name "p"),
// concatenating the text of every text element (local name "t") within it —
// covering both Word's <w:p>/<w:t> and PowerPoint's DrawingML <a:p>/<a:t>
// paragraph/text-run model with the same walker.
func extractParagraphText(zipPath string, xmlPaths []string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", simonerr.NewKnowledgeError("extract: opening OOXML archive failed", err)
	}
	defer zr.Close()

	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}

	var lines []string
	for _, path := range xmlPaths {
		f, ok := files[path]
		if !ok {
			continue
		}
		fileLines, err := paragraphsOf(f)
		if err != nil {
			return "", err
		}
		lines = append(lines, fileLines...)
	}
	return strings.Join(lines, "\n"), nil
}

func paragraphsOf(f *zip.File) ([]string, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	var lines []string
	var current strings.Builder
	inText := false

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, simonerr.NewKnowledgeError("extract: malformed OOXML XML", err)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			if el.Name.Local == "t" {
				inText = true
			}
		case xml.EndElement:
			if el.Name.Local == "t" {
				inText = false
			}
			if el.Name.Local == "p" {
				lines = append(lines, current.String())
				current.Reset()
			}
		case xml.CharData:
			if inText {
				current.Write(el)
			}
		}
	}
	return lines, nil
}

func docxText(path string) (string, error) {
	return extractParagraphText(path, []string{"word/document.xml"})
}

func pptxText(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", simonerr.NewKnowledgeError("extract: opening PPTX archive failed", err)
	}
	var slidePaths []string
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
			slidePaths = append(slidePaths, f.Name)
		}
	}
	zr.Close()
	sortSlidesBySlideNumber(slidePaths)

	return extractParagraphText(path, slidePaths)
}

var slideNumberRe = regexp.MustCompile(`slide(\d+)\.xml$`)

// sortSlidesBySlideNumber orders "ppt/slides/slideN.xml" paths numerically
// by N, since a plain lexicographic sort would put slide10 before slide2.
func sortSlidesBySlideNumber(paths []string) {
	sort.Slice(paths, func(i, j int) bool {
		return slideNumber(paths[i]) < slideNumber(paths[j])
	})
}

func slideNumber(path string) int {
	m := slideNumberRe.FindStringSubmatch(path)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}
