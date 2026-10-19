package tableexport

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// A4 landscape in points, Helvetica (a base-14 font, so nothing is embedded).
const (
	pageW     = 842.0
	pageH     = 595.0
	margin    = 36.0
	fontSize  = 8.0
	rowH      = 13.0
	titleSize = 14.0
)

// glyphWidth approximates Helvetica advance widths in 1/1000 em by class;
// exact metrics are not needed to keep text inside its column.
func glyphWidth(r rune) float64 {
	switch {
	case strings.ContainsRune("iljI.,:;'|!() ", r):
		return 0.28
	case strings.ContainsRune("frt", r):
		return 0.34
	case strings.ContainsRune("mwMW@%", r):
		return 0.85
	case r >= 'A' && r <= 'Z':
		return 0.68
	default:
		return 0.54
	}
}

func textWidth(s string, size float64) float64 {
	w := 0.0
	for _, r := range s {
		w += glyphWidth(r)
	}
	return w * size
}

// pdfLatin1 maps text to what WinAnsi Helvetica can draw: printable ASCII and
// Latin-1 stay, everything else becomes ?.
func pdfLatin1(s string) []rune {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 0x20 && r <= 0x7e, r >= 0xa0 && r <= 0xff:
			out = append(out, r)
		case r == '\t' || r == '\n' || r == '\r':
			out = append(out, ' ')
		default:
			out = append(out, '?')
		}
	}
	return out
}

func fit(s string, width, size float64) string {
	runes := pdfLatin1(s)
	if textWidth(string(runes), size) <= width {
		return string(runes)
	}
	for len(runes) > 0 && textWidth(string(runes)+"...", size) > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "..."
}

func pdfString(s string) string {
	var b strings.Builder
	b.WriteByte('(')
	for _, r := range s {
		switch {
		case r == '(' || r == ')' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r > 0x7e:
			fmt.Fprintf(&b, "\\%03o", r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte(')')
	return b.String()
}

func cellDisplay(c Column, v any) string {
	if c.Display != nil {
		return c.Display(v)
	}
	return plainText(v)
}

// columnWidths shares the printable width out by each column's longest text,
// capped so one long column cannot starve the rest.
func columnWidths(t Table) []float64 {
	avail := pageW - 2*margin
	want := make([]float64, len(t.Columns))
	total := 0.0
	for i, c := range t.Columns {
		w := textWidth(string(pdfLatin1(c.Header)), fontSize)
		for _, r := range t.Rows {
			if i < len(r) {
				w = max(w, textWidth(string(pdfLatin1(cellDisplay(c, r[i]))), fontSize))
			}
		}
		want[i] = min(w+10, avail/2)
		total += want[i]
	}
	if total > avail {
		for i := range want {
			want[i] *= avail / total
		}
	}
	return want
}

// WritePDF writes the table as a paginated A4 landscape PDF with the header
// row repeated on every page. Text is limited to Latin-1.
func WritePDF(w io.Writer, t Table) error {
	widths := columnWidths(t)
	usable := pageH - 2*margin - titleSize - rowH*2
	rowsPerPage := int(usable/rowH) - 1
	var streams []string
	for start := 0; start == 0 || start < len(t.Rows); start += rowsPerPage {
		var s strings.Builder
		y := pageH - margin
		s.WriteString("BT /F1 " + fmt.Sprint(titleSize) + " Tf " + fmt.Sprintf("%.1f %.1f Td ", margin, y-titleSize) + pdfString(fit(t.Title, pageW-2*margin, titleSize)) + " Tj ET\n")
		y -= titleSize + 4
		if t.Subtitle != "" {
			s.WriteString("BT /F1 " + fmt.Sprint(fontSize) + " Tf 0.4 g " + fmt.Sprintf("%.1f %.1f Td ", margin, y-fontSize) + pdfString(fit(t.Subtitle, pageW-2*margin, fontSize)) + " Tj 0 g ET\n")
		}
		y -= rowH + 4
		drawRow := func(cells []string, bold bool, right []bool) {
			x := margin
			font := "/F1"
			if bold {
				font = "/F2"
			}
			for i, text := range cells {
				text = fit(text, widths[i]-8, fontSize)
				tx := x + 4
				if right[i] {
					tx = x + widths[i] - 4 - textWidth(text, fontSize)
				}
				s.WriteString("BT " + font + " " + fmt.Sprint(fontSize) + " Tf " + fmt.Sprintf("%.1f %.1f Td ", tx, y-fontSize-1) + pdfString(text) + " Tj ET\n")
				x += widths[i]
			}
			s.WriteString(fmt.Sprintf("0.8 G %.1f %.1f m %.1f %.1f l S\n", margin, y-rowH+1, pageW-margin, y-rowH+1))
			y -= rowH
		}
		headers := make([]string, len(t.Columns))
		noRight := make([]bool, len(t.Columns))
		for i, c := range t.Columns {
			headers[i] = c.Header
		}
		drawRow(headers, true, noRight)
		for r := start; r < min(start+rowsPerPage, len(t.Rows)); r++ {
			cells := make([]string, len(t.Columns))
			right := make([]bool, len(t.Columns))
			for i, c := range t.Columns {
				if i < len(t.Rows[r]) {
					cells[i] = cellDisplay(c, t.Rows[r][i])
					right[i] = isNumber(t.Rows[r][i])
				}
			}
			drawRow(cells, false, right)
		}
		streams = append(streams, s.String())
	}

	// Objects: 1 catalog, 2 pages, 3 and 4 fonts, then a page and its content per page.
	var out bytes.Buffer
	offsets := []int{}
	obj := func(body string) {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	out.WriteString("%PDF-1.4\n")
	kids := make([]string, len(streams))
	for i := range streams {
		kids[i] = fmt.Sprintf("%d 0 R", 5+2*i)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(streams)))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	for i, s := range streams {
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>", pageW, pageH, 6+2*i))
		obj(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(s), s))
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	_, err := w.Write(out.Bytes())
	return err
}
