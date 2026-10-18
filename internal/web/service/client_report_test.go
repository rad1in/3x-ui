package service

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func seedReport(t *testing.T) {
	t.Helper()
	seedUsageClient(t, 23101, "alice")
	seedUsageClient(t, 23102, "=cmd|' /C calc'!A0")
	now := time.Now()
	for _, u := range []model.ClientDailyUsage{
		{Email: "alice", Day: now.Format("2006-01-02"), Up: 10, Down: 20},
		{Email: "alice", Day: now.AddDate(0, 0, -2).Format("2006-01-02"), Up: 1, Down: 2},
		{Email: "alice", Day: now.AddDate(0, 0, -40).Format("2006-01-02"), Up: 1000, Down: 1000},
	} {
		if err := database.GetDB().Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestUsageReportSumsOnlyTheRequestedWindow(t *testing.T) {
	setupBulkDB(t)
	seedReport(t)

	rows, err := (&ClientService{}).UsageReport(nil, 7)
	if err != nil {
		t.Fatal(err)
	}
	byEmail := map[string]ClientReportRow{}
	for _, r := range rows {
		byEmail[r.Email] = r
	}
	if a := byEmail["alice"]; a.PeriodUp != 11 || a.PeriodDown != 22 {
		t.Fatalf("alice window = %+v, want up 11 down 22 (the 40-day-old row excluded)", a)
	}
	if b := byEmail["=cmd|' /C calc'!A0"]; b.PeriodUp != 0 || b.PeriodDown != 0 {
		t.Fatalf("a client without usage = %+v, want zeros", b)
	}
}

func TestReportCSVNeutralisesSpreadsheetFormulas(t *testing.T) {
	var buf bytes.Buffer
	rows := []ClientReportRow{
		{Email: "=cmd|' /C calc'!A0", Group: "+sum", Enable: true, PeriodUp: 1, PeriodDown: 2, TotalUp: 3, TotalDown: 4, Quota: 5, ExpiryTime: 0},
		{Email: "bob", Enable: false, ExpiryTime: time.Date(2027, 1, 2, 12, 0, 0, 0, time.UTC).UnixMilli()},
	}
	if err := WriteReportCSV(&buf, rows); err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || len(recs[0]) != 9 {
		t.Fatalf("records = %v, want a header and two rows of 9 columns", recs)
	}
	if got := recs[1][0]; got != "'=cmd|' /C calc'!A0" {
		t.Fatalf("email cell = %q, want the formula prefixed with a quote", got)
	}
	if got := recs[1][1]; got != "'+sum" {
		t.Fatalf("group cell = %q, want the formula prefixed with a quote", got)
	}
	want := []string{"bob", "", "false", "0", "0", "0", "0", "0", "2027-01-02"}
	if strings.Join(recs[2], "|") != strings.Join(want, "|") {
		t.Fatalf("row = %v, want %v", recs[2], want)
	}
}

func TestReportHTMLEscapesClientNames(t *testing.T) {
	var buf bytes.Buffer
	rows := []ClientReportRow{{Email: "<script>alert(1)</script>", Enable: true, PeriodDown: 2048}}
	if err := WriteReportHTML(&buf, rows, 30, time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "<script>alert(1)</script>") || !strings.Contains(out, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("client name not escaped in %s", out)
	}
	if !strings.Contains(out, "2.00KB") || !strings.Contains(out, "2026-10-18") || !strings.Contains(out, "30") {
		t.Fatalf("report lacks the usage, date or window: %s", out)
	}
}
