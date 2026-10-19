package sshtunnel

import (
	"io"
	"net"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func init() { failDelay = 0 }

func hostKey(t *testing.T) string {
	t.Helper()
	k, err := NewHostKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func startTest(t *testing.T, inst Instance) *server {
	t.Helper()
	if inst.HostKey == "" {
		inst.HostKey = hostKey(t)
	}
	inst.Listen = "127.0.0.1"
	s, err := startServer(inst)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.stop)
	return s
}

func dial(t *testing.T, s *server, user, password string) (*ssh.Client, error) {
	t.Helper()
	return ssh.Dial("tcp", s.addr().String(), &ssh.ClientConfig{
		User: user, Auth: []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second,
	})
}

// echoServer returns a loopback TCP echo server and how many connections it accepted.
func echoServer(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	accepted := new(atomic.Int32)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	return ln.Addr().String(), accepted
}

func users() map[string]User {
	return map[string]User{"alice@x": {Password: "alice-pass"}, "bob@x": {Password: "bob-pass", MaxConns: 1}}
}

func TestTunnelCarriesBytesAndMetersThemPerUser(t *testing.T) {
	echo, _ := echoServer(t)
	s := startTest(t, Instance{Id: 1, Tag: "ssh-1", AllowLocal: true, Users: users()})
	c, err := dial(t, s, "alice@x", "alice-pass")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	conn, err := c.Dial("tcp", echo)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(strings.Repeat("x", 5000))
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(conn, make([]byte, len(payload))); err != nil {
		t.Fatal(err)
	}

	// Counters are bumped just after each write returns, so the last bytes may land a moment late.
	var total [2]int64
	var deltas map[string][2]int64
	var online []string
	for deadline := time.Now().Add(2 * time.Second); total != [2]int64{5000, 5000} && time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		deltas, online = s.collect()
		total[0] += deltas["alice@x"][0]
		total[1] += deltas["alice@x"][1]
	}
	if total != [2]int64{5000, 5000} {
		t.Errorf("alice up/down = %v, want [5000 5000]", total)
	}
	if len(deltas) > 1 || len(online) != 1 || online[0] != "alice@x" {
		t.Errorf("deltas = %v, online = %v; want only alice", deltas, online)
	}
	if again, _ := s.collect(); len(again) != 0 {
		t.Errorf("a second collect returned %v; bytes must be booked once", again)
	}
}

func TestLoginsAreCheckedAgainstTheAccounts(t *testing.T) {
	s := startTest(t, Instance{Id: 1, Users: users()})
	for _, tc := range []struct{ name, user, pw string }{
		{"wrong password", "alice@x", "nope"},
		{"unknown account", "mallory", "alice-pass"},
		{"another account's password", "alice@x", "bob-pass"},
		{"empty password", "alice@x", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if c, err := dial(t, s, tc.user, tc.pw); err == nil {
				_ = c.Close()
				t.Fatal("login accepted")
			}
		})
	}
	c, err := dial(t, s, "alice@x", "alice-pass")
	if err != nil {
		t.Fatalf("valid login refused: %v", err)
	}
	_ = c.Close()
}

func TestLocalTargetsAreRefusedUnlessAllowed(t *testing.T) {
	echo, accepted := echoServer(t)
	s := startTest(t, Instance{Id: 1, Users: users()})
	c, err := dial(t, s, "alice@x", "alice-pass")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if conn, err := c.Dial("tcp", echo); err == nil {
		_ = conn.Close()
		t.Fatal("a loopback target was reached through the tunnel")
	} else if !strings.Contains(err.Error(), errTargetNotAllowed.Error()) {
		t.Fatalf("err = %v, want %q", err, errTargetNotAllowed)
	}
	if n := accepted.Load(); n != 0 {
		t.Fatalf("the loopback service saw %d connections", n)
	}
}

