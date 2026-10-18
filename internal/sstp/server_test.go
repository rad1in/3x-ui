package sstp

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/ipsec"
)

var (
	pkiOnce sync.Once
	testPKI ipsec.PKI
)

func pki(t *testing.T) ipsec.PKI {
	t.Helper()
	pkiOnce.Do(func() {
		var err error
		if testPKI, err = ipsec.PKIFor(ipsec.PKI{}, "127.0.0.1"); err != nil {
			t.Fatal(err)
		}
	})
	return testPKI
}

// fakePPP stands in for pppd, which needs a PPP kernel: it speaks async HDLC,
// answers every CHAP Response with a Success and echoes IP packets back.
type fakePPP struct {
	dec    hdlcDecoder
	outR   *io.PipeReader
	outW   *io.PipeWriter
	done   chan struct{}
	once   sync.Once
	local  netip.Addr
	remote netip.Addr
}

func newFakePPP() *fakePPP {
	r, w := io.Pipe()
	return &fakePPP{outR: r, outW: w, done: make(chan struct{})}
}

func (f *fakePPP) Read(b []byte) (int, error) { return f.outR.Read(b) }

func (f *fakePPP) Write(b []byte) (int, error) {
	f.dec.feed(b, func(frame []byte) {
		proto, payload, ok := pppProtocol(frame)
		if !ok {
			return
		}
		switch {
		case proto == protoCHAP:
			if code, _, ok := chapPacket(payload); ok && code == chapResponse {
				_, _ = f.outW.Write(hdlcEncode([]byte{0xff, 0x03, 0xc2, 0x23, chapSuccess, 0x07, 0x00, 0x04}))
			}
		case isUserData(proto):
			_, _ = f.outW.Write(hdlcEncode(frame))
		}
	})
	return len(b), nil
}

func (f *fakePPP) Close() error {
	f.once.Do(func() { close(f.done); _ = f.outW.Close() })
	return nil
}

func (f *fakePPP) Done() <-chan struct{} { return f.done }

// exit simulates pppd quitting on its own.
func (f *fakePPP) exit() { _ = f.Close() }

type spawns struct {
	mu    sync.Mutex
	all   []*fakePPP
	count atomic.Int32
	fail  error
}

func (s *spawns) spawn(_ Instance, local, remote netip.Addr) (pppConn, error) {
	s.count.Add(1)
	if s.fail != nil {
		return nil, s.fail
	}
	f := newFakePPP()
	f.local, f.remote = local, remote
	s.mu.Lock()
	s.all = append(s.all, f)
	s.mu.Unlock()
	return f, nil
}

func (s *spawns) last(t *testing.T) *fakePPP {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.all) == 0 {
		t.Fatal("no PPP peer was started")
	}
	return s.all[len(s.all)-1]
}

func testInstance(t *testing.T) Instance {
	return Instance{
		Id: 3, Tag: "sstp-3", Listen: "127.0.0.1", Address: "127.0.0.1",
		Pool: netip.MustParsePrefix("10.91.0.0/24"), PKI: pki(t),
		Users: map[string]User{"alice@x": {Password: "pa"}, "bob@x": {Password: "pb", MaxConns: 1}},
	}
}

func startTest(t *testing.T, inst Instance) (*server, *spawns) {
	t.Helper()
	sp := &spawns{}
	s, err := startServer(inst, sp.spawn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.stop)
	return s, sp
}

type client struct {
	conn net.Conn
	br   *bufio.Reader
	dec  hdlcDecoder
}

func (c *client) send(t *testing.T, b []byte) {
	t.Helper()
	if _, err := c.conn.Write(b); err != nil {
		t.Fatal(err)
	}
}

func (c *client) next(t *testing.T) *packet {
	t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	p, err := readPacket(c.br)
	if err != nil {
		t.Fatalf("reading a packet: %v", err)
	}
	return p
}

