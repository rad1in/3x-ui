package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func setupHosted(t *testing.T, limits hosted.Limits) string {
	t.Helper()
	setupConflictDB(t)
	dir := t.TempDir()
	t.Setenv(hosted.EnvDir, dir)
	if err := hosted.WriteLimits(dir, limits); err != nil {
		t.Fatal(err)
	}
	return dir
}

func hostedVless(t *testing.T, port int, emails ...string) *model.Inbound {
	t.Helper()
	clients := make([]model.Client, 0, len(emails))
	for i, e := range emails {
		clients = append(clients, model.Client{Email: e, ID: uuidFor(i), Enable: true})
	}
	return &model.Inbound{
		Tag:      "in-" + time.Now().Format("150405.000000000"),
		Enable:   true,
		Listen:   "0.0.0.0",
		Port:     port,
		Protocol: model.VLESS,
		Settings: clientsSettings(t, clients),
	}
}

func uuidFor(i int) string {
	return []string{
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444",
	}[i]
}

func TestHostedPanelCapsInboundsAndPorts(t *testing.T) {
	setupHosted(t, hosted.Limits{MaxInbounds: 1, PortMin: 20000, PortMax: 20009})
	svc := &InboundService{}

	if _, _, err := svc.AddInbound(hostedVless(t, 30000)); !errors.Is(err, ErrHostedPortRange) {
		t.Fatalf("port outside the owner's range: err = %v, want ErrHostedPortRange", err)
	}
	first, _, err := svc.AddInbound(hostedVless(t, 20000))
	if err != nil {
		t.Fatalf("first inbound in range: %v", err)
	}
	if _, _, err := svc.AddInbound(hostedVless(t, 20001)); !errors.Is(err, ErrHostedInboundLimit) {
		t.Fatalf("second inbound past MaxInbounds=1: err = %v, want ErrHostedInboundLimit", err)
	}

	moved := *first
	moved.Port = 443
	if _, _, err := svc.UpdateInbound(&moved); !errors.Is(err, ErrHostedPortRange) {
		t.Fatalf("moving an inbound out of range: err = %v, want ErrHostedPortRange", err)
	}
}

func TestHostedPanelCapsClientsAcrossPaths(t *testing.T) {
	setupHosted(t, hosted.Limits{MaxClients: 2})
	inbounds := &InboundService{}

	if _, _, err := inbounds.AddInbound(hostedVless(t, 20000, "a@x", "b@x", "c@x")); !errors.Is(err, ErrHostedClientLimit) {
		t.Fatalf("inbound carrying 3 clients past MaxClients=2: err = %v", err)
	}
	created, _, err := inbounds.AddInbound(hostedVless(t, 20000, "a@x", "b@x"))
	if err != nil {
		t.Fatalf("inbound at the limit: %v", err)
	}
	payload := &ClientCreatePayload{Client: model.Client{Email: "c@x", ID: uuidFor(2), Enable: true}, InboundIds: []int{created.Id}}
	if _, err := (&ClientService{}).Create(inbounds, payload); !errors.Is(err, ErrHostedClientLimit) {
		t.Fatalf("client add past MaxClients: err = %v, want ErrHostedClientLimit", err)
	}
}

