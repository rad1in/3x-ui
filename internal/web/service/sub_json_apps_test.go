package service

import "testing"

func TestNormalizeSubJsonApps(t *testing.T) {
	got, err := normalizeSubJsonApps(" V2rayNG , streisand,v2rayng,, ")
	if err != nil || got != "v2rayng,streisand" {
		t.Fatalf("normalize = %q, %v; want the lower-cased, deduplicated list", got, err)
	}
	if _, err := normalizeSubJsonApps("v2rayng,hiddify"); err == nil || err.Error() != `unknown Xray JSON app "hiddify"` {
		t.Fatalf("an app without an Xray JSON importer must be rejected, got %v", err)
	}
}
