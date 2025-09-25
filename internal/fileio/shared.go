package fileio

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// ReadAnyMaps — выберет парсер по расширению и вернёт строки как срез map[header]value.
// headerRow — номер строки заголовков (1-based).
func ReadAnyMaps(r io.Reader, filename string, headerRow int) ([]map[string]string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".xlsx":
		return readXLSX(r, headerRow)
	case ".xls":
		return readXLS(r, headerRow)
	case ".csv":
		return readCSV(r, headerRow)
	default:
		return nil, fmt.Errorf("unsupported file: %s", filename)
	}
}

// pickHeader — берёт строку заголовков и подставляет Column N для пустых.
func pickHeader(rows [][]string, headerRow int) []string {
	idx := headerRow - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(rows) {
		idx = 0
	}
	h := rows[idx]
	out := make([]string, len(h))
	for i, v := range h {
		v = strings.TrimSpace(v)
		if v == "" {
			v = fmt.Sprintf("Column %d", i+1)
		}
		out[i] = v
	}
	return out
}

// rowsToMaps — конвертирует AoA в []map по заголовкам, пропуская полностью пустые строки.
func rowsToMaps(rows [][]string, headers []string, headerRow int) []map[string]string {
	start := headerRow // первая строка после заголовков
	var out []map[string]string
	for r := start; r < len(rows); r++ {
		rec := rows[r]
		m := map[string]string{}
		for c := 0; c < len(headers); c++ {
			var v string
			if c < len(rec) {
				v = rec[c]
			}
			m[headers[c]] = v
		}
		empty := true
		for _, v := range m {
			if strings.TrimSpace(v) != "" {
				empty = false
				break
			}
		}
		if !empty {
			out = append(out, m)
		}
	}
	return out
}
