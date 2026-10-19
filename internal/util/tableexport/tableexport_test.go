package tableexport

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

type xlsxCell struct {
	Ref  string `xml:"r,attr"`
	Type string `xml:"t,attr"`
	V    string `xml:"v"`
	T    string `xml:"is>t"`
}

type xlsxSheet struct {
	Rows []struct {
		Cells []xlsxCell `xml:"c"`
	} `xml:"sheetData>row"`
}

func readXLSX(t *testing.T, data []byte) (map[string]string, xlsxSheet) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("xlsx is not a zip archive: %v", err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(b)
	}
	var sheet xlsxSheet
	if err := xml.Unmarshal([]byte(files["xl/worksheets/sheet1.xml"]), &sheet); err != nil {
		t.Fatalf("sheet1.xml is not valid XML: %v", err)
	}
	return files, sheet
}

func sampleTable() Table {
	return Table{
		Title:    "Usage",
		Subtitle: "last 30 days",
		Columns:  []Column{{Header: "email"}, {Header: "bytes"}, {Header: "enabled"}},
		Rows: [][]any{
			{"alice@example.com", int64(1048576), true},
			{"=HYPERLINK(\"http://evil\",\"x\")", int64(-5), false},
			{"+1+1", 3.5, true},
			{"-2+3", 0, true},
			{"@SUM(A1)", 1, true},
			{"a <b> & \"c\"", 2, true},
		},
	}
}

func TestXLSXHoldsHeaderNumbersAndEscapedText(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteXLSX(&buf, sampleTable()); err != nil {
		t.Fatal(err)
	}
	files, sheet := readXLSX(t, buf.Bytes())
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels", "xl/styles.xml"} {
		if files[name] == "" {
			t.Errorf("xlsx lacks %s", name)
		}
	}
	if len(sheet.Rows) != 7 {
		t.Fatalf("sheet has %d rows, want header + 6", len(sheet.Rows))
	}
	text := func(r, c int) string { return sheet.Rows[r].Cells[c].T }
	if text(0, 0) != "email" || text(0, 1) != "bytes" {
		t.Errorf("header = %q, %q", text(0, 0), text(0, 1))
	}
	if got := text(1, 0); got != "alice@example.com" {
		t.Errorf("plain text cell = %q", got)
	}
	if c := sheet.Rows[1].Cells[1]; c.Type != "" || c.V != strconv.Itoa(1048576) {
		t.Errorf("number cell = %+v, want numeric 1048576", c)
	}
	if c := sheet.Rows[2].Cells[1]; c.Type != "" || c.V != "-5" {
		t.Errorf("negative number must stay a number, got %+v", c)
	}
	if c := sheet.Rows[3].Cells[1]; c.V != "3.5" {
		t.Errorf("float cell = %+v", c)
	}
	if got := sheet.Rows[1].Cells[2].T; got != "true" {
		t.Errorf("bool cell = %q", got)
	}
	for r, want := range map[int]string{
		2: "'=HYPERLINK(\"http://evil\",\"x\")",
		3: "'+1+1",
		4: "'-2+3",
		5: "'@SUM(A1)",
	} {
		if got := text(r, 0); got != want {
			t.Errorf("row %d text = %q, want formula prefixed with a quote: %q", r, got, want)
		}
		if sheet.Rows[r].Cells[0].Type != "inlineStr" {
			t.Errorf("row %d text cell must be a string cell", r)
		}
	}
	if got := text(6, 0); got != `a <b> & "c"` {
		t.Errorf("markup characters did not survive: %q", got)
	}
	if strings.Contains(files["xl/worksheets/sheet1.xml"], "<f>") {
		t.Error("sheet contains a formula element")
	}
}

func TestEscapeFormulaPrefixesEveryTrigger(t *testing.T) {
	for in, want := range map[string]string{
		"=1": "'=1", "+1": "'+1", "-1": "'-1", "@x": "'@x", "\tx": "'\tx", "\rx": "'\rx",
		"safe": "safe", "": "", "a=b": "a=b",
	} {
		if got := EscapeFormula(in); got != want {
			t.Errorf("EscapeFormula(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPDFIsStructurallyValidAndPaginates(t *testing.T) {
	tbl := Table{Title: "Orders (all)", Subtitle: "by \\ day", Columns: []Column{{Header: "id"}, {Header: "name"}}}
	for i := 0; i < 120; i++ {
		tbl.Rows = append(tbl.Rows, []any{i, "name (" + strconv.Itoa(i) + ") ünï"})
	}
	var buf bytes.Buffer
	if err := WritePDF(&buf, tbl); err != nil {
		t.Fatal(err)
	}
	pdf := buf.String()
	if !strings.HasPrefix(pdf, "%PDF-1.4") || !strings.HasSuffix(strings.TrimSpace(pdf), "%%EOF") {
		t.Fatal("missing PDF header or EOF marker")
	}
	start := regexp.MustCompile(`startxref\n(\d+)\n%%EOF`).FindStringSubmatch(pdf)
	if start == nil {
		t.Fatal("no startxref")
	}
	xrefAt, _ := strconv.Atoi(start[1])
	if !strings.HasPrefix(pdf[xrefAt:], "xref\n") {
		t.Fatalf("startxref points at %q", pdf[xrefAt:xrefAt+10])
	}
	entries := regexp.MustCompile(`(\d{10}) 00000 n \n`).FindAllStringSubmatch(pdf[xrefAt:], -1)
	if len(entries) < 4 {
		t.Fatalf("xref has %d entries", len(entries))
	}
	for i, e := range entries {
		off, _ := strconv.Atoi(e[1])
		if want := strconv.Itoa(i+1) + " 0 obj"; !strings.HasPrefix(pdf[off:], want) {
			t.Errorf("xref entry %d points at %q, want %q", i+1, pdf[off:off+12], want)
		}
	}
	pages := regexp.MustCompile(`/Type /Pages /Kids \[[^\]]*\] /Count (\d+)`).FindStringSubmatch(pdf)
	if pages == nil || pages[1] == "1" {
		t.Fatalf("120 rows must span several pages, got %v", pages)
	}
	if !strings.Contains(pdf, `Orders \(all\)`) || !strings.Contains(pdf, `by \\ day`) {
		t.Error("title parentheses or backslash were not escaped in the content stream")
	}
	if !strings.Contains(pdf, `name \(7\) \374n\357`) {
		t.Error("Latin-1 text was not written as WinAnsi octal escapes")
	}
}

func TestPDFReplacesTextHelveticaCannotDraw(t *testing.T) {
	var buf bytes.Buffer
	err := WritePDF(&buf, Table{Columns: []Column{{Header: "n"}}, Rows: [][]any{{"سلام"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "(????)") {
		t.Error("characters outside Latin-1 should become ? so the file stays valid")
	}
}
