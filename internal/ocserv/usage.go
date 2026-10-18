package ocserv

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const occtlTimeout = 10 * time.Second

// occtlPath finds ocserv's control tool next to the server.
func occtlPath() (string, error) {
	for _, p := range []string{"/usr/bin/occtl", "/usr/local/bin/occtl"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath("occtl"); err == nil {
		return p, nil
	}
	return "", errors.New("occtl not found; it ships with the ocserv package")
}

// session is one connected client as occtl reports it.
type session struct {
	id          string
	user        string
	rx, tx      int64
	connectedAt int64
}

// occtl runs one control command against an ocserv instance.
type occtl func(args ...string) ([]byte, error)

func runOcctl(socket string) occtl {
	return func(args ...string) ([]byte, error) {
		bin, err := occtlPath()
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), occtlTimeout)
		defer cancel()
		return exec.CommandContext(ctx, bin, append([]string{"-s", socket}, args...)...).Output()
	}
}

// parseSessions reads `occtl -j show users`. Its byte counts are strings, and
// the worker's id is the session's id.
func parseSessions(raw []byte) ([]session, error) {
	var rows []struct {
		ID          json.Number `json:"ID"`
		Username    string      `json:"Username"`
		RX          string      `json:"RX"`
		TX          string      `json:"TX"`
		ConnectedAt int64       `json:"raw_connected_at"`
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	out := make([]session, 0, len(rows))
	for _, r := range rows {
		rx, _ := strconv.ParseInt(r.RX, 10, 64)
		tx, _ := strconv.ParseInt(r.TX, 10, 64)
		out = append(out, session{id: r.ID.String(), user: r.Username, rx: rx, tx: tx, connectedAt: r.ConnectedAt})
	}
	return out, nil
}

func liveSessions(run occtl) ([]session, error) {
	raw, err := run("-j", "show", "users")
	if err != nil {
		return nil, err
	}
	return parseSessions(raw)
}

// tracker turns the byte counts of live sessions, and the final counts the
// disconnect script leaves, into per-user deltas that book every byte once.
type tracker struct {
	endedDir string
	last     map[string][2]int64
}

func newTracker(endedDir string) *tracker {
	return &tracker{endedDir: endedDir, last: map[string][2]int64{}}
}

func grow(cur, prev int64) int64 {
	if cur < prev {
		return cur
	}
	return cur - prev
}

// collect returns each user's bytes since the last call (index 0 is what the
// client sent). A session that ended is settled from its final counts and
// not counted again from the live listing.
func (t *tracker) collect(sessions []session) map[string][2]int64 {
	deltas := map[string][2]int64{}
	add := func(user string, up, down int64) {
		if up == 0 && down == 0 {
			return
		}
		d := deltas[user]
		deltas[user] = [2]int64{d[0] + up, d[1] + down}
	}
	settled := map[string]bool{}
	entries, _ := os.ReadDir(t.endedDir)
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		path := filepath.Join(t.endedDir, e.Name())
		raw, err := os.ReadFile(path)
		f := strings.SplitN(strings.TrimRight(string(raw), "\n"), " ", 3)
		if err != nil || len(f) != 3 {
			_ = os.Remove(path)
			continue
		}
		in, _ := strconv.ParseInt(f[0], 10, 64)
		out, _ := strconv.ParseInt(f[1], 10, 64)
		prev := t.last[e.Name()]
		add(f[2], grow(in, prev[0]), grow(out, prev[1]))
		delete(t.last, e.Name())
		settled[e.Name()] = true
		_ = os.Remove(path)
	}
	live := map[string]bool{}
	for _, s := range sessions {
		// A session listed just before it ended was settled from its final counts above.
		if settled[s.id] {
			continue
		}
		live[s.id] = true
		prev := t.last[s.id]
		add(s.user, grow(s.rx, prev[0]), grow(s.tx, prev[1]))
		t.last[s.id] = [2]int64{s.rx, s.tx}
	}
	for id := range t.last {
		if !live[id] {
			delete(t.last, id)
		}
	}
	return deltas
}
