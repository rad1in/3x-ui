package service

import (
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

type eventLog struct {
	mu     sync.Mutex
	events []eventbus.Event
}

// captureClientEvents puts a real bus in place and records its client events.
func captureClientEvents(t *testing.T) *eventLog {
	t.Helper()
	bus := eventbus.New(0)
	log := &eventLog{}
	bus.Subscribe("test", func(e eventbus.Event) {
		log.mu.Lock()
		log.events = append(log.events, e)
		log.mu.Unlock()
	})
	prev := eventBus
	SetEventBus(bus)
	t.Cleanup(func() { bus.Stop(); SetEventBus(prev) })
	return log
}

// wait returns "type:email" for every event once at least n arrived.
func (l *eventLog) wait(t *testing.T, n int) []string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		l.mu.Lock()
		got := make([]string, 0, len(l.events))
		for _, e := range l.events {
			got = append(got, string(e.Type)+":"+e.Source)
		}
		l.mu.Unlock()
		if len(got) >= n || time.Now().After(deadline) {
			sort.Strings(got)
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (l *eventLog) data(t *testing.T, typ eventbus.EventType, email string) *eventbus.ClientEventData {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.events {
		if e.Type == typ && e.Source == email {
			d, _ := e.Data.(*eventbus.ClientEventData)
			return d
		}
	}
	t.Fatalf("no %s event for %s", typ, email)
	return nil
}

func seedVlessForEvents(t *testing.T) (*InboundService, *ClientService, int) {
	t.Helper()
	setupConflictDB(t)
	startSerializedWriter(t)
	useTestRuntimeManager(t).SetLocalRuntimeOverride(&xraylessLocal{})
	seedInboundConflict(t, "vless-ev", "", 4431, model.VLESS, `{"network":"tcp"}`, `{"clients":[],"decryption":"none"}`)
	return &InboundService{}, &ClientService{}, loadInboundByTag(t, "vless-ev").Id
}

func TestClientCRUDPublishesLifecycleEvents(t *testing.T) {
	inbounds, clients, id := seedVlessForEvents(t)
	log := captureClientEvents(t)

	if _, err := clients.Create(inbounds, &ClientCreatePayload{
		Client: model.Client{Email: "ev-a", SubID: "sub-a", TotalGB: 5 << 30, Enable: true}, InboundIds: []int{id},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := clients.Create(inbounds, &ClientCreatePayload{Client: model.Client{Email: "ev-b", Enable: true}, InboundIds: []int{id}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := clients.BulkSetEnable(inbounds, []string{"ev-a"}, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := clients.BulkSetEnable(inbounds, []string{"ev-a"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := clients.ResetTrafficByEmail(inbounds, "ev-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := clients.DeleteByEmail(inbounds, "ev-b", false); err != nil {
		t.Fatal(err)
	}

	got := log.wait(t, 6)
	want := []string{"client.created:ev-a", "client.created:ev-b", "client.deleted:ev-b",
		"client.disabled:ev-a", "client.enabled:ev-a", "client.traffic_reset:ev-a"}
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}
	if d := log.data(t, eventbus.EventClientCreated, "ev-a"); d.SubID != "sub-a" || d.Total != 5<<30 {
		t.Fatalf("created data = %+v", d)
	}
}

// The traffic tick that cuts clients off tells why: quota used up, or time out.
func TestTrafficTickPublishesDepletedAndExpired(t *testing.T) {
	inbounds, clients, id := seedVlessForEvents(t)
	for _, c := range []model.Client{
		{Email: "ev-quota", TotalGB: 1000, Enable: true},
		{Email: "ev-time", ExpiryTime: time.Now().Add(time.Hour).UnixMilli(), Enable: true},
	} {
		if _, err := clients.Create(inbounds, &ClientCreatePayload{Client: c, InboundIds: []int{id}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", "ev-time").
		Update("expiry_time", time.Now().Add(-time.Minute).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	log := captureClientEvents(t)
	if _, _, err := inbounds.AddTraffic(nil, []*xray.ClientTraffic{{Email: "ev-quota", Up: 600, Down: 600}}); err != nil {
		t.Fatal(err)
	}
	got := log.wait(t, 2)
	if len(got) != 2 || got[0] != "client.depleted:ev-quota" || got[1] != "client.expired:ev-time" {
		t.Fatalf("events = %v", got)
	}
	if d := log.data(t, eventbus.EventClientDepleted, "ev-quota"); d.Up+d.Down < 1000 || d.Total != 1000 {
		t.Fatalf("depleted data = %+v; want the usage that crossed the quota", d)
	}
}

// Every reset path reports each client once, however many inbounds it is on.
func TestTrafficResetsPublishOncePerClient(t *testing.T) {
	inbounds, clients, id := seedVlessForEvents(t)
	seedInboundConflict(t, "vless-ev2", "", 4432, model.VLESS, `{"network":"tcp"}`, `{"clients":[],"decryption":"none"}`)
	id2 := loadInboundByTag(t, "vless-ev2").Id
	for _, email := range []string{"rs-a", "rs-b"} {
		if _, err := clients.Create(inbounds, &ClientCreatePayload{
			Client: model.Client{Email: email, Enable: true}, InboundIds: []int{id, id2},
		}); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name  string
		reset func() error
		want  []string
	}{
		{"single", func() error { _, err := clients.ResetTrafficByEmail(inbounds, "rs-a"); return err },
			[]string{"client.traffic_reset:rs-a"}},
		{"bulk", func() error {
			_, err := clients.BulkResetTraffic(inbounds, []string{"rs-a", "rs-b", "nobody"})
			return err
		},
			[]string{"client.traffic_reset:rs-a", "client.traffic_reset:rs-b"}},
		{"inbound", func() error { return clients.ResetAllClientTraffics(inbounds, id2) },
			[]string{"client.traffic_reset:rs-a", "client.traffic_reset:rs-b"}},
		{"all", func() error { _, err := clients.ResetAllTraffics(); return err },
			[]string{"client.traffic_reset:rs-a", "client.traffic_reset:rs-b"}},
		{"by email", func() error { return inbounds.ResetClientTrafficByEmail("rs-b") },
			[]string{"client.traffic_reset:rs-b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log := captureClientEvents(t)
			if err := tc.reset(); err != nil {
				t.Fatal(err)
			}
			log.wait(t, len(tc.want))
			time.Sleep(150 * time.Millisecond) // a duplicate would land by now
			got := log.wait(t, len(tc.want))
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("events = %v, want %v", got, tc.want)
			}
		})
	}
}
