package service

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/util/subsign"
)

func enableSigning(t *testing.T) {
	t.Helper()
	if err := (&SecurityConfigService{}).Save(&SecurityConfig{SubSigningEnable: true}); err != nil {
		t.Fatal(err)
	}
}

func TestEnablingSigningCreatesAKeyThatSurvivesLaterSaves(t *testing.T) {
	setupBulkDB(t)
	svc := &SecurityConfigService{}
	enableSigning(t)
	first, _ := svc.Get()
	if len(first.SubSigningKey) < 32 {
		t.Fatalf("key %q is too short to be a secret", first.SubSigningKey)
	}
	if err := svc.Save(&SecurityConfig{SubSigningEnable: true, CaptchaEnable: true, SubSigningKey: "client-chosen"}); err != nil {
		t.Fatal(err)
	}
	again, _ := svc.Get()
	if again.SubSigningKey != first.SubSigningKey {
		t.Fatal("an unrelated save changed the signing key, which would kill every issued link")
	}
	view, _ := svc.GetView()
	if view.SubSigningKey != "" {
		t.Fatal("the signing key is served to the admin UI")
	}
	if err := svc.Save(&SecurityConfig{SubSigningEnable: true, RotateSubSigningKey: true}); err != nil {
		t.Fatal(err)
	}
	if rotated, _ := svc.Get(); rotated.SubSigningKey == first.SubSigningKey || rotated.SubSigningKey == "" {
		t.Fatal("rotating did not produce a new key")
	}
}

func TestASignedLinkVerifiesAgainstTheStoredKey(t *testing.T) {
	setupBulkDB(t)
	enableSigning(t)
	svc := &SubSigningService{}

	link, err := svc.SignedURL("sub-abc", time.Hour, "panel.example.com")
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	u, err := url.Parse(link.URL)
	if err != nil {
		t.Fatal(err)
	}
	exp, _ := strconv.ParseInt(u.Query().Get("exp"), 10, 64)
	cfg, _ := (&SecurityConfigService{}).Get()
	if err := subsign.Verify([]byte(cfg.SubSigningKey), "sub-abc", exp, u.Query().Get("sig"), time.Now()); err != nil {
		t.Fatalf("the issued link does not verify: %v (%s)", err, link.URL)
	}
	configured, _ := (&SettingService{}).GetSubPath()
	if want := "/" + strings.Trim(configured, "/") + "/sub-abc"; u.Path != want {
		t.Fatalf("path = %q, want %q", u.Path, want)
	}
	if link.ExpiresAt != exp*1000 {
		t.Fatalf("ExpiresAt = %d, want %d", link.ExpiresAt, exp*1000)
	}
	forever, _ := svc.SignedURL("sub-abc", 0, "panel.example.com")
	if forever.ExpiresAt != 0 || forever.URL == "" {
		t.Fatalf("a link without expiry came back as %+v", forever)
	}
}

func TestNoSignedLinkWhileSigningIsOff(t *testing.T) {
	setupBulkDB(t)
	if _, err := (&SubSigningService{}).SignedURL("sub-abc", time.Hour, "h"); err == nil {
		t.Fatal("a link was signed although signing is off")
	}
}
