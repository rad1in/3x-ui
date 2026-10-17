package l2tp

import (
	"net/netip"
	"testing"
)

func TestPoolSplitsIntoServerAddressAndClientRange(t *testing.T) {
	for _, c := range []struct{ pool, local, first, last string }{
		{"10.90.0.0/24", "10.90.0.1", "10.90.0.2", "10.90.0.254"},
		{"10.90.4.0/22", "10.90.4.1", "10.90.4.2", "10.90.7.254"},
		{"192.168.50.8/29", "192.168.50.9", "192.168.50.10", "192.168.50.14"},
	} {
		local, first, last := poolRange(netip.MustParsePrefix(c.pool))
		if local.String() != c.local || first.String() != c.first || last.String() != c.last {
			t.Errorf("%s: %s %s-%s, want %s %s-%s", c.pool, local, first, last, c.local, c.first, c.last)
		}
	}
}
