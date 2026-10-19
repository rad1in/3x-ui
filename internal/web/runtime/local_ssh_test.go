package runtime

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/sshtunnel"
)

func sshInbound(t *testing.T, id, port int, key string, clients ...map[string]any) *model.Inbound {
	t.Helper()
	settings, err := json.Marshal(map[string]any{"hostKey": key, "clients": clients})
	if err != nil {
		t.Fatal(err)
	}
	return &model.Inbound{Id: id, Tag: "ssh-test", Enable: true, Listen: "127.0.0.1", Port: port, Protocol: model.SSH, Settings: string(settings)}
}

func sshLogin(addr, user, pw string) (*ssh.Client, error) {
	return ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User: user, Auth: []ssh.AuthMethod{ssh.Password(pw)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 3 * time.Second,
	})
}

func TestLocalServesAnSSHInboundAndFollowsItsClients(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	key, err := sshtunnel.NewHostKey()
	if err != nil {
		t.Fatal(err)
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	l := NewLocal(LocalDeps{APIPort: func() int { return 0 }, SetNeedRestart: func() {}})
	ctx := context.Background()
	t.Cleanup(sshtunnel.GetManager().StopAll)

	alice := map[string]any{"email": "alice", "password": "pa", "enable": true}
	bob := map[string]any{"email": "bob", "password": "pb", "enable": true}
	ib := sshInbound(t, 91, port, key, alice)
	if err := l.AddInbound(ctx, ib); err != nil {
		t.Fatal(err)
	}
	c, err := sshLogin(addr, "alice", "pa")
	if err != nil {
		t.Fatalf("alice cannot log in after AddInbound: %v", err)
	}
	defer c.Close()

	withBob := sshInbound(t, 91, port, key, alice, bob)
	if err := l.UpdateInbound(ctx, ib, withBob); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.SendRequest("keepalive@openssh.com", true, nil); err != nil {
		t.Fatalf("a client edit dropped alice: %v", err)
	}
	if b, err := sshLogin(addr, "bob", "pb"); err != nil {
		t.Fatalf("bob cannot log in after the edit: %v", err)
	} else {
		_ = b.Close()
	}

	if n, err := l.DisconnectClient(ctx, withBob, "alice", nil); err != nil || n != 1 {
		t.Fatalf("DisconnectClient = %d, %v; want 1, nil", n, err)
	}

	// With nobody left to serve, the listener must go away rather than keep the old logins.
	empty := sshInbound(t, 91, port, key)
	if err := l.UpdateInbound(ctx, withBob, empty); err != nil {
		t.Fatal(err)
	}
	if running, _ := sshtunnel.GetManager().Status(91); running {
		t.Fatal("the server kept running with no clients")
	}
	if c, err := sshLogin(addr, "bob", "pb"); err == nil {
		_ = c.Close()
		t.Fatal("a removed client can still log in")
	}
}