// closed reports whether the server ends the connection soon.
func (c *client) closed() bool {
	_ = c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, err := readPacket(c.br); err != nil {
			var ne net.Error
			return !(errors.As(err, &ne) && ne.Timeout())
		}
	}
}

func dialRaw(t *testing.T, s *server) *client {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(pki(t).CA))
	conn, err := tls.Dial("tcp", s.addr().String(), &tls.Config{RootCAs: pool, ServerName: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &client{conn: conn, br: bufio.NewReader(conn)}
}

const upgradeRequest = "SSTP_DUPLEX_POST " + httpURI + " HTTP/1.1\r\nHost: 127.0.0.1\r\n" +
	"Content-Length: 18446744073709551615\r\nSSTPCORRELATIONID: {00000000-0000-0000-0000-000000000000}\r\n\r\n"

// dialSSTP gets through the HTTP upgrade.
func dialSSTP(t *testing.T, s *server) *client {
	t.Helper()
	c := dialRaw(t, s)
	c.send(t, []byte(upgradeRequest))
	status, err := c.br.ReadString('\n')
	if err != nil || !strings.HasPrefix(status, "HTTP/1.1 200") {
		t.Fatalf("upgrade answered %q, %v", status, err)
	}
	for {
		line, err := c.br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			return c
		}
	}
}

var connectRequest = controlPacket(msgConnectRequest, attr{id: attrProtoID, value: []byte{0, protoPPP}})

// connect runs the handshake up to the Ack.
func connect(t *testing.T, s *server) *client {
	t.Helper()
	c := dialSSTP(t, s)
	c.send(t, connectRequest)
	ack := c.next(t)
	if !ack.control || ack.msg != msgConnectAck || len(ack.attrs) != 1 || ack.attrs[0].id != attrBindReq || len(ack.attrs[0].value) != 4+nonceSize {
		t.Fatalf("connect answered %+v", ack)
	}
	return c
}

func (c *client) sendFrame(t *testing.T, frame []byte) { c.send(t, dataPacket(frame)) }

func ipFrame(payload int) []byte {
	return append([]byte{0xff, 0x03, 0x00, 0x21}, bytes.Repeat([]byte{0x45}, payload)...)
}

// login drives a CHAP Response for user and returns whether the Success came back.
func login(t *testing.T, c *client, user string) bool {
	t.Helper()
	c.sendFrame(t, chapResponseFrame(user))
	_ = c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		p, err := readPacket(c.br)
		if err != nil {
			return false
		}
		if p.control {
			continue
		}
		if proto, payload, ok := pppProtocol(p.data); ok && proto == protoCHAP {
			if code, _, ok := chapPacket(payload); ok && code == chapSuccess {
				return true
			}
		}
	}
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestHandshakeCarriesPPPBothWaysAndMetersTheUser(t *testing.T) {
	s, sp := startTest(t, testInstance(t))
	c := connect(t, s)
	if !login(t, c, "alice@x") {
		t.Fatal("alice's CHAP Success never reached her")
	}

	c.sendFrame(t, ipFrame(1000))
	echo := c.next(t)
	if proto, payload, ok := pppProtocol(echo.data); echo.control || !ok || proto != protoIPv4 || len(payload) != 1000 {
		t.Fatalf("echoed data = %+v", echo)
	}

	var up, down int64
	waitUntil(t, "alice's bytes to be counted", func() bool {
		deltas, _ := s.collect()
		up += deltas["alice@x"][0]
		down += deltas["alice@x"][1]
		return up == 1000 && down == 1000
	})
	if _, online := s.collect(); len(online) != 1 || online[0] != "alice@x" {
		t.Fatalf("online = %v", online)
	}
	if f := sp.last(t); f.local != netip.MustParseAddr("10.91.0.1") || f.remote != netip.MustParseAddr("10.91.0.2") {
		t.Fatalf("tunnel addresses = %s -> %s", f.local, f.remote)
	}
}

