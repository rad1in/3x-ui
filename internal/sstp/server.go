package sstp

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// helloInterval is a var so tests need not wait a minute for the keepalive.
var helloInterval = 60 * time.Second

const (
	negotiationTimeout = 60 * time.Second
	maxHeaderBytes     = 8192
	// Caps keep unauthenticated peers from spawning pppd without bound.
	maxSessions = 1024
	maxPerIP    = 32
	maxNaks     = 3
)

// pppConn is the PPP peer of one SSTP session: asynchronous HDLC both ways.
type pppConn interface {
	io.ReadWriteCloser
	Done() <-chan struct{}
}

// spawner starts the PPP peer for a session, local and remote being the
// tunnel's two addresses.
type spawner func(inst Instance, local, remote netip.Addr) (pppConn, error)

type usage struct{ up, down atomic.Int64 }

type session struct {
	srv      *server
	conn     net.Conn
	remoteIP string

	wmu sync.Mutex // serializes packet writes on conn

	// Guarded by server.mu.
	chapName string
	user     string
	password string
	started  time.Time
	remote   netip.Addr

	ppp       pppConn
	closeOnce sync.Once
	done      chan struct{}
}

type server struct {
	inst  Instance
	cfg   *tls.Config
	ln    net.Listener
	spawn spawner

	mu       sync.Mutex
	users    map[string]User
	sessions map[*session]struct{}
	perIP    map[string]int
	admitted int
	addrs    map[netip.Addr]bool
	usage    map[string]*usage
	closed   bool
	wg       sync.WaitGroup
}

func startServer(inst Instance, spawn spawner) (*server, error) {
	cert, err := tls.X509KeyPair([]byte(inst.PKI.ServerCert), []byte(inst.PKI.ServerKey))
	if err != nil {
		return nil, fmt.Errorf("sstp inbound %d: certificate: %w", inst.Id, err)
	}
	s := &server{
		inst:     inst,
		cfg:      &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
		spawn:    spawn,
		users:    inst.Users,
		sessions: map[*session]struct{}{},
		perIP:    map[string]int{},
		addrs:    map[netip.Addr]bool{},
		usage:    map[string]*usage{},
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(inst.Listen, strconv.Itoa(inst.Port)))
	if err != nil {
		return nil, fmt.Errorf("sstp inbound %d: %w", inst.Id, err)
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

func (s *server) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed
}

// admit counts the peer against the caps; release undoes it.
func (s *server) admit(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.admitted >= maxSessions || s.perIP[ip] >= maxPerIP {
		return false
	}
	s.admitted++
	s.perIP[ip]++
	return true
}

func (s *server) release(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.admitted--
	if s.perIP[ip]--; s.perIP[ip] <= 0 {
		delete(s.perIP, ip)
	}
}

func (s *server) serveConn(nc net.Conn) {
	host, _, _ := net.SplitHostPort(nc.RemoteAddr().String())
	if !s.admit(host) {
		_ = nc.Close()
		return
	}
	defer s.release(host)

	tc := tls.Server(nc, s.cfg)
	defer tc.Close()
	_ = tc.SetDeadline(time.Now().Add(negotiationTimeout))
	if err := tc.Handshake(); err != nil {
		return
	}
	br := bufio.NewReaderSize(tc, 8192)
	if !upgradeHTTP(tc, br) {
		return
	}
	sess := &session{srv: s, conn: tc, remoteIP: host, done: make(chan struct{})}
	sess.run(br)
}

// upgradeHTTP reads the client's SSTP_DUPLEX_POST request and answers it; any
// other request gets a plain 404 like a web server with nothing there.
func upgradeHTTP(w io.Writer, br *bufio.Reader) bool {
	var line string
	total := 0
	first := true
	ok := false
	for {
		l, err := br.ReadString('\n')
		total += len(l)
		if err != nil || total > maxHeaderBytes {
			return false
		}
		l = strings.TrimRight(l, "\r\n")
		if l == "" {
			break
		}
		if first {
			line, first = l, false
			f := strings.Fields(line)
			ok = len(f) == 3 && f[0] == httpMethod && strings.EqualFold(f[1], httpURI) && strings.HasPrefix(f[2], "HTTP/1.")
		}
	}
	if !ok {
		_, _ = io.WriteString(w, "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return false
	}
	_, err := io.WriteString(w, "HTTP/1.1 200 OK\r\nContent-Length: 18446744073709551615\r\nDate: "+
		time.Now().UTC().Format(time.RFC1123)+"\r\n\r\n")
	return err == nil
}

func (sess *session) write(b []byte) error {
	sess.wmu.Lock()
	defer sess.wmu.Unlock()
	_ = sess.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	_, err := sess.conn.Write(b)
	return err
}

// close ends the session from either side; it is safe to call repeatedly.
func (sess *session) close() {
	sess.closeOnce.Do(func() {
		close(sess.done)
		_ = sess.conn.Close()
		if sess.ppp != nil {
			_ = sess.ppp.Close()
		}
	})
}

func (sess *session) run(br *bufio.Reader) {
	s := sess.srv
	defer sess.finish()

	_ = sess.conn.SetReadDeadline(time.Now().Add(negotiationTimeout))
	if !sess.negotiate(br) {
		return
	}
	s.mu.Lock()
	s.sessions[sess] = struct{}{}
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return
	}

	go sess.pump()

	echoPending := false
	for {
		_ = sess.conn.SetReadDeadline(time.Now().Add(helloInterval))
		p, err := readPacket(br)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() && !echoPending {
				echoPending = true
				if sess.write(controlPacket(msgEchoRequest)) != nil {
					return
				}
				continue
			}
			return
		}
		echoPending = false
		if !p.control {
			sess.fromClient(p.data)
			continue
		}
		switch p.msg {
		case msgEchoRequest:
			if sess.write(controlPacket(msgEchoResponse)) != nil {
				return
			}
		case msgDisconnect:
			_ = sess.write(controlPacket(msgDisconnectAck))
			return
		case msgAbort, msgDisconnectAck:
			return
		}
	}
}

