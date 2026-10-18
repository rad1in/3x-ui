// Package pppsecrets keeps the panel's PPP logins in marked blocks of pppd's
// chap-secrets file, one block per protocol, leaving the admin's lines alone.
package pppsecrets

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Path is fixed in pppd 2.4, so the logins of every PPP-based inbound share this file.
var Path = "/etc/ppp/chap-secrets"

// Any matches every pppd server name.
const Any = "*"

// Entry is one login, valid for the pppd server named Server.
type Entry struct {
	User, Server, Secret string
}

// One lock for the read-modify-write: two protocols rewrite the same file.
var mu sync.Mutex

func begin(name string) string {
	return "# BEGIN x-ui " + name + " (managed by the panel; edits here are overwritten)"
}

func end(name string) string { return "# END x-ui " + name }

// quote quotes a word the way pppd's secrets parser reads it back.
func quote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// Write replaces the block named name with the entries; none removes the block.
func Write(path, name string, entries []Entry) error {
	mu.Lock()
	defer mu.Unlock()
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var kept []string
	inBlock := false
	for _, line := range strings.SplitAfter(string(raw), "\n") {
		switch strings.TrimRight(line, "\n") {
		case begin(name):
			inBlock = true
			continue
		case end(name):
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
	if len(entries) > 0 {
		sorted := append([]Entry(nil), entries...)
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].User != sorted[j].User {
				return sorted[i].User < sorted[j].User
			}
			return sorted[i].Server < sorted[j].Server
		})
		var b strings.Builder
		b.WriteString(begin(name) + "\n")
		for _, e := range sorted {
			server := e.Server
			if server != Any {
				server = quote(server)
			}
			b.WriteString(quote(e.User) + " " + server + " " + quote(e.Secret) + " *\n")
		}
		b.WriteString(end(name) + "\n")
		out += b.String()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(out), 0o600)
}