func TestLoginOfAnotherInboundsUserIsNotForwardedAndEndsTheSession(t *testing.T) {
	s, _ := startTest(t, testInstance(t))
	c := connect(t, s)
	// pppd reads one chap-secrets file for every PPP inbound, so it may accept a name this inbound does not serve.
	if login(t, c, "mallory@x") {
		t.Fatal("a user this inbound does not serve was told the login succeeded")
	}
	if !c.closed() {
		t.Fatal("the session stayed open after an unauthorised login")
	}
	waitUntil(t, "the session to be gone", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return len(s.sessions) == 0
	})
}

func TestOnlyTheRightHTTPRequestStartsPPP(t *testing.T) {
	s, sp := startTest(t, testInstance(t))
	for name, request := range map[string]string{
		"wrong method": "GET " + httpURI + " HTTP/1.1\r\nHost: x\r\n\r\n",
		"wrong path":   "SSTP_DUPLEX_POST /index.html HTTP/1.1\r\nHost: x\r\n\r\n",
	} {
		c := dialRaw(t, s)
		c.send(t, []byte(request))
		status, _ := c.br.ReadString('\n')
		if !strings.HasPrefix(status, "HTTP/1.1 404") {
			t.Errorf("%s answered %q, want a 404", name, status)
		}
	}
	if n := sp.count.Load(); n != 0 {
		t.Fatalf("%d PPP peers were started for requests that are not SSTP", n)
	}
}

func TestNonPPPProtocolIsRefusedThenAborted(t *testing.T) {
	s, sp := startTest(t, testInstance(t))
	c := dialSSTP(t, s)
	notPPP := controlPacket(msgConnectRequest, attr{id: attrProtoID, value: []byte{0, 0x07}})
	for range maxNaks {
		c.send(t, notPPP)
		if p := c.next(t); p.msg != msgConnectNak {
			t.Fatalf("answered %+v, want a Nak", p)
		}
	}
	c.send(t, notPPP)
	if p := c.next(t); p.msg != msgAbort {
		t.Fatalf("answered %+v, want an Abort after repeated Naks", p)
	}
	if sp.count.Load() != 0 {
		t.Fatal("a PPP peer was started for a protocol other than PPP")
	}
}

func TestEchoAndDisconnectAreAnswered(t *testing.T) {
	s, _ := startTest(t, testInstance(t))
	c := connect(t, s)
	c.send(t, controlPacket(msgEchoRequest))
	if p := c.next(t); p.msg != msgEchoResponse {
		t.Fatalf("echo answered %+v", p)
	}
	c.send(t, controlPacket(msgDisconnect, statusInfo(0, statusNoErr)))
	if p := c.next(t); p.msg != msgDisconnectAck {
		t.Fatalf("disconnect answered %+v", p)
	}
	if !c.closed() {
		t.Fatal("the connection stayed open after Disconnect")
	}
}

func TestPPPExitTellsTheClientAndFreesItsAddress(t *testing.T) {
	s, sp := startTest(t, testInstance(t))
	c := connect(t, s)
	sp.last(t).exit()
	if p := c.next(t); p.msg != msgDisconnect {
		t.Fatalf("pppd exit answered %+v, want a Call Disconnect", p)
	}
	c.send(t, controlPacket(msgDisconnectAck))
	waitUntil(t, "the tunnel address to be released", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return len(s.addrs) == 0
	})
	next := connect(t, s)
	_ = next
	if f := sp.last(t); f.remote != netip.MustParseAddr("10.91.0.2") {
		t.Fatalf("the freed address was not reused: %s", f.remote)
	}
}

func TestEachSessionGetsItsOwnAddress(t *testing.T) {
	s, sp := startTest(t, testInstance(t))
	connect(t, s)
	connect(t, s)
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if len(sp.all) != 2 || sp.all[0].remote == sp.all[1].remote {
		t.Fatalf("two sessions share an address: %v", sp.all)
	}
}

