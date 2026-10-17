package openvpn

import (
	"bufio"
	"net"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeServer plays the openvpn side of the management socket.
type fakeServer struct {
	t     *testing.T
	conn  net.Conn
	lines chan string
}

func newSessionPair(t *testing.T, users map[string]User) (*session, *fakeServer) {
	t.Helper()
	ours, theirs := net.Pipe()
	s := newSession(ours)
	s.SetUsers(users)
	go s.serve()
	t.Cleanup(func() { _ = ours.Close(); _ = theirs.Close() })
	f := &fakeServer{t: t, conn: theirs, lines: make(chan string, 64)}
	go func() {
		r := bufio.NewReader(theirs)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				close(f.lines)
				return
			}
			f.lines <- strings.TrimSpace(line)
		}
	}()
	return s, f
}

func (f *fakeServer) send(lines ...string) {
	f.t.Helper()
	_ = f.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := f.conn.Write([]byte(strings.Join(lines, "\r\n") + "\r\n")); err != nil {
		f.t.Fatalf("write to session: %v", err)
	}
}

// expect reads command lines from the session until one matches want.
func (f *fakeServer) expect(want string) {
	f.t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case line, ok := <-f.lines:
			if !ok {
				f.t.Fatalf("session closed while waiting for %q", want)
			}
			if line == want {
				return
			}
		case <-timeout:
			f.t.Fatalf("timed out waiting for %q", want)
		}
	}
}

func (f *fakeServer) connect(cid, kid, user, pass string) {
	f.send(">CLIENT:CONNECT,"+cid+","+kid, ">CLIENT:ENV,username="+user, ">CLIENT:ENV,password="+pass,
		">CLIENT:ENV,untrusted_ip=198.51.100.7", ">CLIENT:ENV,END")
}

func waitCollect(t *testing.T, s *session, cond func(map[string]Counters, []string) bool) (map[string]Counters, []string) {
	t.Helper()
	total := map[string]Counters{}
	var online []string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		d, on := s.Collect()
		for u, c := range d {
			acc := total[u]
			acc.Up += c.Up
			acc.Down += c.Down
			total[u] = acc
		}
		online = on
		if cond(total, online) {
			return total, online
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met; totals %v online %v", total, online)
	return nil, nil
}

func TestSessionAuthorizesOnlyValidCredentials(t *testing.T) {
	_, srv := newSessionPair(t, map[string]User{"alice": {Password: "s3cret"}})
	srv.send(">INFO:OpenVPN Management Interface Version 5 -- type 'help' for more info")
	srv.expect("bytecount 5")

	srv.connect("0", "1", "alice", "s3cret")
	srv.expect("client-auth-nt 0 1")
	srv.connect("1", "1", "alice", "wrong")
	srv.expect(`client-deny 1 1 "invalid credentials"`)
	srv.connect("2", "1", "mallory", "s3cret")
	srv.expect(`client-deny 2 1 "invalid credentials"`)
}

func TestSessionMetersTrafficPerUserAcrossConnections(t *testing.T) {
	s, srv := newSessionPair(t, map[string]User{"alice": {Password: "pw"}})
	srv.connect("7", "1", "alice", "pw")
	srv.expect("client-auth-nt 7 1")
	srv.connect("8", "1", "alice", "pw")
	srv.expect("client-auth-nt 8 1")
	srv.send(">CLIENT:ESTABLISHED,7", ">CLIENT:ENV,END")

	// BYTECOUNT_CLI is cumulative per connection: in = from the client (upload).
	srv.send(">BYTECOUNT_CLI:7,100,1000", ">BYTECOUNT_CLI:8,10,20", ">BYTECOUNT_CLI:7,150,1600")
	got, online := waitCollect(t, s, func(m map[string]Counters, _ []string) bool { return m["alice"].Down == 1620 })
	if got["alice"].Up != 160 || !slices.Equal(online, []string{"alice"}) {
		t.Fatalf("alice = %+v online %v; want up 160 down 1620, online", got["alice"], online)
	}

	// The final count arrives with the disconnect; only the unseen remainder is added.
	srv.send(">CLIENT:DISCONNECT,7", ">CLIENT:ENV,bytes_received=170", ">CLIENT:ENV,bytes_sent=1700", ">CLIENT:ENV,END")
	got, _ = waitCollect(t, s, func(m map[string]Counters, _ []string) bool { return m["alice"].Down == 100 })
	if got["alice"].Up != 20 {
		t.Fatalf("disconnect remainder = %+v, want up 20 down 100", got["alice"])
	}
}

func TestSessionKicksRevokedUsersAndCapsConnections(t *testing.T) {
	s, srv := newSessionPair(t, map[string]User{"alice": {Password: "pw"}, "bob": {Password: "pw", MaxConns: 1}})
	srv.connect("1", "1", "alice", "pw")
	srv.expect("client-auth-nt 1 1")
	srv.connect("2", "1", "bob", "pw")
	srv.expect("client-auth-nt 2 1")
	srv.connect("3", "1", "bob", "pw")
	srv.expect(`client-deny 3 1 "too many connections"`)

	s.SetUsers(map[string]User{"bob": {Password: "pw", MaxConns: 1}})
	srv.expect("client-kill 1")
	srv.connect("4", "1", "alice", "pw")
	srv.expect(`client-deny 4 1 "invalid credentials"`)

	// A password change also ends the sessions opened with the old one.
	s.SetUsers(map[string]User{"bob": {Password: "new", MaxConns: 1}})
	srv.expect("client-kill 2")
}
