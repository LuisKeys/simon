package extract

import (
	"strings"

	"github.com/xuri/excelize/v2"

	"simon-go/pkg/simonerr"
)

// xlsxText joins every sheet's rows tab-separated, one line per non-blank
// row, mirroring KnowledgeBase._read_file's openpyxl-based .xlsx handling.
func xlsxText(path string) (string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return "", simonerr.NewKnowledgeError("extract: opening XLSX failed", err)
	}
	defer f.Close()

	var lines []string
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			return "", simonerr.NewKnowledgeError("extract: reading XLSX sheet failed", err)
		}
		for _, row := range rows {
			line := strings.Join(row, "\t")
			if strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		}
	}
	return strings.Join(lines, "\n"), nil
}
