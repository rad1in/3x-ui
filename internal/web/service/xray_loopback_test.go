package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// A hosted panel shares the host's network namespace with its siblings; every
// fixed internal port must move to its own loopback address or two Xrays collide.
func TestGeneratedConfigBindsInternalPortsToPanelLoopback(t *testing.T) {
	t.Setenv("XUI_LOOPBACK_IP", "127.0.0.7")
	seedVlessInbound(t, "in-443", 443, []model.Client{{Email: "a@x", ID: "11111111-1111-4111-8111-111111111111", Enable: true}})

	cfg, err := (&XrayService{}).GetXrayConfig()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ib := range cfg.InboundConfigs {
		if ib.Tag == "api" {
			found = true
			if string(ib.Listen) != `"127.0.0.7"` {
				t.Errorf("api inbound listen = %s, want the panel loopback", ib.Listen)
			}
		}
	}
	if !found {
		t.Fatal("template api inbound missing from generated config")
	}

	egress := egressTestConfig()
	injectPanelEgress(egress, "warp")
	if got := string(egress.InboundConfigs[len(egress.InboundConfigs)-1].Listen); got != `"127.0.0.7"` {
		t.Errorf("panel egress listen = %s, want the panel loopback", got)
	}
}

func TestGeneratedConfigMovesMetricsListenToPanelLoopback(t *testing.T) {
	t.Setenv("XUI_LOOPBACK_IP", "127.0.0.7")
	seedVlessInbound(t, "in-443", 443, []model.Client{{Email: "a@x", ID: "11111111-1111-4111-8111-111111111111", Enable: true}})

	cfg, err := (&XrayService{}).GetXrayConfig()
	if err != nil {
		t.Fatal(err)
	}
	var metrics struct {
		Listen string `json:"listen"`
	}
	if err := json.Unmarshal(cfg.Metrics, &metrics); err != nil {
		t.Fatal(err)
	}
	if metrics.Listen != "127.0.0.7:11111" {
		t.Errorf("metrics listen = %q, want the panel loopback with the template port", metrics.Listen)
	}
}

func TestMetricsListenKeepsNonLoopbackHosts(t *testing.T) {
	t.Setenv("XUI_LOOPBACK_IP", "127.0.0.7")
	for in, want := range map[string]string{
		"127.0.0.1:11111": "127.0.0.7:11111",
		"0.0.0.0:11111":   "0.0.0.0:11111",
		"[::1]:11111":     "[::1]:11111",
		"":                "",
	} {
		if got := metricsListenOnLoopback(in); got != want {
			t.Errorf("metricsListenOnLoopback(%q) = %q, want %q", in, got, want)
		}
	}
}
