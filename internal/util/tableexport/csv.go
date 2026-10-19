package tableexport

import (
	"encoding/csv"
	"io"
)

// WriteCSV writes the raw values with formula triggers quoted, like the workbook.
func WriteCSV(w io.Writer, t Table) error {
	cw := csv.NewWriter(w)
	header := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		header[i] = EscapeFormula(c.Header)
	}
	_ = cw.Write(header)
	for _, row := range t.Rows {
		rec := make([]string, len(t.Columns))
		for i := range t.Columns {
			if i < len(row) {
				rec[i] = plainText(row[i])
				if !isNumber(row[i]) {
					rec[i] = EscapeFormula(rec[i])
				}
			}
		}
		_ = cw.Write(rec)
	}
	cw.Flush()
	return cw.Error()
}