func TestHostedUsageIsMeteredBlocksAndResetsByEpoch(t *testing.T) {
	dir := setupHosted(t, hosted.Limits{TrafficBytes: 1000})
	h := &HostedService{}

	traffic := []*xray.Traffic{
		{IsInbound: true, Tag: "in-1", Up: 300, Down: 300},
		{IsInbound: true, Tag: "api", Up: 5000, Down: 5000},
		{IsOutbound: true, Tag: "direct", Up: 9000, Down: 9000},
	}
	if blocked := h.AddUsage(traffic); blocked != "" {
		t.Fatalf("600 of 1000 bytes: blocked = %q", blocked)
	}
	if blocked := h.AddUsage(traffic); blocked != hosted.BlockedTraffic {
		t.Fatalf("1200 of 1000 bytes: blocked = %q, want traffic", blocked)
	}
	view, err := h.View()
	if err != nil || view.UsedBytes != 1200 {
		t.Fatalf("used = %d, %v; only user inbound traffic counts", view.UsedBytes, err)
	}

	// The owner renewing the panel bumps the epoch; usage restarts from zero.
	if err := hosted.WriteLimits(dir, hosted.Limits{TrafficBytes: 1000, UsageEpoch: 1}); err != nil {
		t.Fatal(err)
	}
	if blocked := h.AddUsage(traffic[:1]); blocked != "" {
		t.Fatalf("after reset: blocked = %q", blocked)
	}
	if view, _ := h.View(); view.UsedBytes != 600 {
		t.Fatalf("after reset used = %d, want 600", view.UsedBytes)
	}
}

func TestHostedPanelBlocksWhenExpired(t *testing.T) {
	setupHosted(t, hosted.Limits{ExpiresAt: time.Now().Add(-time.Minute).UnixMilli()})
	if got := (&HostedService{}).Blocked(); got != hosted.BlockedExpired {
		t.Fatalf("Blocked() = %q, want expired", got)
	}
}

func TestHostedPanelDropsTemplateInbounds(t *testing.T) {
	setupHosted(t, hosted.Limits{})
	s := &SettingService{}
	tmpl, err := s.GetXrayConfigTemplate()
	if err != nil {
		t.Fatal(err)
	}
	extra := `{"tag":"sneaky","port":9999,"protocol":"socks","settings":{}}`
	tmpl = replaceOnce(t, tmpl, `"inbounds": [`, `"inbounds": [`+extra+`,`)
	if err := s.saveSetting("xrayTemplateConfig", tmpl); err != nil {
		t.Fatal(err)
	}
	cfg, err := (&XrayService{}).GetXrayConfig()
	if err != nil {
		t.Fatal(err)
	}
	for _, ib := range cfg.InboundConfigs {
		if ib.Tag == "sneaky" {
			t.Fatal("a hosted panel's template inbound would be unmetered and outside its caps")
		}
	}
}

func replaceOnce(t *testing.T, s, old, repl string) string {
	t.Helper()
	i := strings.Index(s, old)
	if i < 0 {
		t.Fatalf("%q not found in template", old)
	}
	return s[:i] + repl + s[i+len(old):]
}

// The owner hands out the panel's address; the hosted admin must not move it
// onto a port or path that collides with a sibling panel or breaks the link.
func TestHostedPanelKeepsOwnerAssignedAddress(t *testing.T) {
	setupHosted(t, hosted.Limits{})
	s := &SettingService{}
	before, err := s.GetAllSetting()
	if err != nil {
		t.Fatal(err)
	}
	edited := *before
	edited.WebPort, edited.WebBasePath, edited.WebListen = 2053, "/mine/", "10.0.0.1"
	edited.SubPort, edited.SubListen = 2096, "10.0.0.1"
	edited.WebDomain = "panel.example.com"
	edited.PageSize = 77
	if err := s.UpdateAllSetting(&edited, SecretClears{}); err != nil {
		t.Fatal(err)
	}
	after, err := s.GetAllSetting()
	if err != nil {
		t.Fatal(err)
	}
	if after.WebPort != before.WebPort || after.WebBasePath != before.WebBasePath || after.WebListen != before.WebListen ||
		after.SubPort != before.SubPort || after.SubListen != before.SubListen || after.WebDomain != before.WebDomain {
		t.Errorf("hosted panel moved its address: web %s:%d%s sub %s:%d",
			after.WebListen, after.WebPort, after.WebBasePath, after.SubListen, after.SubPort)
	}
	if after.PageSize != 77 {
		t.Errorf("ordinary settings must still save, pageSize = %d", after.PageSize)
	}
}

