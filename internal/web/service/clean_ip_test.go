package service

import (
	"context"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func addrStrings(addrs []netip.Addr) []string {
	out := make([]string, len(addrs))
	for i, a := range addrs {
		out[i] = a.String()
	}
	return out
}

func TestExpandCleanIPTargets(t *testing.T) {
	cases := []struct {
		name string
		spec string
		want []string
	}{
		{"single and list", "1.1.1.1, 1.0.0.1\n1.1.1.1", []string{"1.1.1.1", "1.0.0.1"}},
		{"cidr", "104.16.0.0/30", []string{"104.16.0.0", "104.16.0.1", "104.16.0.2", "104.16.0.3"}},
		{"range", "104.16.0.254-104.16.1.1", []string{"104.16.0.254", "104.16.0.255", "104.16.1.0", "104.16.1.1"}},
		{"ipv6 single", "2606:4700::1111", []string{"2606:4700::1111"}},
		{"ipv6 cidr crosses a byte", "2606:4700::ffff/127", []string{"2606:4700::fffe", "2606:4700::ffff"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expandCleanIPTargets(tc.spec, 100)
			if err != nil {
				t.Fatalf("expand: %v", err)
			}
			if !slices.Equal(addrStrings(got), tc.want) {
				t.Fatalf("got %v, want %v", addrStrings(got), tc.want)
			}
		})
	}
}

func TestExpandCleanIPTargetsRejects(t *testing.T) {
	for _, spec := range []string{"", "not-an-ip", "1.1.1.1/33", "1.1.1.9-1.1.1.1", "1.1.1.1-2606:4700::1", "example.com"} {
		if _, err := expandCleanIPTargets(spec, 100); err == nil {
			t.Errorf("expand(%q) succeeded, want an error", spec)
		}
	}
}

func TestExpandCleanIPTargetsSamplesLargeRanges(t *testing.T) {
	got, err := expandCleanIPTargets("104.16.0.0/16", 64)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(got) != 64 {
		t.Fatalf("len = %d, want 64 sampled addresses", len(got))
	}
	prefix := netip.MustParsePrefix("104.16.0.0/16")
	seen := map[netip.Addr]bool{}
	for _, a := range got {
		if !prefix.Contains(a) {
			t.Fatalf("%s outside the range", a)
		}
		if seen[a] {
			t.Fatalf("%s sampled twice", a)
		}
		seen[a] = true
	}
}

func TestClassifyCleanIPError(t *testing.T) {
	refused, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := refused.Addr().String()
	_ = refused.Close()
	_, refusedErr := (&net.Dialer{}).DialContext(context.Background(), "tcp", addr)
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{"refused", refusedErr, "refused"},
		{"timeout", ctx.Err(), "timeout"},
		{"hostname", x509.HostnameError{Host: "a.test"}, "certificate"},
		{"unknown authority", x509.UnknownAuthorityError{}, "certificate"},
	}
	for _, tc := range cases {
		if got := classifyCleanIPError(tc.err); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func openListener(t *testing.T) (port int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func testScanner() *cleanIPScanner {
	s := newCleanIPScanner()
	s.blocked = func(net.IP) bool { return false }
	return s
}

func TestCleanIPScanReportsReachableAndUnreachable(t *testing.T) {
	setupBulkDB(t)
	port := openListener(t)
	s := testScanner()

	err := s.run(CleanIPScanRequest{Targets: "127.0.0.1, 127.0.0.2", Port: port, Samples: 2, TimeoutMs: 500})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	rows, err := s.Results()
	if err != nil {
		t.Fatalf("Results: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want 2", rows)
	}
	if rows[0].IP != "127.0.0.1" || !rows[0].Ok || rows[0].LossPct != 0 {
		t.Errorf("reachable row first = %+v, want ok 127.0.0.1 with no loss", rows[0])
	}
	if rows[1].IP != "127.0.0.2" || rows[1].Ok || rows[1].Reason != "refused" || rows[1].LossPct != 100 {
		t.Errorf("unreachable row second = %+v, want refused 127.0.0.2 with 100%% loss", rows[1])
	}
}

func TestCleanIPScanReplacesPreviousResults(t *testing.T) {
	setupBulkDB(t)
	port := openListener(t)
	s := testScanner()
	for _, ip := range []string{"127.0.0.1", "127.0.0.2"} {
		if err := s.run(CleanIPScanRequest{Targets: ip, Port: port, Samples: 1, TimeoutMs: 500}); err != nil {
			t.Fatalf("run %s: %v", ip, err)
		}
	}
	rows, _ := s.Results()
	if len(rows) != 1 || rows[0].IP != "127.0.0.2" {
		t.Fatalf("rows = %+v, want only the second scan's 127.0.0.2", rows)
	}
}

func TestCleanIPScanRefusesInternalAddresses(t *testing.T) {
	setupBulkDB(t)
	s := newCleanIPScanner()
	dialed := false
	s.dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialed = true
		return nil, net.ErrClosed
	}
	if err := s.run(CleanIPScanRequest{Targets: "127.0.0.1, 10.1.2.3", Port: 443, Samples: 1, TimeoutMs: 200}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if dialed {
		t.Fatal("scanner dialed a loopback/private address")
	}
	rows, _ := s.Results()
	for _, r := range rows {
		if r.Ok || r.Reason != "blocked" {
			t.Errorf("row %+v, want blocked", r)
		}
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want both blocked targets listed", len(rows))
	}
}

func TestCleanIPScanVerifiesCertificateAgainstSNI(t *testing.T) {
	setupBulkDB(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())

	s := testScanner()
	s.roots = pool

	// httptest's certificate is issued for example.com.
	if err := s.run(CleanIPScanRequest{Targets: "127.0.0.1", Port: port, SNI: "example.com", Samples: 1, TimeoutMs: 2000}); err != nil {
		t.Fatalf("run: %v", err)
	}
	rows, _ := s.Results()
	if len(rows) != 1 || !rows[0].Ok {
		t.Fatalf("matching SNI: rows = %+v, want ok", rows)
	}

	if err := s.run(CleanIPScanRequest{Targets: "127.0.0.1", Port: port, SNI: "other.test", Samples: 1, TimeoutMs: 2000}); err != nil {
		t.Fatalf("run: %v", err)
	}
	rows, _ = s.Results()
	if len(rows) != 1 || rows[0].Ok || rows[0].Reason != "certificate" {
		t.Fatalf("wrong SNI: rows = %+v, want a certificate failure", rows)
	}
}

func TestCleanIPStartRejectsConcurrentScan(t *testing.T) {
	setupBulkDB(t)
	s := testScanner()
	release := make(chan struct{})
	s.dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
		<-release
		return nil, net.ErrClosed
	}
	if err := s.Start(CleanIPScanRequest{Targets: "1.1.1.1", Port: 443, Samples: 1, TimeoutMs: 200}); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if err := s.Start(CleanIPScanRequest{Targets: "1.0.0.1", Port: 443}); err == nil {
		t.Fatal("second Start succeeded while a scan is running")
	}
	if !s.Status().Running {
		t.Fatal("Status().Running = false during a scan")
	}
	close(release)
	deadline := time.Now().Add(3 * time.Second)
	for s.Status().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if st := s.Status(); st.Running || st.Done != 1 || st.Total != 1 {
		t.Fatalf("Status after scan = %+v, want finished 1/1", st)
	}
}

func TestApplyCleanIP(t *testing.T) {
	setupBulkDB(t)
	svc := &HostService{}
	ib := mkInbound(t, 443, model.VLESS, `{"clients":[]}`)
	group := mkHost(t, svc, ib.Id, "cdn", 0)

	if _, err := svc.ApplyCleanIP("104.16.1.2", group.GroupId, 0, "replace"); err != nil {
		t.Fatalf("replace: %v", err)
	}
	got, _ := svc.GetHostGroup(group.GroupId)
	if !slices.Equal(got.Hosts, []string{"104.16.1.2:8443"}) {
		t.Fatalf("replace: hosts = %v, want only the new address on the group's port", got.Hosts)
	}

	if _, err := svc.ApplyCleanIP("104.16.9.9", group.GroupId, 0, "add"); err != nil {
		t.Fatalf("add: %v", err)
	}
	got, _ = svc.GetHostGroup(group.GroupId)
	if !slices.Equal(got.Hosts, []string{"104.16.1.2:8443", "104.16.9.9:8443"}) {
		t.Fatalf("add: hosts = %v", got.Hosts)
	}
	if got.Remark != "cdn" || got.Port != 8443 {
		t.Fatalf("apply changed unrelated fields: %+v", got)
	}

	created, err := svc.ApplyCleanIP("2606:4700::1", "", ib.Id, "replace")
	if err != nil {
		t.Fatalf("new host on inbound: %v", err)
	}
	if len(created) != 1 || created[0].Address != "2606:4700::1" || created[0].InboundId != ib.Id {
		t.Fatalf("new host rows = %+v", created)
	}
	var count int64
	database.GetDB().Model(&model.Host{}).Where("inbound_id = ?", ib.Id).Count(&count)
	if count != 3 {
		t.Fatalf("host rows on inbound = %d, want 3 (2 in the group + 1 new)", count)
	}
}

func TestApplyCleanIPRejectsBadInput(t *testing.T) {
	setupBulkDB(t)
	svc := &HostService{}
	ib := mkInbound(t, 443, model.VLESS, `{"clients":[]}`)
	group := mkHost(t, svc, ib.Id, "cdn", 0)
	for name, c := range map[string]struct {
		ip, group, mode string
		inbound         int
	}{
		"not an ip":       {"cdn.example.com", group.GroupId, "replace", 0},
		"no target":       {"104.16.1.2", "", "replace", 0},
		"unknown group":   {"104.16.1.2", "nope", "replace", 0},
		"unknown inbound": {"104.16.1.2", "", "replace", 9999},
		"bad mode":        {"104.16.1.2", group.GroupId, "wipe", 0},
	} {
		if _, err := svc.ApplyCleanIP(c.ip, c.group, c.inbound, c.mode); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	got, _ := svc.GetHostGroup(group.GroupId)
	if !slices.Equal(got.Hosts, []string{"cdn.example.com:8443"}) {
		t.Fatalf("a rejected apply changed the group: %v", got.Hosts)
	}
}
