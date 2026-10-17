package l2tp

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// session is one PPP link as pppd's ip-up script recorded it.
type session struct {
	ifname string
	pid    int
	user   string
}

type linkKey struct {
	ifname string
	pid    int
}

// statsReader returns the bytes an interface received (from the client) and sent.
type statsReader func(ifname string) (rx, tx int64, ok bool)

func sysfsStats(ifname string) (int64, int64, bool) {
	read := func(name string) (int64, bool) {
		raw, err := os.ReadFile(filepath.Join("/sys/class/net", ifname, "statistics", name))
		if err != nil {
			return 0, false
		}
		n, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
		return n, err == nil
	}
	rx, ok1 := read("rx_bytes")
	tx, ok2 := read("tx_bytes")
	return rx, tx, ok1 && ok2
}

// tracker turns live interface counters and pppd's final totals into
// per-user deltas, booking every byte of a link once.
type tracker struct {
	dir   string
	stats statsReader
	last  map[linkKey][2]int64
}

func newTracker(dir string, stats statsReader) *tracker {
	return &tracker{dir: dir, stats: stats, last: map[linkKey][2]int64{}}
}

func grow(cur, prev int64) int64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}

// liveSessions reads the links ip-up recorded, oldest first.
func liveSessions(dir string) []session {
	entries, err := os.ReadDir(filepath.Join(dir, "sessions"))
	if err != nil {
		return nil
	}
	type dated struct {
		s   session
		mod int64
	}
	var out []dated
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, "sessions", e.Name()))
		if err != nil {
			continue
		}
		f := strings.SplitN(strings.TrimRight(string(raw), "\n"), " ", 3)
		if len(f) != 3 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		var mod int64
		if info, err := e.Info(); err == nil {
			mod = info.ModTime().UnixNano()
		}
		out = append(out, dated{session{ifname: e.Name(), pid: pid, user: f[2]}, mod})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].mod < out[j].mod })
	sessions := make([]session, len(out))
	for i, d := range out {
		sessions[i] = d.s
	}
	return sessions
}

// collect returns each user's bytes since the last call (index 0 is what the
// client sent) and the links that are up now.
func (t *tracker) collect() (map[string][2]int64, []session) {
	deltas := map[string][2]int64{}
	add := func(user string, up, down int64) {
		if up == 0 && down == 0 {
			return
		}
		d := deltas[user]
		deltas[user] = [2]int64{d[0] + up, d[1] + down}
	}

	ended, _ := os.ReadDir(filepath.Join(t.dir, "ended"))
	for _, e := range ended {
		name := e.Name()
		dot := strings.LastIndexByte(name, '.')
		if e.IsDir() || dot <= 0 || strings.HasSuffix(name, ".tmp") {
			continue
		}
		pid, err := strconv.Atoi(name[dot+1:])
		path := filepath.Join(t.dir, "ended", name)
		raw, rerr := os.ReadFile(path)
		f := strings.SplitN(strings.TrimRight(string(raw), "\n"), " ", 3)
		if err != nil || rerr != nil || len(f) != 3 {
			_ = os.Remove(path)
			continue
		}
		rcvd, _ := strconv.ParseInt(f[0], 10, 64)
		sent, _ := strconv.ParseInt(f[1], 10, 64)
		key := linkKey{name[:dot], pid}
		prev := t.last[key]
		add(f[2], grow(rcvd, prev[0]), grow(sent, prev[1]))
		delete(t.last, key)
		_ = os.Remove(path)
	}

	sessions := liveSessions(t.dir)
	live := map[linkKey]bool{}
	for _, s := range sessions {
		key := linkKey{s.ifname, s.pid}
		live[key] = true
		rx, tx, ok := t.stats(s.ifname)
		if !ok {
			continue
		}
		prev := t.last[key]
		add(s.user, grow(rx, prev[0]), grow(tx, prev[1]))
		t.last[key] = [2]int64{rx, tx}
	}
	for key := range t.last {
		if !live[key] {
			delete(t.last, key)
		}
	}
	return deltas, sessions
}