func TestHostedStatusReportsWhetherThePanelServesTLS(t *testing.T) {
	dir := setupHosted(t, hosted.Limits{})
	h := &HostedService{}
	h.Reconcile(&XrayService{})
	if st, err := hosted.ReadStatus(dir); err != nil || st.WebTLS {
		t.Fatalf("no certificate: status = %+v, %v; want webTls false", st, err)
	}
	s := &SettingService{}
	if err := s.setString("webCertFile", "/etc/ssl/panel.crt"); err != nil {
		t.Fatal(err)
	}
	if err := s.setString("webKeyFile", "/etc/ssl/panel.key"); err != nil {
		t.Fatal(err)
	}
	h.Reconcile(&XrayService{})
	if st, _ := hosted.ReadStatus(dir); !st.WebTLS {
		t.Fatal("with a certificate the master would link the panel over plain http")
	}
}

// A hosted panel belongs to someone else; the server's own load and traffic
// are the owner's business and must not reach its dashboard, bots or API.
func TestHostedPanelStatusCarriesNoHostMetrics(t *testing.T) {
	setupConflictDB(t)
	server := &ServerService{}
	if own := server.GetStatus(nil); own.Mem.Total == 0 || own.Disk.Total == 0 {
		t.Fatalf("ordinary panel status lacks host metrics: mem %d disk %d", own.Mem.Total, own.Disk.Total)
	}

	t.Setenv(hosted.EnvDir, t.TempDir())
	st := server.GetStatus(nil)
	host := []struct {
		name string
		zero bool
	}{
		{"cpu", st.Cpu == 0 && st.CpuCores == 0 && st.LogicalPro == 0 && st.CpuSpeedMhz == 0},
		{"memory", st.Mem.Total == 0 && st.Mem.Current == 0 && st.Swap.Total == 0},
		{"disk", st.Disk.Total == 0 && st.DiskTraffic.Read == 0 && st.DiskTraffic.Write == 0},
		{"network", st.NetTraffic.Sent == 0 && st.NetTraffic.Recv == 0 && st.TcpCount == 0 && st.UdpCount == 0},
		{"uptime and load", st.Uptime == 0 && len(st.Loads) == 0},
	}
	for _, h := range host {
		if !h.zero {
			t.Errorf("hosted panel status still reports the server's %s: %+v", h.name, st)
		}
	}
	if st.PanelVersion == "" {
		t.Error("the panel's own fields must still be reported")
	}
}

// MTProto not routed through Xray reports its bytes to AddTraffic only; the
// hosted meter must see them or that traffic escapes the owner's quota.
func TestHostedUsageMetersTrafficFromEveryProtocol(t *testing.T) {
	setupHosted(t, hosted.Limits{TrafficBytes: 1000})
	direct := []*xray.Traffic{{IsInbound: true, Tag: "mt-direct", Up: 400, Down: 300}}
	if _, _, err := (&InboundService{}).AddTraffic(direct, nil); err != nil {
		t.Fatal(err)
	}
	if view, _ := (&HostedService{}).View(); view.UsedBytes != 700 {
		t.Fatalf("used = %d after 700 bytes of sidecar traffic", view.UsedBytes)
	}
}

func TestSuspendedHostedPanelStopsItsSidecars(t *testing.T) {
	setupHosted(t, hosted.Limits{ExpiresAt: time.Now().Add(-time.Minute).UnixMilli()})
	seedInboundConflict(t, "mt-hosted", "", 30001, model.MTProto, "",
		`{"clients":[{"email":"alice","secret":"`+mtprotoTestSecretA+`","enable":true}]}`)
	seedClientTraffic(t, loadInboundByTag(t, "mt-hosted").Id, "alice", true)

	instances, err := (&InboundService{}).DesiredMtprotoInstances()
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 0 {
		t.Fatalf("an expired hosted panel still serves %d mtproto sidecars", len(instances))
	}
}