// negotiate runs the SSTP handshake up to the Call Connect Ack and starts the
// PPP peer; false means the session is over.
func (sess *session) negotiate(br *bufio.Reader) bool {
	naks := 0
	for {
		p, err := readPacket(br)
		if err != nil || !p.control {
			return false
		}
		if p.msg != msgConnectRequest {
			_ = sess.write(controlPacket(msgAbort, statusInfo(0, statusBadFrm)))
			return false
		}
		if !requestsPPP(p) {
			if naks++; naks > maxNaks {
				_ = sess.write(controlPacket(msgAbort, statusInfo(0, statusBadFrm)))
				return false
			}
			nak := statusInfo(attrProtoID, statusBadVal)
			nak.value = append(nak.value, 0, 0)
			if sess.write(controlPacket(msgConnectNak, nak)) != nil {
				return false
			}
			continue
		}
		break
	}

	local, remote, ok := sess.srv.allocate()
	if !ok {
		_ = sess.write(controlPacket(msgAbort, statusInfo(0, statusBadFrm)))
		return false
	}
	sess.remote = remote
	ppp, err := sess.srv.spawn(sess.srv.inst, local, remote)
	if err != nil {
		_ = sess.write(controlPacket(msgAbort, statusInfo(0, statusBadFrm)))
		return false
	}
	sess.ppp = ppp
	nonce := make([]byte, nonceSize)
	_, _ = rand.Read(nonce)
	return sess.write(connectAck(nonce)) == nil
}

func requestsPPP(p *packet) bool {
	for _, a := range p.attrs {
		if a.id == attrProtoID && len(a.value) >= 2 && int(a.value[0])<<8|int(a.value[1]) == protoPPP {
			return true
		}
	}
	return false
}

// allocate hands out the tunnel's two addresses for a session.
func (s *server) allocate() (local, remote netip.Addr, ok bool) {
	local, first, last := poolRange(s.inst.Pool)
	s.mu.Lock()
	defer s.mu.Unlock()
	for a := first; a.Compare(last) <= 0; a = a.Next() {
		if !s.addrs[a] {
			s.addrs[a] = true
			return local, a, true
		}
	}
	return netip.Addr{}, netip.Addr{}, false
}

func (sess *session) finish() {
	s := sess.srv
	sess.close()
	s.mu.Lock()
	delete(s.sessions, sess)
	if sess.remote.IsValid() {
		delete(s.addrs, sess.remote)
	}
	s.mu.Unlock()
}

