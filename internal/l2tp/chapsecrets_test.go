package l2tp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChapSecretsKeepsTheAdminsOwnLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chap-secrets")
	own := "# Secrets for authentication using CHAP\n\"office\" * \"office-pass\" *\n"
	if err := os.WriteFile(path, []byte(own), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeChapSecrets(path, map[string]string{"bob": "b-pass", `we"ird\name`: `p"w\x`}); err != nil {
		t.Fatal(err)
	}
	want := own + "# BEGIN x-ui l2tp (managed by the panel; edits here are overwritten)\n" +
		"\"bob\" * \"b-pass\" *\n" +
		"\"we\\\"ird\\\\name\" * \"p\\\"w\\\\x\" *\n" +
		"# END x-ui l2tp\n"
	if got := readFile(t, path); got != want {
		t.Fatalf("after the first write:\n%s\nwant:\n%s", got, want)
	}

	// A rewrite replaces only the panel's block; emptying it leaves the file as the admin had it.
	if err := writeChapSecrets(path, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != own {
		t.Fatalf("after removing every login:\n%q\nwant:\n%q", got, own)
	}
}

func TestChapSecretsCreatesAMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ppp", "chap-secrets")
	if err := writeChapSecrets(path, map[string]string{"a": "b"}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("stat = %v, %v; the secrets file must be private", fi, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
