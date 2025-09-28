package fileio

import (
	"bytes"
	"io"
	"strings"

	excelize "github.com/xuri/excelize/v2"
)

// readXLSX читает лист XLSX так, чтобы числа приходили сырыми, а формулы — рассчитанными.
// headerRow — 1-based индекс строки заголовков.
func readXLSX(r io.Reader, headerRow int) ([]map[string]string, error) {
	if headerRow <= 0 {
		headerRow = 1
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// выберем активный лист (иначе первый)
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, nil
	}
	sheet := sheets[0]
	if idx := f.GetActiveSheetIndex(); idx >= 0 && idx < len(sheets) {
		sheet = sheets[idx]
	}

	// определяем границы листа
	dim, err := f.GetSheetDimension(sheet)
	if err != nil || dim == "" {
		// запасной режим — построчно
		rows, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, err
		}
		for i := range rows {
			for j := range rows[i] {
				rows[i][j] = normalizeCell(rows[i][j])
			}
		}
		h := pickHeader(rows, headerRow)
		return rowsToMaps(rows, h, headerRow), nil
	}

	// dim = "A1:K234" → конца координаты
	parts := strings.Split(dim, ":")
	end := parts[len(parts)-1]
	lastCol, lastRow, err := excelize.CellNameToCoordinates(end)
	if err != nil {
		return nil, err
	}

	rows := make([][]string, lastRow)
	for rIdx := 1; rIdx <= lastRow; rIdx++ {
		row := make([]string, lastCol)
		for cIdx := 1; cIdx <= lastCol; cIdx++ {
			addr, _ := excelize.CoordinatesToCellName(cIdx, rIdx)

			// 1) пробуем честно посчитать формулу
			// 2) если не формула или Calc не поддерживается — берём raw
			// 3) если raw пусто — берём форматированное (как видит пользователь)
			val := ""
			if ct, _ := f.GetCellType(sheet, addr); ct == excelize.CellTypeFormula {
				if s, err := f.CalcCellValue(sheet, addr); err == nil && s != "" {
					val = s
				}
			}
			if val == "" {
				if s, _ := f.GetCellValue(sheet, addr, excelize.Options{RawCellValue: true}); s != "" {
					val = s
				}
			}
			if val == "" {
				// форматированное (может вернуть "495 558,073" — потом нормализуем)
				val, _ = f.GetCellValue(sheet, addr)
			}

			row[cIdx-1] = normalizeCell(val)
		}
		rows[rIdx-1] = row
	}

	h := pickHeader(rows, headerRow)
	return rowsToMaps(rows, h, headerRow), nil
}
