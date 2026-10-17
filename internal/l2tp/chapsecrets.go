package l2tp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// chapSecretsPath is fixed in pppd 2.4, so the panel keeps its logins in a
// marked block of the system file and leaves every other line alone.
var chapSecretsPath = "/etc/ppp/chap-secrets"

const (
	blockBegin = "# BEGIN x-ui l2tp (managed by the panel; edits here are overwritten)"
	blockEnd   = "# END x-ui l2tp"
)

// chapQuote quotes a word the way pppd's secrets parser reads it back.
func chapQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func writeChapSecrets(path string, users map[string]string) error {
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var kept []string
	inBlock := false
	for _, line := range strings.SplitAfter(string(raw), "\n") {
		switch strings.TrimRight(line, "\n") {
		case blockBegin:
			inBlock = true
			continue
		case blockEnd:
			inBlock = false
			continue
		}
		if !inBlock && line != "" {
			kept = append(kept, line)
		}
	}
	out := strings.Join(kept, "")
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	if len(users) > 0 {
		names := make([]string, 0, len(users))
		for name := range users {
			names = append(names, name)
		}
		sort.Strings(names)
		var b strings.Builder
		b.WriteString(blockBegin + "\n")
		for _, name := range names {
			b.WriteString(chapQuote(name) + " * " + chapQuote(users[name]) + " *\n")
		}
		b.WriteString(blockEnd + "\n")
		out += b.String()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(out), 0o600)
}
