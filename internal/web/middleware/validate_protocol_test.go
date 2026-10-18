package middleware

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// The inbound add/update handlers validate the body with this validator, so a protocol
// missing from Inbound.Protocol's oneof makes every save of that protocol fail.
func TestValidateAcceptsInboundProtocols(t *testing.T) {
	for _, p := range []model.Protocol{model.MASQUE, model.OpenVPN, model.IKEv2, model.L2TP, model.SSH, model.SSTP, model.OCServ} {
		if err := validate.StructPartial(model.Inbound{Protocol: p}, "Protocol"); err != nil {
			t.Errorf("validate rejected a %s inbound: %v", p, err)
		}
	}
	if err := validate.StructPartial(model.Inbound{Protocol: "dokodemo"}, "Protocol"); err == nil {
		t.Fatal("validate accepted an unknown protocol; the oneof no longer guards it")
	}
}
