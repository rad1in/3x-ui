package service

import (
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
