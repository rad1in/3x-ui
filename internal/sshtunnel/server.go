package sshtunnel

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

const (
	handshakeTimeout = 30 * time.Second
	dialTimeout      = 10 * time.Second
	maxAuthTries     = 3
	serverVersion    = "SSH-2.0-xui"
	sessionNotice    = "This account is for tunnelling only; it has no shell.\r\n"
)

// failDelay slows password guessing; tests set it to zero.
var failDelay = 500 * time.Millisecond

var errTargetNotAllowed = errors.New("tunnelling to local or private addresses is not allowed")

// User is one account the server accepts.
type User struct {
	Password string
	MaxConns int
}

// Instance is one ssh inbound.
type Instance struct {
	Id         int
	Tag        string
	Listen     string
	Port       int
	HostKey    string
	Banner     string
	AllowLocal bool
	Users      map[string]User
}

// structuralFingerprint changes when the listener has to be rebuilt; user
// changes are applied to a running server instead.
func (i Instance) structuralFingerprint() string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%d|%s|%s|%t", i.Listen, i.Port, i.HostKey, i.Banner, i.AllowLocal))
	return fmt.Sprintf("%x", sum)
}

type usage struct{ up, down atomic.Int64 }

// liveConn is one authenticated SSH connection.
type liveConn struct {
	user     string
	password string
	started  time.Time
	conn     ssh.Conn
	done     chan struct{}
}

type server struct {
	inst Instance
	cfg  *ssh.ServerConfig
	ln   net.Listener

	mu     sync.Mutex
	users  map[string]User
	conns  map[*liveConn]struct{}
	usage  map[string]*usage
	closed bool
	wg     sync.WaitGroup
}

func startServer(inst Instance) (*server, error) {
	signer, err := ssh.ParsePrivateKey([]byte(inst.HostKey))
	if err != nil {
		return nil, fmt.Errorf("ssh inbound %d: host key: %w", inst.Id, err)
	}
	s := &server{
		inst:  inst,
		users: inst.Users,
		conns: map[*liveConn]struct{}{},
		usage: map[string]*usage{},
	}
	s.cfg = &ssh.ServerConfig{
		MaxAuthTries:  maxAuthTries,
		ServerVersion: serverVersion,
		PasswordCallback: func(cm ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			return s.authenticate(cm.User(), string(pw))
		},
		KeyboardInteractiveCallback: func(cm ssh.ConnMetadata, ask ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			answers, err := ask("", "", []string{"Password: "}, []bool{false})
			if err != nil || len(answers) != 1 {
				return nil, errors.New("keyboard-interactive failed")
			}
			return s.authenticate(cm.User(), answers[0])
		},
	}
	if inst.Banner != "" {
		banner := inst.Banner
		s.cfg.BannerCallback = func(ssh.ConnMetadata) string { return banner + "\r\n" }
	}
	s.cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", net.JoinHostPort(inst.Listen, strconv.Itoa(inst.Port)))
	if err != nil {
		return nil, fmt.Errorf("ssh inbound %d: %w", inst.Id, err)
	}
	s.ln = ln
	s.wg.Add(1)
	go s.acceptLoop()
	return s, nil
}

func (s *server) addr() net.Addr { return s.ln.Addr() }

func (s *server) acceptLoop() {
	defer s.wg.Done()
	for {
		nc, err := s.ln.Accept()
		if err != nil {
			s.mu.Lock()
			s.closed = true
			s.mu.Unlock()
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.serveConn(nc)
		}()
	}
}

func digest(v string) [32]byte { return sha256.Sum256([]byte(v)) }

func (s *server) authenticate(user, password string) (*ssh.Permissions, error) {
	s.mu.Lock()
	u, ok := s.users[user]
	s.mu.Unlock()
	want, got := digest(u.Password), digest(password)
	if !ok || subtle.ConstantTimeCompare(want[:], got[:]) != 1 {
		time.Sleep(failDelay)
		return nil, errors.New("invalid login")
	}
	return &ssh.Permissions{Extensions: map[string]string{"user": user, "password": password}}, nil
}

func (s *server) serveConn(nc net.Conn) {
	_ = nc.SetDeadline(time.Now().Add(handshakeTimeout))
	sc, chans, reqs, err := ssh.NewServerConn(nc, s.cfg)
	if err != nil {
		_ = nc.Close()
		return
	}
	_ = nc.SetDeadline(time.Time{})
	lc := &liveConn{
		user: sc.Permissions.Extensions["user"], password: sc.Permissions.Extensions["password"],
		started: time.Now(), conn: sc, done: make(chan struct{}),
	}
	if !s.admit(lc) {
		_ = sc.Close()
		return
	}
	go ssh.DiscardRequests(reqs)
	go func() {
		_ = sc.Wait()
		close(lc.done)
		s.mu.Lock()
		delete(s.conns, lc)
		s.mu.Unlock()
	}()
	for ch := range chans {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleChannel(lc, ch)
		}()
	}
	<-lc.done
}

// admit registers the connection unless the account was removed or
// re-passworded since it logged in, or is at its connection limit.
func (s *server) admit(lc *liveConn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[lc.user]
	if s.closed || !ok || u.Password != lc.password {
		return false
	}
	if u.MaxConns > 0 {
		live := 0
		for c := range s.conns {
			if c.user == lc.user {
				live++
			}
		}
		if live >= u.MaxConns {
			return false
		}
	}
	s.conns[lc] = struct{}{}
	return true
}