func TestAccountChangesEndLiveConnections(t *testing.T) {
	s := startTest(t, Instance{Id: 1, Users: users()})
	alice, err := dial(t, s, "alice@x", "alice-pass")
	if err != nil {
		t.Fatal(err)
	}
	waits := func(c *ssh.Client) chan struct{} {
		ch := make(chan struct{})
		go func() { _ = c.Wait(); close(ch) }()
		return ch
	}
	aliceGone := waits(alice)

	// A change to someone else's account leaves alice connected.
	s.setUsers(map[string]User{"alice@x": {Password: "alice-pass"}, "bob@x": {Password: "new"}})
	select {
	case <-aliceGone:
		t.Fatal("alice was disconnected by a change to bob")
	case <-time.After(200 * time.Millisecond):
	}

	s.setUsers(map[string]User{"alice@x": {Password: "rotated"}})
	select {
	case <-aliceGone:
	case <-time.After(3 * time.Second):
		t.Fatal("alice stayed connected after her password changed")
	}
	if c, err := dial(t, s, "alice@x", "alice-pass"); err == nil {
		_ = c.Close()
		t.Fatal("the old password still logs in")
	}

	bob, err := dial(t, s, "bob@x", "bob-pass")
	if err == nil {
		_ = bob.Close()
		t.Fatal("bob's removed login still works")
	}
}

func TestConnectionLimitRefusesTheNewestLogin(t *testing.T) {
	s := startTest(t, Instance{Id: 1, Users: users()})
	first, err := dial(t, s, "bob@x", "bob-pass")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := dial(t, s, "bob@x", "bob-pass"); err == nil {
		// The handshake completes before the limit check, so the refusal shows as a closed connection.
		defer second.Close()
		done := make(chan struct{})
		go func() { _ = second.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("a second connection stayed open past bob's limit of 1")
		}
	}
	if _, _, err := first.SendRequest("keepalive@openssh.com", true, nil); err != nil {
		t.Fatalf("the first connection was dropped: %v", err)
	}
}

func TestKickEndsOnlyThatUsersConnections(t *testing.T) {
	s := startTest(t, Instance{Id: 1, Users: users()})
	alice, _ := dial(t, s, "alice@x", "alice-pass")
	bob, _ := dial(t, s, "bob@x", "bob-pass")
	defer alice.Close()
	defer bob.Close()
	if got := s.kick("alice@x"); got != 1 {
		t.Fatalf("kick = %d, want 1", got)
	}
	done := make(chan struct{})
	go func() { _ = alice.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("alice is still connected")
	}
	if _, _, err := bob.SendRequest("keepalive@openssh.com", true, nil); err != nil {
		t.Fatalf("bob was dropped: %v", err)
	}
}

func TestSessionHasNoShellAndRefusesCommands(t *testing.T) {
	s := startTest(t, Instance{Id: 1, Users: users()})
	c, err := dial(t, s, "alice@x", "alice-pass")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	exec, err := c.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.CombinedOutput("id"); err == nil {
		t.Fatalf("a command ran: %q", out)
	}

	shell, err := c.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	out, err := shell.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := shell.Shell(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(sessionNotice))
	if _, err := io.ReadFull(out, buf); err != nil || string(buf) != sessionNotice {
		t.Fatalf("shell greeting = %q, %v", buf, err)
	}
}

func TestLocalAddressClassification(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1": true, "10.1.2.3": true, "192.168.0.9": true, "172.16.5.5": true,
		"169.254.169.254": true, "100.64.0.1": true, "0.0.0.0": true, "::1": true,
		"fe80::1": true, "fd00::1": true, "224.0.0.1": true,
		"8.8.8.8": false, "1.1.1.1": false, "2606:4700:4700::1111": false,
	} {
		ip, err := netip.ParseAddr(addr)
		if err != nil {
			t.Fatal(err)
		}
		if got := localAddress(ip); got != want {
			t.Errorf("localAddress(%s) = %v, want %v", addr, got, want)
		}
	}
}
