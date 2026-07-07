package extract

import (
	"io"

	"github.com/ledongthuc/pdf"

	"simon-go/pkg/simonerr"
)

func pdfText(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", simonerr.NewKnowledgeError("extract: opening PDF failed", err)
	}
	defer f.Close()

	reader, err := r.GetPlainText()
	if err != nil {
		return "", simonerr.NewKnowledgeError("extract: reading PDF text failed", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
