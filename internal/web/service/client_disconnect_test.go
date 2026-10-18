package service

import (
	"net"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/sockkill"
)

func attachClient(t *testing.T, ib *model.Inbound, email string) {
	t.Helper()
	var rec model.ClientRecord
	if err := database.GetDB().Where("email = ?", email).First(&rec).Error; err != nil {
		t.Fatalf("client %s: %v", email, err)
	}
	if err := database.GetDB().Create(&model.ClientInbound{ClientId: rec.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestDisconnectClientAsksEachNodeOnceAndReportsUnsupportedLocalInbound(t *testing.T) {
	setupBulkDB(t)
	nodeID, fake := setupNodeRuntime(t)
	clients := []model.Client{{ID: "11111111-1111-1111-1111-111111111111", Email: "d@x", SubID: "sd", Enable: true}}
	ibA := nodeInbound(t, nodeID, 31001, clients)
	ibB := nodeInbound(t, nodeID, 31002, nil)
	attachClient(t, ibB, "d@x")
	hy := mkInbound(t, 31003, model.Hysteria, `{"clients":[]}`)
	attachClient(t, hy, "d@x")

	res, err := (&ClientService{}).DisconnectClient(&InboundService{}, "d@x")
	if err != nil {
		t.Fatalf("DisconnectClient: %v", err)
	}
	if got := fake.disconnect.Load(); got != 1 {
		t.Fatalf("node asked %d times for two inbounds on one node, want 1", got)
	}
	if res.Disconnected != 3 {
		t.Fatalf("disconnected = %d, want the node's 3", res.Disconnected)
	}
	if len(res.Unsupported) != 1 || res.Unsupported[0] != hy.Tag {
		t.Fatalf("unsupported = %v, want [%s]", res.Unsupported, hy.Tag)
	}
	_ = ibA
}

func TestDisconnectClientDestroysLiveSocketOfRecordedIP(t *testing.T) {
	setupBulkDB(t)
	useTestRuntimeManager(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		if c, _ := ln.Accept(); c != nil {
			defer c.Close()
			time.Sleep(5 * time.Second)
		}
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	clients := []model.Client{{ID: "22222222-2222-2222-2222-222222222222", Email: "live@x", SubID: "sl", Enable: true}}
	ib := mkInbound(t, port, model.VLESS, clientsSettings(t, clients))
	if err := (&ClientService{}).SyncInbound(nil, ib.Id, clients); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.InboundClientIps{ClientEmail: "live@x", Ips: `[{"ip":"127.0.0.1","timestamp":1}]`}).Error; err != nil {
		t.Fatal(err)
	}

	res, err := (&ClientService{}).DisconnectClient(&InboundService{}, "live@x")
	if len(res.Failed) > 0 {
		if _, unsupported := res.Failed[ib.Tag]; unsupported {
			t.Skipf("socket destroy unavailable here: %v (%v)", res.Failed, sockkill.ErrUnsupported)
		}
	}
	if err != nil || res.Disconnected != 1 {
		t.Fatalf("DisconnectClient = %+v, %v; want 1 disconnected", res, err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("socket still readable")
	} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatalf("socket survived the disconnect: %v", err)
	}
}

func TestDisconnectClientUnknownEmailIsAnError(t *testing.T) {
	setupBulkDB(t)
	if _, err := (&ClientService{}).DisconnectClient(&InboundService{}, "missing@x"); err == nil {
		t.Fatal("disconnecting an unknown client succeeded")
	}
}
