package fileio

import (
	"bufio"
	"encoding/csv"
	"io"
	"strings"

	"github.com/saintfish/chardet"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// readCSV reads CSV with headerRow (1-based), auto-detecting encoding and converting to UTF-8.
// It supports UTF-8 and Windows-1251 out of the box.
func readCSV(r io.Reader, headerRow int) ([]map[string]string, error) {
	br := bufio.NewReader(r)

	// Peek a bit to detect encoding
	peek, _ := br.Peek(2048)
	cs := "utf-8"
	if len(peek) > 0 {
		if det, err := chardet.NewTextDetector().DetectBest(peek); err == nil && det != nil {
			cs = strings.ToLower(det.Charset)
		}
	}

	var dec io.Reader = br
	switch cs {
	case "windows-1251", "cp1251":
		dec = transform.NewReader(br, charmap.Windows1251.NewDecoder())
	default:
		// assume UTF-8
	}

	cr := csv.NewReader(dec)
	cr.FieldsPerRecord = -1

	var rows [][]string
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, rec)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	h := pickHeader(rows, headerRow)
	return rowsToMaps(rows, h, headerRow), nil
}
