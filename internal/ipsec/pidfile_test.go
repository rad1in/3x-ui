package ipsec

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestRunningCharonReadsALivePidFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	live := write("live.pid", strconv.Itoa(os.Getpid())+"\n")
	if pid, ok := runningCharon(live); !ok || pid != os.Getpid() {
		t.Fatalf("live pid file = %d, %v; want this process", pid, ok)
	}
	// A stale file from a crashed daemon must not block the start.
	if _, ok := runningCharon(write("stale.pid", "2147483646\n")); ok {
		t.Fatal("a pid file naming no process counted as a running charon")
	}
	if _, ok := runningCharon(filepath.Join(dir, "missing.pid")); ok {
		t.Fatal("a missing pid file counted as a running charon")
	}
}
