// Надёжный парсер .xls: фиксируем ширину таблицы сами и читаем все ячейки до неё.
package fileio

import (
	"bytes"
	"errors"
	"io"

	xls "github.com/extrame/xls"
)

// вычисляем "реальную" ширину: пробегаем разумное число колонок и ищем непустые
func computeMaxCols(sheet *xls.WorkSheet, headerRow int) int {
	const probeMax = 512
	maxCols := 0

	hdr0 := headerRow - 1
	if hdr0 < 0 {
		hdr0 = 0
	}
	checkRow := func(i int) {
		if i < 0 || i > int(sheet.MaxRow) {
			return
		}
		r := sheet.Row(i)
		if r == nil {
			return
		}
		for j := 0; j < probeMax; j++ {
			if v := normalizeCell(r.Col(j)); v != "" {
				if j+1 > maxCols {
					maxCols = j + 1
				}
			}
		}
	}

	// шапка и строка под ней — часто самые широкие
	checkRow(hdr0)
	checkRow(hdr0 + 1)
	// общий проход
	for i := 0; i <= int(sheet.MaxRow); i++ {
		checkRow(i)
	}
	if maxCols == 0 {
		maxCols = 1
	}
	return maxCols
}

func readXLS(r io.Reader, headerRow int) ([]map[string]string, error) {
	if headerRow <= 0 {
		return nil, errors.New("headerRow must be 1-based and >= 1")
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// .xls из 1С чаще всего cp1251, но иногда UTF-8/KOI8-R
	var wb *xls.WorkBook
	tryCharsets := []string{"windows-1251", "utf-8", "koi8-r"}
	var lastErr error
	for _, ch := range tryCharsets {
		wb, err = xls.OpenReader(bytes.NewReader(b), ch)
		if err == nil && wb != nil {
			lastErr = nil
			break
		}
		lastErr = err
	}
	if wb == nil {
		if lastErr == nil {
			lastErr = errors.New("xls: failed to open workbook")
		}
		return nil, lastErr
	}

	sheet := wb.GetSheet(0)
	if sheet == nil {
		return nil, nil
	}

	// фиксируем ширину и читаем все строки до неё (НЕ полагаемся на Row.LastCol())
	maxCols := computeMaxCols(sheet, headerRow)
	rows := make([][]string, 0, int(sheet.MaxRow)+1)
	for i := 0; i <= int(sheet.MaxRow); i++ {
		row := sheet.Row(i)
		cols := make([]string, maxCols)
		if row != nil {
			for j := 0; j < maxCols; j++ {
				cols[j] = normalizeCell(row.Col(j)) // безопасно: пустые -> ""
			}
		}
		rows = append(rows, cols)
	}

	// общий pickHeader объединит верх+низ шапки, затем rowsToMaps соберёт записи
	h := pickHeader(rows, headerRow)
	return rowsToMaps(rows, h, headerRow), nil
}
