package controller

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/util/tableexport"
)

func TestServeTableSendsEachFormatAsADownload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	table := tableexport.Table{Title: "T", Columns: []tableexport.Column{{Header: "a"}}, Rows: [][]any{{"=1"}}}
	cases := []struct{ format, ctype, ext string }{
		{"csv", "text/csv; charset=utf-8", ".csv"},
		{"xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx"},
		{"pdf", "application/pdf", ".pdf"},
	}
	for _, tc := range cases {
		t.Run(tc.format, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			serveTable(c, tc.format, "report", table)
			if got := w.Header().Get("Content-Type"); got != tc.ctype {
				t.Errorf("Content-Type = %q, want %q", got, tc.ctype)
			}
			if got := w.Header().Get("Content-Disposition"); got != `attachment; filename="report`+tc.ext+`"` {
				t.Errorf("Content-Disposition = %q", got)
			}
			body := w.Body.Bytes()
			switch tc.format {
			case "xlsx":
				if _, err := zip.NewReader(bytes.NewReader(body), int64(len(body))); err != nil {
					t.Errorf("xlsx body is not a zip: %v", err)
				}
			case "pdf":
				if !strings.HasPrefix(string(body), "%PDF-") {
					t.Errorf("pdf body starts %q", body[:8])
				}
			default:
				if !strings.Contains(string(body), "'=1") {
					t.Errorf("csv body %q lacks the escaped formula", body)
				}
			}
		})
	}
}
