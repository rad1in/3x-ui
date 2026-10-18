package sockkill

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func dialPair(t *testing.T) (client net.Conn, port int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		accepted <- c
	}()
	client, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	server := <-accepted
	t.Cleanup(func() { _ = server.Close() })
	return client, ln.Addr().(*net.TCPAddr).Port
}

func TestKillClosesEstablishedConnectionFromListedIP(t *testing.T) {
	client, port := dialPair(t)
	killed, err := Kill(context.Background(), port, []string{"127.0.0.1"})
	if errors.Is(err, ErrUnsupported) {
		t.Skipf("socket destroy unavailable here: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if killed != 1 {
		t.Fatalf("killed = %d, want 1", killed)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("client read succeeded after its socket was destroyed")
	} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatalf("client still connected after kill: %v", err)
	}
}

func TestKillLeavesOtherPeersAlone(t *testing.T) {
	client, port := dialPair(t)
	killed, err := Kill(context.Background(), port, []string{"192.0.2.55", "not-an-ip"})
	if errors.Is(err, ErrUnsupported) {
		t.Skipf("socket destroy unavailable here: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if killed != 0 {
		t.Fatalf("killed = %d, want 0 for an unrelated peer", killed)
	}
	_ = client.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, err = client.Read(make([]byte, 1))
	if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("connection of an unlisted peer was disturbed: %v", err)
	}
}
