package l2tp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// fakePPPD runs a process whose name is pppd, as a real link's would be.
func fakePPPD(t *testing.T, name string) *exec.Cmd {
	t.Helper()
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep binary")
	}
	bin := filepath.Join(t.TempDir(), name)
	raw, err := os.ReadFile(sleep)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, raw, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd
}

func exited(t *testing.T, cmd *exec.Cmd) bool {
	t.Helper()
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(time.Second):
		return false
	}
}

func TestKickEndsRevokedChangedAndExcessLinks(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	links := map[string]*exec.Cmd{}
	record := func(ifname, user string, cmd *exec.Cmd) {
		writeSpool(t, runDir(), "sessions", ifname, strconv.Itoa(cmd.Process.Pid)+" 10.90.0.9 "+user+"\n")
		links[ifname] = cmd
		time.Sleep(20 * time.Millisecond) // distinct mtimes: the newest link is the one to end
	}
	record("ppp0", "kept", fakePPPD(t, "pppd"))
	record("ppp1", "gone", fakePPPD(t, "pppd"))
	record("ppp2", "repassed", fakePPPD(t, "pppd"))
	record("ppp3", "capped", fakePPPD(t, "pppd"))
	record("ppp4", "capped", fakePPPD(t, "pppd"))
	// A stale record whose pid now belongs to something else must not be killed.
	record("ppp5", "gone", fakePPPD(t, "nginx"))

	inst := Instance{Users: map[string]User{
		"kept": {Password: "k"}, "repassed": {Password: "new"}, "capped": {Password: "c", MaxConns: 1},
	}}
	(&Manager{}).kickLocked(inst, map[string]string{"kept": "k", "gone": "g", "repassed": "old", "capped": "c"})

	for ifname, want := range map[string]bool{"ppp0": false, "ppp1": true, "ppp2": true, "ppp3": false, "ppp4": true, "ppp5": false} {
		if got := exited(t, links[ifname]); got != want {
			t.Errorf("%s ended = %v, want %v", ifname, got, want)
		}
	}
}