// setUsers applies an account change without a restart: connections of a
// removed or re-passworded account end, and an account over its limit keeps
// its oldest connections.
func (s *server) setUsers(users map[string]User) {
	s.mu.Lock()
	s.users = users
	var doomed []*liveConn
	perUser := map[string][]*liveConn{}
	for c := range s.conns {
		u, ok := users[c.user]
		if !ok || u.Password != c.password {
			doomed = append(doomed, c)
			continue
		}
		perUser[c.user] = append(perUser[c.user], c)
	}
	for user, links := range perUser {
		limit := users[user].MaxConns
		if limit <= 0 || len(links) <= limit {
			continue
		}
		slices.SortFunc(links, func(a, b *liveConn) int { return a.started.Compare(b.started) })
		doomed = append(doomed, links[limit:]...)
	}
	s.mu.Unlock()
	for _, c := range doomed {
		_ = c.conn.Close()
	}
}

func (s *server) kick(user string) int {
	s.mu.Lock()
	var doomed []*liveConn
	for c := range s.conns {
		if c.user == user {
			doomed = append(doomed, c)
		}
	}
	s.mu.Unlock()
	for _, c := range doomed {
		_ = c.conn.Close()
	}
	return len(doomed)
}

func (s *server) usageOf(user string) *usage {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.usage[user]
	if u == nil {
		u = new(usage)
		s.usage[user] = u
	}
	return u
}

// collect returns each user's bytes since the last call and who is connected.
func (s *server) collect() (map[string][2]int64, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	deltas := map[string][2]int64{}
	for user, u := range s.usage {
		up, down := u.up.Swap(0), u.down.Swap(0)
		if up != 0 || down != 0 {
			deltas[user] = [2]int64{up, down}
		}
	}
	seen := map[string]bool{}
	var online []string
	for c := range s.conns {
		if !seen[c.user] {
			seen[c.user] = true
			online = append(online, c.user)
		}
	}
	slices.Sort(online)
	return deltas, online
}

func (s *server) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed
}

func (s *server) stop() {
	s.mu.Lock()
	s.closed = true
	conns := make([]*liveConn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	_ = s.ln.Close()
	for _, c := range conns {
		_ = c.conn.Close()
	}
	s.wg.Wait()
}

type directTCPIP struct {
	Host     string
	Port     uint32
	OrigHost string
	OrigPort uint32
}

func (s *server) handleChannel(lc *liveConn, nch ssh.NewChannel) {
	switch nch.ChannelType() {
	case "direct-tcpip":
		s.handleDirect(lc, nch)
	case "session":
		handleSession(nch)
	default:
		_ = nch.Reject(ssh.UnknownChannelType, "only port forwarding is served")
	}
}

func (s *server) handleDirect(lc *liveConn, nch ssh.NewChannel) {
	var p directTCPIP
	if err := ssh.Unmarshal(nch.ExtraData(), &p); err != nil || p.Port == 0 || p.Port > 65535 {
		_ = nch.Reject(ssh.ConnectionFailed, "bad forwarding request")
		return
	}
	dialer := net.Dialer{Timeout: dialTimeout}
	if !s.inst.AllowLocal {
		dialer.Control = refuseLocalTarget
	}
	target, err := dialer.DialContext(context.Background(), "tcp", net.JoinHostPort(p.Host, strconv.Itoa(int(p.Port))))
	if err != nil {
		_ = nch.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	ch, reqs, err := nch.Accept()
	if err != nil {
		_ = target.Close()
		return
	}
	go ssh.DiscardRequests(reqs)

	u := s.usageOf(lc.user)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-lc.done:
			_ = target.Close()
		case <-stop:
		}
	}()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(countingWriter{target, &u.up}, ch)
		closeWrite(target)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(countingWriter{ch, &u.down}, target)
		_ = ch.CloseWrite()
	}()
	wg.Wait()
	_ = ch.Close()
	_ = target.Close()
}

type countingWriter struct {
	w io.Writer
	n *atomic.Int64
}

func (c countingWriter) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	c.n.Add(int64(n))
	return n, err
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
}

// handleSession keeps the channel open without a shell: tunnelling apps open
// one as a keepalive, while a command is refused.
func handleSession(nch ssh.NewChannel) {
	ch, reqs, err := nch.Accept()
	if err != nil {
		return
	}
	for r := range reqs {
		switch r.Type {
		case "pty-req", "env", "window-change":
			_ = r.Reply(true, nil)
		case "shell":
			_ = r.Reply(true, nil)
			_, _ = io.WriteString(ch, sessionNotice)
		default:
			_ = r.Reply(false, nil)
			if r.Type == "exec" || r.Type == "subsystem" {
				_ = ch.Close()
			}
		}
	}
}

var carrierGradeNAT = netip.MustParsePrefix("100.64.0.0/10")

// refuseLocalTarget runs on the resolved address of every connection attempt,
// so a name that resolves to a local address cannot slip past.
func refuseLocalTarget(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}
	if localAddress(ip.Unmap().WithZone("")) {
		return errTargetNotAllowed
	}
	return nil
}

func localAddress(ip netip.Addr) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || carrierGradeNAT.Contains(ip) {
		return true
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return true
	}
	for _, a := range addrs {
		if p, err := netip.ParsePrefix(a.String()); err == nil && p.Addr().Unmap() == ip {
			return true
		}
	}
	return false
}

func logStart(inst Instance) {
	logger.Infof("ssh: started inbound %d on %s", inst.Id, net.JoinHostPort(inst.Listen, strconv.Itoa(inst.Port)))
}
