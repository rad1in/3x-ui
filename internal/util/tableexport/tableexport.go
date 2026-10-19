// Package tableexport writes one table as a minimal .xlsx workbook or a plain
// tabular PDF, using only the standard library.
package tableexport

import (
	"fmt"
	"strconv"
)

// Column is one table column. Display renders a cell for the PDF; the
// workbook always carries the raw value.
type Column struct {
	Header  string
	Display func(v any) string
}

// Table is a titled grid. Cell values are strings, integers, floats or bools.
type Table struct {
	Title    string
	Subtitle string
	Columns  []Column
	Rows     [][]any
}

// EscapeFormula quotes a leading = + - @ tab or CR so a spreadsheet shows the
// text instead of running it, as OWASP's CSV-injection advice says.
func EscapeFormula(s string) string {
	if s != "" && (s[0] == '=' || s[0] == '+' || s[0] == '-' || s[0] == '@' || s[0] == '\t' || s[0] == '\r') {
		return "'" + s
	}
	return s
}

func isNumber(v any) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	}
	return false
}

func numberText(v any) string {
	switch n := v.(type) {
	case float32:
		return strconv.FormatFloat(float64(n), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(n, 'g', -1, 64)
	default:
		return fmt.Sprint(n)
	}
}

func plainText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	}
	if isNumber(v) {
		return numberText(v)
	}
	return fmt.Sprint(v)
}
