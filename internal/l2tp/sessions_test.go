package l2tp

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

type fakeStats map[string][2]int64

func (f fakeStats) read(ifname string) (int64, int64, bool) {
	v, ok := f[ifname]
	return v[0], v[1], ok
}

func writeSpool(t *testing.T, dir, sub, name, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sub, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// ip-up and ip-down write what pppd knows; the tracker turns interface
// counters and final totals into deltas that are each booked once.
func TestTrackerMetersLiveAndEndedSessions(t *testing.T) {
	dir := t.TempDir()
	stats := fakeStats{}
	tr := newTracker(dir, stats.read)

	writeSpool(t, dir, "sessions", "ppp0", "4100 10.90.0.2 ali reza\n")
	stats["ppp0"] = [2]int64{1000, 5000}
	deltas, sessions := tr.collect()
	if want := map[string][2]int64{"ali reza": {1000, 5000}}; !reflect.DeepEqual(deltas, want) {
		t.Fatalf("first collect = %v, want %v", deltas, want)
	}
	if len(sessions) != 1 || sessions[0] != (session{ifname: "ppp0", pid: 4100, user: "ali reza"}) {
		t.Fatalf("sessions = %+v", sessions)
	}

	stats["ppp0"] = [2]int64{1500, 5200}
	if deltas, _ = tr.collect(); !reflect.DeepEqual(deltas, map[string][2]int64{"ali reza": {500, 200}}) {
		t.Fatalf("second collect = %v, want the growth", deltas)
	}

	// The link drops: pppd reports its totals and ppp0 is handed to someone new.
	os.Remove(filepath.Join(dir, "sessions", "ppp0"))
	writeSpool(t, dir, "ended", "ppp0.4100", "1600 5300 ali reza\n")
	writeSpool(t, dir, "sessions", "ppp0", "4200 10.90.0.3 bob\n")
	stats["ppp0"] = [2]int64{40, 60}
	deltas, _ = tr.collect()
	if want := map[string][2]int64{"ali reza": {100, 100}, "bob": {40, 60}}; !reflect.DeepEqual(deltas, want) {
		t.Fatalf("after the hand-over = %v, want %v", deltas, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "ended", "ppp0.4100")); !os.IsNotExist(err) {
		t.Fatal("a booked ending was left to be booked again")
	}
	if deltas, _ = tr.collect(); !reflect.DeepEqual(deltas, map[string][2]int64{}) {
		t.Fatalf("an idle collect booked %v", deltas)
	}
}

// runScript calls a hook the way pppd does: interface, tty, speed, local and
// remote address as arguments, the peer and counters in the environment.
func runScript(t *testing.T, path string, env ...string) {
	t.Helper()
	cmd := exec.Command("sh", path, "ppp7", "/dev/pts/3", "0", "10.90.0.1", "10.90.0.9", "")
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", path, err, out)
	}
}

func TestPPPScriptsFeedTheTracker(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"ip-up": renderIPUp(dir), "ip-down": renderIPDown(dir)} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	stats := fakeStats{"ppp7": {300, 900}}
	tr := newTracker(dir, stats.read)

	runScript(t, filepath.Join(dir, "ip-up"), "PPPD_PID=777", "PEERNAME=sara jones")
	deltas, sessions := tr.collect()
	if len(sessions) != 1 || sessions[0] != (session{ifname: "ppp7", pid: 777, user: "sara jones"}) {
		t.Fatalf("sessions = %+v", sessions)
	}
	if !reflect.DeepEqual(deltas, map[string][2]int64{"sara jones": {300, 900}}) {
		t.Fatalf("live deltas = %v", deltas)
	}

	runScript(t, filepath.Join(dir, "ip-down"), "PPPD_PID=777", "PEERNAME=sara jones", "BYTES_RCVD=350", "BYTES_SENT=1000")
	deltas, sessions = tr.collect()
	if len(sessions) != 0 || !reflect.DeepEqual(deltas, map[string][2]int64{"sara jones": {50, 100}}) {
		t.Fatalf("after ip-down: deltas %v, sessions %+v", deltas, sessions)
	}
}
