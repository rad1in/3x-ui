package job

import (
	"net"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/op/go-logging"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	xuilogger "github.com/mhsanaei/3x-ui/v3/internal/logger"
)

func TestPortProbeJobPublishesBlockedThenRestoredForARealPort(t *testing.T) {
	xuilogger.InitLogger(logging.ERROR)
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close() // nothing answers on the port yet

	db := database.GetDB()
	for k, v := range map[string]string{"portProbeEnable": "true", "portProbeHost": "127.0.0.1", "portProbeFailures": "1"} {
		if err := db.Create(&model.Setting{Key: k, Value: v}).Error; err != nil {
			t.Fatalf("set %s: %v", k, err)
		}
	}
	ib := &model.Inbound{Tag: "in-1", Remark: "edge", Port: port, Protocol: model.VLESS, Enable: true, Settings: `{"clients":[]}`}
	if err := db.Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	events := collectNodeEvents(t)
	j := NewPortProbeJob()

	j.Run()
	j.Run() // still down: no second alert

	live, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Skipf("port %d was taken before the probe could be restored: %v", port, err)
	}
	defer func() { _ = live.Close() }()
	go func() {
		for {
			c, err := live.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	j.Run()

	got := events()
	if len(got) != 2 || got[0].Type != eventbus.EventPortBlocked || got[1].Type != eventbus.EventPortRestored || got[0].Source != "edge" {
		t.Fatalf("events = %+v, want port.blocked then port.restored for edge", got)
	}
}
