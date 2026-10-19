package pppsecrets

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestBlocksOfDifferentProtocolsCoexistAndRewriteIndependently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chap-secrets")
	own := "\"office\" * \"office-pass\" *\n"
	if err := os.WriteFile(path, []byte(own), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, "l2tp", []Entry{{"alice", Any, "a-pass"}}); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, "sstp", []Entry{{"bob", "x-ui-sstp-2", "b-pass"}, {"bob", "x-ui-sstp-1", "b-pass"}}); err != nil {
		t.Fatal(err)
	}
	want := own +
		"# BEGIN x-ui l2tp (managed by the panel; edits here are overwritten)\n\"alice\" * \"a-pass\" *\n# END x-ui l2tp\n" +
		"# BEGIN x-ui sstp (managed by the panel; edits here are overwritten)\n" +
		"\"bob\" \"x-ui-sstp-1\" \"b-pass\" *\n\"bob\" \"x-ui-sstp-2\" \"b-pass\" *\n# END x-ui sstp\n"
	if got := read(t, path); got != want {
		t.Fatalf("two blocks:\n%s\nwant:\n%s", got, want)
	}

	// Rewriting one protocol's block must not touch the other's.
	if err := Write(path, "l2tp", []Entry{{"carol", Any, "c-pass"}}); err != nil {
		t.Fatal(err)
	}
	got := read(t, path)
	if !strings.Contains(got, "\"bob\" \"x-ui-sstp-1\" \"b-pass\" *") || strings.Contains(got, "alice") || !strings.Contains(got, "carol") {
		t.Fatalf("after rewriting l2tp:\n%s", got)
	}
	if err := Write(path, "sstp", nil); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); strings.Contains(got, "sstp") || !strings.Contains(got, "carol") || !strings.HasPrefix(got, own) {
		t.Fatalf("after clearing sstp:\n%s", got)
	}
}

// Two managers rewrite one file; without the shared lock a block is lost or doubled.
func TestConcurrentWritersNeverLoseOrDoubleABlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chap-secrets")
	var wg sync.WaitGroup
	for _, proto := range []string{"l2tp", "sstp"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				if err := Write(path, proto, []Entry{{proto + "-user", Any, "pw"}}); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	got := read(t, path)
	for _, proto := range []string{"l2tp", "sstp"} {
		if n := strings.Count(got, "# BEGIN x-ui "+proto); n != 1 {
			t.Errorf("%s block appears %d times:\n%s", proto, n, got)
		}
	}
}