// fromClient hands a PPP frame from the client to pppd, noting who is logging
// in and what they send.
func (sess *session) fromClient(frame []byte) {
	if len(frame) == 0 || len(frame) > maxFrame {
		return
	}
	if proto, payload, ok := pppProtocol(frame); ok {
		switch {
		case proto == protoCHAP:
			if code, name, ok := chapPacket(payload); ok && code == chapResponse {
				sess.srv.mu.Lock()
				sess.chapName = name
				sess.srv.mu.Unlock()
			}
		case isUserData(proto):
			if u := sess.usageIfAuthed(); u != nil {
				u.up.Add(int64(len(payload)))
			}
		}
	}
	if _, err := sess.ppp.Write(hdlcEncode(frame)); err != nil {
		sess.close()
	}
}

func (sess *session) usageIfAuthed() *usage {
	s := sess.srv
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess.user == "" {
		return nil
	}
	u := s.usage[sess.user]
	if u == nil {
		u = new(usage)
		s.usage[sess.user] = u
	}
	return u
}

// pump carries pppd's frames to the client until pppd or the session ends.
func (sess *session) pump() {
	var dec hdlcDecoder
	buf := make([]byte, 4096)
	for {
		n, err := sess.ppp.Read(buf)
		dec.feed(buf[:n], sess.toClient)
		if err != nil {
			break
		}
	}
	select {
	case <-sess.done:
	default:
		// pppd ended on its own (bad login, hang-up): tell the client, then let go.
		_ = sess.write(controlPacket(msgDisconnect, statusInfo(0, statusNoErr)))
		select {
		case <-sess.done:
		case <-time.After(2 * time.Second):
		}
		sess.close()
	}
}

// toClient sends one frame from pppd to the client. A CHAP Success is held
// back until the account is known to belong to this inbound: pppd's secrets
// file also holds other protocols' logins.
func (sess *session) toClient(frame []byte) {
	proto, payload, ok := pppProtocol(frame)
	if ok {
		switch {
		case proto == protoCHAP:
			if code, _, ok := chapPacket(payload); ok && code == chapSuccess && !sess.authorize() {
				sess.close()
				return
			}
		case isUserData(proto):
			if u := sess.usageIfAuthed(); u != nil {
				u.down.Add(int64(len(payload)))
			}
		}
	}
	if sess.write(dataPacket(frame)) != nil {
		sess.close()
	}
}

// authorize admits the account pppd just authenticated if this inbound serves
// it and it is under its connection limit.
func (sess *session) authorize() bool {
	s := sess.srv
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[sess.chapName]
	if !ok {
		return false
	}
	if u.MaxConns > 0 {
		live := 0
		for o := range s.sessions {
			if o != sess && o.user == sess.chapName {
				live++
			}
		}
		if live >= u.MaxConns {
			return false
		}
	}
	sess.user, sess.password, sess.started = sess.chapName, u.Password, time.Now()
	return true
}

// setUsers applies an account change without a restart: the sessions of a
// removed or re-passworded account end, and an account over its limit keeps
// its oldest sessions.
func (s *server) setUsers(users map[string]User) {
	s.mu.Lock()
	s.users = users
	var doomed []*session
	perUser := map[string][]*session{}
	for sess := range s.sessions {
		if sess.user == "" {
			continue
		}
		u, ok := users[sess.user]
		if !ok || u.Password != sess.password {
			doomed = append(doomed, sess)
			continue
		}
		perUser[sess.user] = append(perUser[sess.user], sess)
	}
	for user, links := range perUser {
		limit := users[user].MaxConns
		if limit <= 0 || len(links) <= limit {
			continue
		}
		slices.SortFunc(links, func(a, b *session) int { return a.started.Compare(b.started) })
		doomed = append(doomed, links[limit:]...)
	}
	s.mu.Unlock()
	for _, sess := range doomed {
		sess.close()
	}
}

func (s *server) kick(user string) int {
	s.mu.Lock()
	var doomed []*session
	for sess := range s.sessions {
		if sess.user == user {
			doomed = append(doomed, sess)
		}
	}
	s.mu.Unlock()
	for _, sess := range doomed {
		sess.close()
	}
	return len(doomed)
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
	for sess := range s.sessions {
		if sess.user != "" && !seen[sess.user] {
			seen[sess.user] = true
			online = append(online, sess.user)
		}
	}
	slices.Sort(online)
	return deltas, online
}

func (s *server) stop() {
	s.mu.Lock()
	s.closed = true
	live := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		live = append(live, sess)
	}
	s.mu.Unlock()
	_ = s.ln.Close()
	for _, sess := range live {
		sess.close()
	}
	s.wg.Wait()
}
