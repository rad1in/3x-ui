package runtime

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/sockkill"
)

func TestLocalDisconnectClientDestroysLiveXraySocketsOfListedIPs(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, _ := ln.Accept()
		if c != nil {
			defer c.Close()
			time.Sleep(5 * time.Second)
		}
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	l := NewLocal(LocalDeps{APIPort: func() int { return 0 }, SetNeedRestart: func() {}})
	ib := &model.Inbound{Protocol: model.VLESS, Port: port}
	n, err := l.DisconnectClient(context.Background(), ib, "alice", []string{"127.0.0.1"})
	if errors.Is(err, sockkill.ErrUnsupported) {
		t.Skipf("socket destroy unavailable here: %v", err)
	}
	if err != nil || n != 1 {
		t.Fatalf("DisconnectClient = %d, %v; want 1, nil", n, err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("client still readable after disconnect")
	} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatalf("connection survived the disconnect: %v", err)
	}
}

func TestLocalDisconnectClientRefusesProtocolsItCannotKick(t *testing.T) {
	l := NewLocal(LocalDeps{APIPort: func() int { return 0 }, SetNeedRestart: func() {}})
	for _, p := range []model.Protocol{model.Hysteria, model.WireGuard, model.MTProto, model.TUIC, model.AmneziaWG} {
		_, err := l.DisconnectClient(context.Background(), &model.Inbound{Protocol: p, Port: 443}, "alice", []string{"203.0.113.5"})
		if !errors.Is(err, ErrDisconnectUnsupported) {
			t.Errorf("%s: err = %v, want ErrDisconnectUnsupported", p, err)
		}
	}
}
