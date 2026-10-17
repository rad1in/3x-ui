package cliclient

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

const Usage = `Usage: x-ui client <command> [arguments]

Commands:
  list [-inbound ID]                      list every client, or those on one inbound
  show EMAIL                              one client with its usage and inbounds
  add EMAIL -inbound ID[,ID...] [-gb N] [-days N] [-limit-ip N] [-comment TEXT]
                                          create a client (no -gb or -days = unlimited)
  del EMAIL...                            delete clients
  enable EMAIL... | disable EMAIL...      switch clients on or off
  reset EMAIL...                          zero clients' used traffic
`

var errUsage = errors.New("see usage above")

// Run executes one `x-ui client` command against the panel.
func Run(api *API, args []string, out io.Writer, now time.Time) error {
	if len(args) == 0 {
		fmt.Fprint(out, Usage)
		return errUsage
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "list":
		fs := flag.NewFlagSet("list", flag.ContinueOnError)
		fs.SetOutput(out)
		inbound := fs.Int("inbound", 0, "only clients on this inbound")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		rows, err := api.List()
		if err != nil {
			return err
		}
		printList(out, rows, *inbound, now)
		return nil
	case "show":
		if len(rest) != 1 {
			fmt.Fprint(out, Usage)
			return errUsage
		}
		row, err := api.Get(rest[0])
		if err != nil {
			return err
		}
		printShow(out, row, now)
		return nil
	case "add":
		return runAdd(api, rest, out, now)
	case "del", "delete", "enable", "disable", "reset":
		if len(rest) == 0 {
			fmt.Fprint(out, Usage)
			return errUsage
		}
		return runEach(api, cmd, rest, out)
	default:
		fmt.Fprint(out, Usage)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func runAdd(api *API, args []string, out io.Writer, now time.Time) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fmt.Fprint(out, Usage)
		return errUsage
	}
	email := args[0]
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(out)
	inbounds := fs.String("inbound", "", "inbound IDs, comma separated")
	gb := fs.Float64("gb", 0, "traffic quota in GB, 0 = unlimited")
	days := fs.Int("days", 0, "valid for this many days, 0 = never expires")
	limitIP := fs.Int("limit-ip", 0, "max concurrent IPs, 0 = unlimited")
	comment := fs.String("comment", "", "free-text note")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	ids, err := parseIDs(*inbounds)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return errors.New("-inbound is required: the inbound IDs to attach the client to")
	}
	if *gb < 0 || *days < 0 || *limitIP < 0 {
		return errors.New("-gb, -days and -limit-ip cannot be negative")
	}
	c := Client{Email: email, Enable: true, LimitIP: *limitIP, Comment: *comment,
		TotalGB: int64(math.Round(*gb * (1 << 30)))}
	if *days > 0 {
		c.ExpiryTime = now.Add(time.Duration(*days) * 24 * time.Hour).UnixMilli()
	}
	if err := api.Add(c, ids); err != nil {
		return err
	}
	fmt.Fprintf(out, "added %s\n", email)
	return nil
}

// runEach applies the command to every email and reports each failure,
// so one bad email does not hide what happened to the rest.
func runEach(api *API, cmd string, emails []string, out io.Writer) error {
	if cmd == "enable" || cmd == "disable" {
		skipped, err := api.SetEnable(emails, cmd == "enable")
		if err != nil {
			return err
		}
		for _, s := range skipped {
			fmt.Fprintln(out, "skipped", s)
		}
		fmt.Fprintf(out, "%sd %d client(s)\n", cmd, len(emails)-len(skipped))
		if len(skipped) > 0 {
			return fmt.Errorf("%d client(s) not %sd", len(skipped), cmd)
		}
		return nil
	}
	failed := 0
	for _, email := range emails {
		var err error
		if cmd == "reset" {
			err = api.ResetTraffic(email)
		} else {
			err = api.Delete(email)
		}
		if err != nil {
			failed++
			fmt.Fprintf(out, "%s: %v\n", email, err)
			continue
		}
		verb := "deleted"
		if cmd == "reset" {
			verb = "reset"
		}
		fmt.Fprintf(out, "%s %s\n", verb, email)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d failed", failed, len(emails))
	}
	return nil
}

func parseIDs(raw string) ([]int, error) {
	var ids []int
	for f := range strings.SplitSeq(raw, ",") {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		id, err := strconv.Atoi(f)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("bad inbound ID %q", f)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func quota(total int64) string {
	if total <= 0 {
		return "unlimited"
	}
	return common.FormatTraffic(total)
}

func expiry(ms int64, now time.Time) string {
	switch {
	case ms == 0:
		return "never"
	case ms < 0:
		return fmt.Sprintf("%d days after first use", -ms/int64(24*time.Hour/time.Millisecond))
	case ms <= now.UnixMilli():
		return "expired " + time.UnixMilli(ms).Format("2006-01-02")
	default:
		return time.UnixMilli(ms).Format("2006-01-02 15:04")
	}
}

func used(t *Traffic) int64 {
	if t == nil {
		return 0
	}
	return t.Up + t.Down
}

func joinIDs(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}

func printList(out io.Writer, rows []Row, inbound int, now time.Time) {
	sort.Slice(rows, func(i, j int) bool { return rows[i].Email < rows[j].Email })
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "EMAIL\tENABLED\tUSED\tQUOTA\tEXPIRES\tINBOUNDS")
	for _, r := range rows {
		if inbound > 0 && !containsID(r.InboundIds, inbound) {
			continue
		}
		fmt.Fprintf(w, "%s\t%t\t%s\t%s\t%s\t%s\n", r.Email, r.Enable, common.FormatTraffic(used(r.Traffic)),
			quota(r.TotalGB), expiry(r.ExpiryTime, now), joinIDs(r.InboundIds))
	}
	w.Flush()
}

func containsID(ids []int, id int) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func printShow(out io.Writer, r Row, now time.Time) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "email\t%s\n", r.Email)
	fmt.Fprintf(w, "enabled\t%t\n", r.Enable)
	fmt.Fprintf(w, "used\t%s of %s\n", common.FormatTraffic(used(r.Traffic)), quota(r.TotalGB))
	fmt.Fprintf(w, "expires\t%s\n", expiry(r.ExpiryTime, now))
	fmt.Fprintf(w, "ip limit\t%d\n", r.LimitIP)
	fmt.Fprintf(w, "inbounds\t%s\n", joinIDs(r.InboundIds))
	fmt.Fprintf(w, "sub id\t%s\n", r.SubID)
	if r.Comment != "" {
		fmt.Fprintf(w, "comment\t%s\n", r.Comment)
	}
	w.Flush()
}
