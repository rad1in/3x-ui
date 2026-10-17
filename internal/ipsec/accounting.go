package ipsec

import (
	"path/filepath"
	"sync"

	"github.com/strongswan/govici/vici"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

type childCount struct {
	tag, user string
	in, out   int64
}

type userKey struct{ tag, user string }

// accounting turns charon's cumulative per-SA byte counters into deltas. A
// child SA that ends between two polls reports its last bytes in an event.
type accounting struct {
	mu       sync.Mutex
	counters map[uint64]childCount
	finished map[uint64]bool // ended SAs the next list may still show
	pending  map[userKey]*Traffic
	tags     map[string]string // connection name -> inbound tag
}

func (a *accounting) reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.counters = map[uint64]childCount{}
	a.finished = map[uint64]bool{}
	a.pending = map[userKey]*Traffic{}
	a.tags = map[string]string{}
}

func (a *accounting) addLocked(tag, user string, in, out int64) {
	if in == 0 && out == 0 {
		return
	}
	k := userKey{tag, user}
	t := a.pending[k]
	if t == nil {
		t = &Traffic{Tag: tag, Email: user}
		a.pending[k] = t
	}
	// bytes-in on the server is what the client sent.
	t.Up += in
	t.Down += out
}

func grow(cur, prev int64) int64 {
	if cur < prev {
		return cur
	}
	return cur - prev
}

// drain books the listed SAs against their last counts and returns, then
// clears, everything booked since the previous drain.
func (a *accounting) drain(sas []ikeSA, insts map[string]Instance) []Traffic {
	a.mu.Lock()
	defer a.mu.Unlock()
	for conn, inst := range insts {
		a.tags[conn] = inst.Tag
	}
	listed := map[uint64]bool{}
	for _, sa := range sas {
		inst, ok := insts[sa.conn]
		if !ok || sa.user == "" {
			continue
		}
		for _, c := range sa.children {
			listed[c.id] = true
			if a.finished[c.id] {
				continue
			}
			prev := a.counters[c.id]
			a.addLocked(inst.Tag, sa.user, grow(c.in, prev.in), grow(c.out, prev.out))
			a.counters[c.id] = childCount{tag: inst.Tag, user: sa.user, in: c.in, out: c.out}
		}
	}
	if sas != nil {
		for id := range a.counters {
			if !listed[id] {
				delete(a.counters, id)
			}
		}
		for id := range a.finished {
			if !listed[id] {
				delete(a.finished, id)
			}
		}
	}
	out := make([]Traffic, 0, len(a.pending))
	for _, t := range a.pending {
		out = append(out, *t)
	}
	a.pending = map[userKey]*Traffic{}
	return out
}

func (a *accounting) finish(conn, user string, c childSA) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.finished[c.id] {
		return
	}
	prev, known := a.counters[c.id]
	tag := prev.tag
	if !known {
		tag = a.tags[conn]
	}
	if tag == "" || user == "" {
		return
	}
	a.addLocked(tag, user, grow(c.in, prev.in), grow(c.out, prev.out))
	a.finished[c.id] = true
	delete(a.counters, c.id)
}

// listen books child SAs as they go down or are rekeyed, until d exits.
func (a *accounting) listen(d *daemon) {
	sess, err := vici.NewSession(vici.WithSocketPath(filepath.Join(runDir(), "charon.vici")))
	if err != nil {
		logger.Warning("ipsec: event session:", err)
		return
	}
	defer sess.Close()
	if err := sess.Subscribe("child-updown", "child-rekey"); err != nil {
		logger.Warning("ipsec: subscribe:", err)
		return
	}
	events := make(chan vici.Event, 256)
	sess.NotifyEvents(events)
	for {
		select {
		case <-d.done:
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev.Name == "child-updown" && str(ev.Message, "up") == "yes" {
				continue
			}
			for _, sa := range parseIKESAs(ev.Message, false) {
				for _, c := range sa.children {
					a.finish(sa.conn, sa.user, c)
				}
			}
		}
	}
}
