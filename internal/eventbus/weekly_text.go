package eventbus

import (
	"fmt"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// Lines renders the alert as plain text for channels without their own layout.
func (d NewLoginLocationData) Lines() []string {
	place := d.IP
	if d.Country != "" {
		place += " (" + d.Country + ")"
	}
	line := fmt.Sprintf("%s signed in from a new address: %s", d.Username, place)
	if d.NewCountry {
		line = fmt.Sprintf("%s signed in from a new country: %s", d.Username, place)
	}
	return []string{line}
}

// Lines renders the alert as plain text for channels without their own layout.
func (d SharedAccountData) Lines() []string {
	line := fmt.Sprintf("%s was seen from %d addresses in %d hours", d.Email, d.IPCount, d.WindowHours)
	if d.Countries != "" {
		line += fmt.Sprintf(" across %d countries (%s)", d.CountryCount, d.Countries)
	}
	return []string{line}
}

// Lines renders the summary as plain text, one fact per line, for channels without their own layout.
func (d WeeklySummaryData) Lines() []string {
	from := time.UnixMilli(d.SinceMs).Format("2006-01-02")
	to := time.UnixMilli(d.UntilMs).Format("2006-01-02")
	traffic := "Traffic"
	if d.FirstReport {
		traffic = "Traffic since the start"
	}
	lines := []string{
		fmt.Sprintf("%s - %s", from, to),
		fmt.Sprintf("%s: %s", traffic, common.FormatTraffic(d.Bytes)),
		fmt.Sprintf("Clients: %d (new %d, expired %d, out of traffic now %d)", d.TotalClients, d.NewClients, d.ExpiredClients, d.DepletedClients),
	}
	for i, u := range d.TopUsers {
		if i == 0 {
			lines = append(lines, "Top users:")
		}
		lines = append(lines, fmt.Sprintf("%d. %s - %s", i+1, u.Email, common.FormatTraffic(u.Bytes)))
	}
	return lines
}
