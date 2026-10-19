package tableexport

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
)

const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

func xmlText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// columnName turns a zero-based index into A, B, ... Z, AA.
func columnName(i int) string {
	name := ""
	for i++; i > 0; i = (i - 1) / 26 {
		name = string(rune('A'+(i-1)%26)) + name
	}
	return name
}

func stringCell(ref string, style int, s string) string {
	return `<c r="` + ref + `" s="` + strconv.Itoa(style) + `" t="inlineStr"><is><t xml:space="preserve">` + xmlText(s) + `</t></is></c>`
}

func columnWidth(t Table, col int) int {
	w := len([]rune(t.Columns[col].Header))
	for _, r := range t.Rows {
		if col < len(r) {
			w = max(w, len([]rune(plainText(r[col]))))
		}
	}
	return min(max(w+2, 8), 60)
}

func sheetXML(t Table) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	b.WriteString(`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`)
	b.WriteString(`<cols>`)
	for i := range t.Columns {
		n := strconv.Itoa(i + 1)
		b.WriteString(`<col min="` + n + `" max="` + n + `" width="` + strconv.Itoa(columnWidth(t, i)) + `" customWidth="1"/>`)
	}
	b.WriteString(`</cols><sheetData><row r="1">`)
	for i, c := range t.Columns {
		b.WriteString(stringCell(columnName(i)+"1", 1, EscapeFormula(c.Header)))
	}
	b.WriteString(`</row>`)
	for ri, row := range t.Rows {
		rn := strconv.Itoa(ri + 2)
		b.WriteString(`<row r="` + rn + `">`)
		for ci := range t.Columns {
			if ci >= len(row) {
				continue
			}
			ref := columnName(ci) + rn
			if isNumber(row[ci]) {
				b.WriteString(`<c r="` + ref + `" s="0"><v>` + numberText(row[ci]) + `</v></c>`)
			} else {
				b.WriteString(stringCell(ref, 0, EscapeFormula(plainText(row[ci]))))
			}
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

// sheetName keeps to what Excel accepts: at most 31 characters, none of []:*?/\.
func sheetName(title string) string {
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) {
			return ' '
		}
		return r
	}, title)
	clean = strings.TrimSpace(clean)
	if clean == "" {
		clean = "Report"
	}
	if r := []rune(clean); len(r) > 31 {
		clean = string(r[:31])
	}
	return clean
}

// WriteXLSX writes the table as a one-sheet workbook. Text is stored as string
// cells with formula triggers quoted; numbers stay numeric.
func WriteXLSX(w io.Writer, t Table) error {
	parts := []struct{ name, body string }{
		{"[Content_Types].xml", xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`},
		{"_rels/.rels", xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", xmlHeader + `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="` + xmlText(sheetName(t.Title)) + `" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`},
		{"xl/styles.xml", xmlHeader + `<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts><fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/></cellXfs></styleSheet>`},
		{"xl/worksheets/sheet1.xml", sheetXML(t)},
	}
	zw := zip.NewWriter(w)
	for _, p := range parts {
		f, err := zw.Create(p.name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(f, p.body); err != nil {
			return err
		}
	}
	return zw.Close()
}
