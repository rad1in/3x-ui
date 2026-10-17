package service

import (
	"errors"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestSubShortLinkLifecycle(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	var svc SubShortLinkService
	for _, sub := range []string{"sub-a", "sub-b"} {
		if err := database.GetDB().Create(&model.ClientRecord{Email: sub + "@e", SubID: sub}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Ensure("no-such-sub"); !errors.Is(err, ErrSubShortLinkNotFound) {
		t.Fatalf("a subId no client carries must get no token, got %v", err)
	}

	first, err := svc.Ensure("sub-a")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9A-Za-z]{10}$`).MatchString(first.Token) {
		t.Fatalf("token %q is not 10 random alphanumerics", first.Token)
	}
	again, err := svc.Ensure("sub-a")
	if err != nil || again.Token != first.Token {
		t.Fatalf("Ensure must reuse the existing token: got %q (%v), want %q", again.Token, err, first.Token)
	}
	other, _ := svc.Ensure("sub-b")
	if other.Token == first.Token {
		t.Fatal("two subscriptions share one token")
	}
	if subID, err := svc.Resolve(first.Token); err != nil || subID != "sub-a" {
		t.Fatalf("Resolve = %q, %v; want sub-a", subID, err)
	}

	if err := svc.Revoke("sub-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resolve(first.Token); !errors.Is(err, ErrSubShortLinkNotFound) {
		t.Fatalf("a revoked token must stop resolving, got %v", err)
	}
	renewed, _ := svc.Ensure("sub-a")
	if renewed.Token == first.Token {
		t.Fatal("re-creating after revoke must not bring back the revoked token")
	}
	if _, err := svc.Resolve(""); !errors.Is(err, ErrSubShortLinkNotFound) {
		t.Fatalf("empty token must not resolve, got %v", err)
	}
}
