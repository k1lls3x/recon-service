package fileio

import (
	"bytes"
	"io"

	xls "github.com/extrame/xls"
)

func readXLS(r io.Reader, headerRow int) ([]map[string]string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	wb, err := xls.OpenReader(bytes.NewReader(b), "windows-1251")
	if err != nil {
	  wb, err  = xls.OpenReader(bytes.NewReader(b), "utf-8")
		if err != nil {
			return nil, err
		}
	}
	sheet := wb.GetSheet(0)
	if sheet == nil {
		return nil, nil
	}

	rows := make([][]string, 0, sheet.MaxRow)
	for i := 0; i <= int(sheet.MaxRow); i++ {
		row := sheet.Row(i)
		if row == nil {
			continue
		}
		cols := make([]string, row.LastCol())
		for j := 0; j < row.LastCol(); j++ {
			cols[j] = row.Col(j)
		}
		rows = append(rows, cols)
	}
	h := pickHeader(rows, headerRow)
	return rowsToMaps(rows, h, headerRow), nil
}