func TestPPPStartFailureAbortsTheCall(t *testing.T) {
	s, sp := startTest(t, testInstance(t))
	sp.fail = errors.New("no pppd")
	c := dialSSTP(t, s)
	c.send(t, connectRequest)
	if p := c.next(t); p.msg != msgAbort {
		t.Fatalf("answered %+v, want an Abort", p)
	}
}

func TestAccountChangesEndSessionsAndLimitsApply(t *testing.T) {
	s, _ := startTest(t, testInstance(t))
	alice := connect(t, s)
	if !login(t, alice, "alice@x") {
		t.Fatal("alice cannot log in")
	}

	// Another account changing leaves her connected.
	s.setUsers(map[string]User{"alice@x": {Password: "pa"}, "bob@x": {Password: "new"}})
	_ = alice.conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, err := readPacket(alice.br); err == nil || !isTimeout(err) {
		t.Fatalf("alice was dropped by an unrelated change: %v", err)
	}

	s.setUsers(map[string]User{"alice@x": {Password: "rotated"}})
	if !alice.closed() {
		t.Fatal("alice stayed connected after her password changed")
	}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func TestConnectionLimitKeepsTheOldestSession(t *testing.T) {
	s, _ := startTest(t, testInstance(t))
	first := connect(t, s)
	if !login(t, first, "bob@x") {
		t.Fatal("bob's first login failed")
	}
	second := connect(t, s)
	if login(t, second, "bob@x") {
		t.Fatal("bob's second session was admitted past his limit of 1")
	}
	first.send(t, controlPacket(msgEchoRequest))
	if p := first.next(t); p.msg != msgEchoResponse {
		t.Fatalf("bob's first session was disturbed: %+v", p)
	}
}

func TestKickEndsOnlyThatUsersSessions(t *testing.T) {
	s, _ := startTest(t, testInstance(t))
	alice, bob := connect(t, s), connect(t, s)
	if !login(t, alice, "alice@x") || !login(t, bob, "bob@x") {
		t.Fatal("logins failed")
	}
	if got := s.kick("alice@x"); got != 1 {
		t.Fatalf("kick = %d, want 1", got)
	}
	if !alice.closed() {
		t.Fatal("alice is still connected")
	}
	bob.send(t, controlPacket(msgEchoRequest))
	if p := bob.next(t); p.msg != msgEchoResponse {
		t.Fatalf("bob was disturbed: %+v", p)
	}
}

func TestSilentClientIsProbedThenDropped(t *testing.T) {
	prev := helloInterval
	helloInterval = 150 * time.Millisecond
	t.Cleanup(func() { helloInterval = prev })
	s, _ := startTest(t, testInstance(t))

	// A client that answers the probe stays; one that does not is let go after a second silence.
	alive := connect(t, s)
	for range 3 {
		if p := alive.next(t); p.msg != msgEchoRequest {
			t.Fatalf("idle client probed with %+v, want an Echo Request", p)
		}
		alive.send(t, controlPacket(msgEchoResponse))
	}
	silent := connect(t, s)
	if p := silent.next(t); p.msg != msgEchoRequest {
		t.Fatalf("idle client probed with %+v, want an Echo Request", p)
	}
	if !silent.closed() {
		t.Fatal("a client that never answered the probe stayed connected")
	}
}

func TestPeersBeyondThePerAddressCapAreDropped(t *testing.T) {
	s, _ := startTest(t, testInstance(t))
	var held []net.Conn
	t.Cleanup(func() {
		for _, c := range held {
			_ = c.Close()
		}
	})
	for range maxPerIP {
		c, err := net.Dial("tcp", s.addr().String())
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, c)
	}
	waitUntil(t, "the held connections to be admitted", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.perIP["127.0.0.1"] == maxPerIP
	})
	extra, err := net.Dial("tcp", s.addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer extra.Close()
	_ = extra.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := extra.Read(make([]byte, 1)); err != io.EOF && !errors.Is(err, net.ErrClosed) && !strings.Contains(err.Error(), "reset") {
		t.Fatalf("the connection past the cap was kept open: %v", err)
	}
}
