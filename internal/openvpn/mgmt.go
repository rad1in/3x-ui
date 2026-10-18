package openvpn

import (
	"bufio"
	"crypto/subtle"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// bytecountInterval is how often openvpn reports per-client byte counters.
const bytecountInterval = 5

// User is one account the panel lets connect. MaxConns 0 means unlimited.
type User struct {
	Password string
	MaxConns int
}

// Counters is traffic in bytes; Up is what the client sent.
type Counters struct {
	Up   int64
	Down int64
}

type conn struct {
	user        string
	password    string
	in, out     int64
	established bool
}

// session speaks the openvpn management protocol: it decides every client
// login (management-client-auth), meters traffic and drops revoked users.
type session struct {
	nc net.Conn

	writeMu sync.Mutex
	mu      sync.Mutex
	users   map[string]User
	conns   map[uint64]*conn
	deltas  map[string]Counters

	// The client block being read: CONNECT/REAUTH/ESTABLISHED/DISCONNECT + its ENV lines.
	event    string
	cid, kid uint64
	env      map[string]string
}

func newSession(nc net.Conn) *session {
	return &session{nc: nc, users: map[string]User{}, conns: map[uint64]*conn{}, deltas: map[string]Counters{}}
}

func (s *session) command(format string, args ...any) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := fmt.Fprintf(s.nc, format+"\n", args...); err != nil {
		logger.Debug("openvpn: management write:", err)
	}
}

// SetUsers replaces the allowed accounts and drops connections whose account
// is gone, disabled, or now has another password.
func (s *session) SetUsers(users map[string]User) {
	s.mu.Lock()
	s.users = users
	var kill []uint64
	for cid, c := range s.conns {
		u, ok := users[c.user]
		if !ok || (c.password != "" && !equalSecret(u.Password, c.password)) {
			kill = append(kill, cid)
		}
	}
	s.mu.Unlock()
	slices.Sort(kill)
	for _, cid := range kill {
		s.command("client-kill %d", cid)
	}
}

// KickUser ends every connection of one account and returns how many; the
// account stays valid, so the client may reconnect at once.
func (s *session) KickUser(user string) int {
	s.mu.Lock()
	var kill []uint64
	for cid, c := range s.conns {
		if c.user == user {
			kill = append(kill, cid)
		}
	}
	s.mu.Unlock()
	slices.Sort(kill)
	for _, cid := range kill {
		s.command("client-kill %d", cid)
	}
	return len(kill)
}

// Collect returns the traffic since the last call and the users online now.
func (s *session) Collect() (map[string]Counters, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.deltas
	s.deltas = map[string]Counters{}
	var online []string
	for _, c := range s.conns {
		if c.established && !slices.Contains(online, c.user) {
			online = append(online, c.user)
		}
	}
	slices.Sort(online)
	return out, online
}

func (s *session) serve() {
	s.command("bytecount %d", bytecountInterval)
	sc := bufio.NewScanner(s.nc)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		s.handleLine(strings.TrimRight(sc.Text(), "\r"))
	}
}

func (s *session) handleLine(line string) {
	rest, ok := strings.CutPrefix(line, ">")
	if !ok {
		if strings.HasPrefix(line, "ERROR") {
			logger.Debug("openvpn: management:", line)
		}
		return
	}
	kind, args, _ := strings.Cut(rest, ":")
	switch kind {
	case "BYTECOUNT_CLI":
		f := strings.Split(args, ",")
		if len(f) == 3 {
			s.addBytes(parseUint(f[0]), parseInt(f[1]), parseInt(f[2]))
		}
	case "CLIENT":
		s.handleClient(args)
	}
}

func (s *session) handleClient(args string) {
	event, rest, _ := strings.Cut(args, ",")
	if event == "ENV" {
		if rest == "END" {
			s.finishEvent()
			return
		}
		if k, v, ok := strings.Cut(rest, "="); ok && s.env != nil {
			s.env[k] = v
		}
		return
	}
	f := strings.Split(rest, ",")
	s.event, s.env = event, map[string]string{}
	s.cid = parseUint(f[0])
	s.kid = 0
	if len(f) > 1 {
		s.kid = parseUint(f[1])
	}
}

func (s *session) finishEvent() {
	event, cid, kid, env := s.event, s.cid, s.kid, s.env
	s.event, s.env = "", nil
	switch event {
	case "CONNECT", "REAUTH":
		if reason := s.authorize(cid, env, event == "REAUTH"); reason != "" {
			s.command("client-deny %d %d %q", cid, kid, reason)
			return
		}
		s.command("client-auth-nt %d %d", cid, kid)
	case "ESTABLISHED":
		s.mu.Lock()
		if c := s.conns[cid]; c != nil {
			c.established = true
		}
		s.mu.Unlock()
	case "DISCONNECT":
		s.addBytes(cid, parseInt(env["bytes_received"]), parseInt(env["bytes_sent"]))
		s.mu.Lock()
		delete(s.conns, cid)
		s.mu.Unlock()
	}
}

// authorize returns "" to admit the client, or the reason it is refused.
func (s *session) authorize(cid uint64, env map[string]string, reauth bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	name, pass := env["username"], env["password"]
	u, ok := s.users[name]
	if reauth && pass == "" {
		// A renegotiation may omit the password; the account must still exist unchanged.
		if c := s.conns[cid]; ok && c != nil && c.user == name && equalSecret(u.Password, c.password) {
			return ""
		}
		return "invalid credentials"
	}
	if !ok || name == "" || !equalSecret(u.Password, pass) {
		return "invalid credentials"
	}
	if !reauth && u.MaxConns > 0 {
		n := 0
		for _, c := range s.conns {
			if c.user == name {
				n++
			}
		}
		if n >= u.MaxConns {
			return "too many connections"
		}
	}
	if c := s.conns[cid]; c != nil {
		c.password = pass
	} else {
		s.conns[cid] = &conn{user: name, password: pass}
	}
	return ""
}

func (s *session) addBytes(cid uint64, in, out int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.conns[cid]
	if c == nil || in < c.in || out < c.out {
		return
	}
	d := s.deltas[c.user]
	d.Up += in - c.in
	d.Down += out - c.out
	s.deltas[c.user] = d
	c.in, c.out = in, out
}

func equalSecret(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func parseUint(s string) uint64 {
	n, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	return n
}

func parseInt(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}
