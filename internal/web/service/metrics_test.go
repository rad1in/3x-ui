package service

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func TestMetricsRenderExposesTrafficAndNodeHealth(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	db := database.GetDB()
	if err := db.Create(&model.Inbound{Tag: "in-443", Remark: `a "quoted"\name`, Protocol: model.VLESS, Port: 443, Enable: true, Up: 11, Down: 22, Settings: `{"clients":[]}`}).Error; err != nil {
		t.Fatalf("seed inbound: %v", err)
	}
	if err := db.Create(&xray.ClientTraffic{InboundId: 1, Email: "alice@x", Enable: true, Up: 5, Down: 7, Total: 100}).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}
	if err := db.Create(&model.Node{Name: "de-1", Address: "n1", Port: 2053, Enable: true, Status: "online", LatencyMs: 42, CpuPct: 12.5, MemPct: 30}).Error; err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if err := db.Create(&model.Node{Name: "us-1", Address: "n2", Port: 2053, Enable: true, Status: "offline"}).Error; err != nil {
		t.Fatalf("seed node: %v", err)
	}

	out := (&MetricsService{}).Render(&Status{})

	for _, want := range []string{
		`xui_inbound_traffic_bytes{inbound="in-443",remark="a \"quoted\"\\name",protocol="vless",direction="up"} 11` + "\n",
		`xui_inbound_traffic_bytes{inbound="in-443",remark="a \"quoted\"\\name",protocol="vless",direction="down"} 22` + "\n",
		`xui_client_traffic_bytes{email="alice@x",inbound="in-443",direction="down"} 7` + "\n",
		`xui_client_traffic_limit_bytes{email="alice@x",inbound="in-443"} 100` + "\n",
		`xui_node_up{node="de-1"} 1` + "\n",
		`xui_node_up{node="us-1"} 0` + "\n",
		`xui_node_latency_seconds{node="de-1"} 0.042` + "\n",
		`xui_node_cpu_ratio{node="de-1"} 0.125` + "\n",
		"# TYPE xui_inbound_traffic_bytes counter\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics output lacks %q\n---\n%s", want, out)
		}
	}
}

// The shipped dashboard queries metric names by hand; a renamed or dropped
// family would leave its panels silently empty.
func TestGrafanaDashboardQueriesOnlyExportedMetrics(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "monitoring", "grafana-dashboard.json"))
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	exported := (&MetricsService{}).Render(&Status{})
	names := regexp.MustCompile(`xui_[a-z_]+`).FindAllString(string(raw), -1)
	if len(names) == 0 {
		t.Fatal("dashboard references no xui_ metrics")
	}
	for _, name := range names {
		if !strings.Contains(exported, "# TYPE "+name+" ") {
			t.Errorf("dashboard queries %s, which the endpoint does not export", name)
		}
	}
}
