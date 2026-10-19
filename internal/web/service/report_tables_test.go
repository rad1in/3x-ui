package service

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/tableexport"
)

// sheetRows opens a generated workbook and returns each row's cell texts; a
// numeric cell comes back as "#" plus its value.
func sheetRows(t *testing.T, table tableexport.Table) [][]string {
	t.Helper()
	var buf bytes.Buffer
	if err := tableexport.WriteXLSX(&buf, table); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	f, err := zr.Open("xl/worksheets/sheet1.xml")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(f)
	var sheet struct {
		Rows []struct {
			Cells []struct {
				Type string `xml:"t,attr"`
				V    string `xml:"v"`
				T    string `xml:"is>t"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(raw, &sheet); err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for _, r := range sheet.Rows {
		var cells []string
		for _, c := range r.Cells {
			if c.Type == "inlineStr" {
				cells = append(cells, c.T)
			} else {
				cells = append(cells, "#"+c.V)
			}
		}
		out = append(out, cells)
	}
	return out
}

func TestOrdersExportFiltersByStatusAndKeepsSecretsOut(t *testing.T) {
	setupBulkDB(t)
	paidAt := time.Date(2026, 3, 4, 5, 6, 0, 0, time.Local).UnixMilli()
	for _, o := range []model.Order{
		{Token: "secret-token-1", Status: "paid", Kind: "new", PlanName: "=1+1", ClientEmail: "alice", Contact: "@alice", Amount: 250000, Currency: "IRT", PayURL: "https://pay.example/secret", PaidAt: paidAt},
		{Token: "secret-token-2", Status: "pending", Kind: "new", PlanName: "Basic", ClientEmail: "bob", Amount: 100, Currency: "IRT"},
	} {
		if err := database.GetDB().Create(&o).Error; err != nil {
			t.Fatal(err)
		}
	}
	orders, err := (&OrderService{}).ExportOrders("paid")
	if err != nil || len(orders) != 1 {
		t.Fatalf("ExportOrders(paid) = %d orders, %v; want 1", len(orders), err)
	}
	rows := sheetRows(t, OrdersTable(orders, "paid", time.Now()))
	if len(rows) != 2 {
		t.Fatalf("rows = %v", rows)
	}
	flat := strings.Join(rows[0], "|") + "|" + strings.Join(rows[1], "|")
	for _, secret := range []string{"secret-token", "pay.example", "Token"} {
		if strings.Contains(flat, secret) {
			t.Errorf("export leaks %q: %s", secret, flat)
		}
	}
	want := map[string]bool{"'=1+1": true, "'@alice": true, "#250000": true, "2026-03-04 05:06": true}
	for _, cell := range rows[1] {
		delete(want, cell)
	}
	if len(want) != 0 {
		t.Errorf("order row %v is missing cells %v", rows[1], want)
	}
}

func TestInboundTrafficTableCarriesNumbersAndEscapesRemarks(t *testing.T) {
	inbounds := []*model.Inbound{{Id: 3, Remark: "-evil", Protocol: "vless", Port: 443, Enable: true, Up: 100, Down: 200, Total: 0}}
	rows := sheetRows(t, InboundTrafficTable(inbounds, time.Now()))
	got := strings.Join(rows[1], "|")
	if want := "#3|'-evil|vless|#443|true|#100|#200|#0||#0"; got != want {
		t.Fatalf("inbound row = %q, want %q", got, want)
	}
}

func TestReportTableWorkbookEscapesClientNames(t *testing.T) {
	rows := sheetRows(t, ReportTable([]ClientReportRow{{Email: "=cmd|' /C calc'!A0", PeriodUp: 5}}, 30, time.Now()))
	if rows[1][0] != "'=cmd|' /C calc'!A0" || rows[1][3] != "#5" {
		t.Fatalf("client row = %v", rows[1])
	}
}
