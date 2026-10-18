package service

import (
	"encoding/csv"
	"html/template"
	"io"
	"strconv"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// ClientReportRow is one client's usage over the report window plus its
// lifetime totals, quota and expiry.
type ClientReportRow struct {
	Email      string
	Group      string
	Enable     bool
	PeriodUp   int64
	PeriodDown int64
	TotalUp    int64
	TotalDown  int64
	Quota      int64
	ExpiryTime int64
}

type periodUsage struct {
	Email string
	Up    int64
	Down  int64
}

// UsageReport lists the clients visible to scope with their usage over the last
// `days` days (today included), summed from the per-day usage rows.
func (s *ClientService) UsageReport(scope *AdminScope, days int) ([]ClientReportRow, error) {
	clients, err := s.List(scope)
	if err != nil {
		return nil, err
	}
	since := time.Now().AddDate(0, 0, -(max(days, 1) - 1)).Format(usageDayLayout)
	var sums []periodUsage
	err = database.GetDB().Table("client_daily_usage").
		Select("email, SUM(up) AS up, SUM(down) AS down").
		Where("day >= ?", since).Group("email").Scan(&sums).Error
	if err != nil {
		return nil, err
	}
	byEmail := make(map[string]periodUsage, len(sums))
	for _, p := range sums {
		byEmail[p.Email] = p
	}
	rows := make([]ClientReportRow, 0, len(clients))
	for _, c := range clients {
		row := ClientReportRow{
			Email: c.Email, Group: c.Group, Enable: c.Enable, Quota: c.TotalGB, ExpiryTime: c.ExpiryTime,
			PeriodUp: byEmail[c.Email].Up, PeriodDown: byEmail[c.Email].Down,
		}
		if c.Traffic != nil {
			row.TotalUp, row.TotalDown = c.Traffic.Up, c.Traffic.Down
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func reportExpiry(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).Format(usageDayLayout)
}

// csvSafe stops a spreadsheet from running a cell as a formula: a leading
// = + - @ tab or CR gets a quote in front, as OWASP's CSV-injection advice says.
func csvSafe(cell string) string {
	if cell != "" && (cell[0] == '=' || cell[0] == '+' || cell[0] == '-' || cell[0] == '@' || cell[0] == '\t' || cell[0] == '\r') {
		return "'" + cell
	}
	return cell
}

// WriteReportCSV writes the rows as CSV with byte counts in plain integers.
func WriteReportCSV(w io.Writer, rows []ClientReportRow) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"email", "group", "enabled", "period_up_bytes", "period_down_bytes", "total_up_bytes", "total_down_bytes", "quota_bytes", "expiry"})
	for _, r := range rows {
		_ = cw.Write([]string{
			csvSafe(r.Email), csvSafe(r.Group), strconv.FormatBool(r.Enable),
			strconv.FormatInt(r.PeriodUp, 10), strconv.FormatInt(r.PeriodDown, 10),
			strconv.FormatInt(r.TotalUp, 10), strconv.FormatInt(r.TotalDown, 10),
			strconv.FormatInt(r.Quota, 10), reportExpiry(r.ExpiryTime),
		})
	}
	cw.Flush()
	return cw.Error()
}

var reportTemplate = template.Must(template.New("report").Funcs(template.FuncMap{
	"bytes":  common.FormatTraffic,
	"expiry": reportExpiry,
	"quota": func(v int64) string {
		if v <= 0 {
			return "∞"
		}
		return common.FormatTraffic(v)
	},
}).Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Client usage report</title>
<style>
body{font:14px/1.4 system-ui,sans-serif;margin:24px;color:#111}
h1{font-size:20px;margin:0 0 4px}
p{margin:0 0 16px;color:#555}
table{border-collapse:collapse;width:100%}
th,td{border-bottom:1px solid #ddd;padding:6px 8px;text-align:left}
th{background:#f4f4f4}
td.n,th.n{text-align:right;font-variant-numeric:tabular-nums}
tr{break-inside:avoid}
.off{color:#a00}
@media print{body{margin:0}.hint{display:none}}
</style></head><body>
<h1>Client usage report</h1>
<p>Generated {{.Date}} · last {{.Days}} days · {{len .Rows}} clients</p>
<p class="hint">Use your browser's print command (Ctrl+P / Cmd+P) to print or save as PDF.</p>
<table>
<thead><tr><th>Client</th><th>Group</th><th class="n">Up ({{.Days}}d)</th><th class="n">Down ({{.Days}}d)</th><th class="n">Total up</th><th class="n">Total down</th><th class="n">Quota</th><th>Expiry</th></tr></thead>
<tbody>
{{range .Rows}}<tr><td{{if not .Enable}} class="off"{{end}}>{{.Email}}</td><td>{{.Group}}</td><td class="n">{{bytes .PeriodUp}}</td><td class="n">{{bytes .PeriodDown}}</td><td class="n">{{bytes .TotalUp}}</td><td class="n">{{bytes .TotalDown}}</td><td class="n">{{quota .Quota}}</td><td>{{expiry .ExpiryTime}}</td></tr>
{{end}}</tbody></table>
</body></html>
`))

// WriteReportHTML renders a self-contained, print-friendly report page.
func WriteReportHTML(w io.Writer, rows []ClientReportRow, days int, now time.Time) error {
	return reportTemplate.Execute(w, map[string]any{"Rows": rows, "Days": days, "Date": now.Format(usageDayLayout)})
}
