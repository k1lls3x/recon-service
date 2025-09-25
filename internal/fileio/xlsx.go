package fileio

import (
	"bytes"
	"io"

	excelize "github.com/xuri/excelize/v2"
)

func readXLSX(r io.Reader, headerRow int) ([]map[string]string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, err
	}
	h := pickHeader(rows, headerRow)
	return rowsToMaps(rows, h, headerRow), nil
}
