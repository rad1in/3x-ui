package ocserv

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// This is what `occtl -j show users` prints on ocserv 1.2: tabs in places,
// a key twice, and the byte counts as strings.
const occtlUsers = `[
  {
    "ID":  14414,
    "Username":  "alice",
    "State":  "connected",
    "RX":  "2048",
    "TX":  "4096",
    "_RX":  "2 KB",
    "DNS":	["1.1.1.1"],
    "Routes":	[],
    "Routes":  "defaultroute",
    "raw_connected_at":  1792338222,
    "Restricted to ports":	[]
  },
  {
    "ID":  14500,
    "Username":  "bob@x",
    "RX":  "0",
    "TX":  "7",
    "raw_connected_at":  1792338300
  }
]`

func TestParseSessionsReadsRealOcctlOutput(t *testing.T) {
	got, err := parseSessions([]byte(occtlUsers))
	if err != nil {
		t.Fatal(err)
	}
	want := []session{
		{id: "14414", user: "alice", rx: 2048, tx: 4096, connectedAt: 1792338222},
		{id: "14500", user: "bob@x", rx: 0, tx: 7, connectedAt: 1792338300},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sessions = %+v\nwant %+v", got, want)
	}
	if none, err := parseSessions([]byte("  \n")); err != nil || len(none) != 0 {
		t.Fatalf("empty output parsed as %v, %v", none, err)
	}
}

func writeEnded(t *testing.T, dir, id, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, id), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTrackerBooksEveryByteOnce(t *testing.T) {
	dir := t.TempDir()
	tr := newTracker(dir)

	// Two polls of a growing session, then it ends having moved more than the last poll saw.
	d := tr.collect([]session{{id: "1", user: "alice", rx: 100, tx: 200}})
	if got := d["alice"]; got != [2]int64{100, 200} {
		t.Fatalf("first poll = %v", got)
	}
	d = tr.collect([]session{{id: "1", user: "alice", rx: 150, tx: 260}})
	if got := d["alice"]; got != [2]int64{50, 60} {
		t.Fatalf("second poll = %v, want only the growth", got)
	}
	writeEnded(t, dir, "1", "180 300 alice\n")
	d = tr.collect(nil)
	if got := d["alice"]; got != [2]int64{30, 40} {
		t.Fatalf("after it ended = %v, want the bytes since the last poll", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("the settled file was left behind: %v", entries)
	}
	if d = tr.collect(nil); len(d) != 0 {
		t.Fatalf("nothing is live, yet %v was booked", d)
	}
}

func TestTrackerDoesNotCountASessionTwiceWhenItEndsMidPoll(t *testing.T) {
	dir := t.TempDir()
	tr := newTracker(dir)
	tr.collect([]session{{id: "7", user: "bob", rx: 10, tx: 10}})
	// The listing was taken just before the session ended, and its file is there by now.
	writeEnded(t, dir, "7", "25 35 bob\n")
	d := tr.collect([]session{{id: "7", user: "bob", rx: 20, tx: 30}})
	if got := d["bob"]; got != [2]int64{15, 25} {
		t.Fatalf("booked %v; want the 15/25 the session moved after the first poll, once", got)
	}
}

func TestTrackerSettlesASessionItNeverPolled(t *testing.T) {
	dir := t.TempDir()
	tr := newTracker(dir)
	writeEnded(t, dir, "9", "500 600 carol\n")
	writeEnded(t, dir, "10", "garbage\n")
	d := tr.collect(nil)
	if got := d["carol"]; got != [2]int64{500, 600} || len(d) != 1 {
		t.Fatalf("booked %v; want carol's full counts and nothing for the malformed file", d)
	}
}
