//go:build linux

package sstp

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The real sstp-client on the other end: it connects, upgrades, runs the SSTP
// handshake and converts between SSTP and the HDLC a pppd would speak, which
// this test plays by hand. Skipped where sstpc is not installed.
func TestRealSstpcClientTalksToTheServer(t *testing.T) {
	bin, err := exec.LookPath("sstpc")
	if err != nil {
		t.Skip("sstpc (the sstp-client package) is not installed")
	}
	s, _ := startTest(t, testInstance(t))
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, []byte(pki(t).CA), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, port, _ := strings.Cut(s.addr().String(), ":")
	cmd := exec.CommandContext(ctx, bin, "--nolaunchpppd", "--ca-cert", ca, "--log-level", "4", "--log-stderr", "127.0.0.1:"+port)
	// sstpc uses one descriptor both ways, as it would a pppd pty.
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	theirs := os.NewFile(uintptr(fds[1]), "sstpc-pty")
	mine, err := net.FileConn(os.NewFile(uintptr(fds[0]), "pppd-side"))
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdin, cmd.Stdout = theirs, theirs
	var stdin io.Writer = mine
	var stdout io.Reader = mine
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = theirs.Close()
	t.Cleanup(func() { cancel(); _ = mine.Close(); _ = cmd.Wait() })
	frames := make(chan []byte, 8)
	go func() {
		var dec hdlcDecoder
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			dec.feed(buf[:n], func(f []byte) { frames <- f })
			if err != nil {
				close(frames)
				return
			}
		}
	}()
	next := func(what string) []byte {
		t.Helper()
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatalf("sstpc ended before %s; its log:\n%s", what, stderr.String())
			}
			return f
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for %s; sstpc log:\n%s", what, stderr.String())
		}
		return nil
	}

	send := func(frame []byte) {
		t.Helper()
		if _, err := io.WriteString(stdin, string(hdlcEncode(frame))); err != nil {
			t.Fatalf("writing to sstpc: %v", err)
		}
	}
	send(chapResponseFrame("alice@x"))
	proto, payload, _ := pppProtocol(next("the CHAP Success"))
	if code, _, ok := chapPacket(payload); proto != protoCHAP || !ok || code != chapSuccess {
		t.Fatalf("answered with protocol %#04x payload %x", proto, payload)
	}

	send(ipFrame(1200))
	proto, payload, _ = pppProtocol(next("the echoed IP packet"))
	if proto != protoIPv4 || len(payload) != 1200 {
		t.Fatalf("echo = protocol %#04x, %d bytes", proto, len(payload))
	}
	var up, down int64
	waitUntil(t, "the client's bytes to be counted", func() bool {
		deltas, _ := s.collect()
		up += deltas["alice@x"][0]
		down += deltas["alice@x"][1]
		return up == 1200 && down == 1200
	})
	if _, online := s.collect(); len(online) != 1 || online[0] != "alice@x" {
		t.Fatalf("online = %v (log: %s)", online, strconv.Quote(stderr.String()))
	}
}
