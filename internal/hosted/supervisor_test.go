package hosted

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const helperEnv = "HOSTED_SUPERVISOR_HELPER"

// TestMain lets the test binary double as the supervised panel process.
func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		runHelper(mode)
		return
	}
	os.Exit(m.Run())
}

func runHelper(mode string) {
	f, err := os.OpenFile(os.Getenv("HELPER_STARTS"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err == nil {
		_, _ = f.WriteString("start\n")
		_ = f.Close()
	}
	if mode == "crash" {
		os.Exit(3)
	}
	time.Sleep(time.Minute)
}

func helperSupervisor(t *testing.T, mode string) (*Supervisor, string) {
	t.Helper()
	starts := filepath.Join(t.TempDir(), "starts")
	s := NewSupervisor(func(Panel) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^$")
		cmd.Env = append(os.Environ(), helperEnv+"="+mode, "HELPER_STARTS="+starts)
		return cmd
	})
	s.backoffMin, s.backoffMax = 10*time.Millisecond, 20*time.Millisecond
	t.Cleanup(s.StopAll)
	return s, starts
}

func countStarts(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "start\n")
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSupervisorRestartsACrashedPanel(t *testing.T) {
	s, starts := helperSupervisor(t, "crash")
	s.Start(Panel{ID: 1, LogPath: filepath.Join(t.TempDir(), "panel.log")})

	waitFor(t, "three starts", func() bool { return countStarts(t, starts) >= 3 })
	st := s.State(1)
	if st.Restarts < 2 || !strings.Contains(st.LastError, "exit status 3") {
		t.Fatalf("state after crashes = %+v, want restarts>=2 and the exit error kept", st)
	}
}

func TestSupervisorStopKillsThePanelAndDoesNotRestartIt(t *testing.T) {
	s, starts := helperSupervisor(t, "sleep")
	s.Start(Panel{ID: 7})
	waitFor(t, "running", func() bool { return s.State(7).Running && countStarts(t, starts) == 1 })

	s.Stop(7)
	if st := s.State(7); st.Running {
		t.Fatalf("still running after Stop: %+v", st)
	}
	time.Sleep(100 * time.Millisecond)
	if n := countStarts(t, starts); n != 1 {
		t.Fatalf("panel started %d times, want exactly 1 after Stop", n)
	}
}

func TestWatchParentFiresWhenReparented(t *testing.T) {
	var ppid atomic.Int32
	ppid.Store(100)
	gone := make(chan struct{})
	go watchParent(func() int { return int(ppid.Load()) }, time.Millisecond, func() { close(gone) })
	select {
	case <-gone:
		t.Fatal("fired while the parent was still alive")
	case <-time.After(20 * time.Millisecond):
	}
	ppid.Store(1)
	select {
	case <-gone:
	case <-time.After(time.Second):
		t.Fatal("did not fire after the parent changed")
	}
}
