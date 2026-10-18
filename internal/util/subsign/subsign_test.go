package subsign

import (
	"errors"
	"testing"
	"time"
)

var (
	key = []byte("0123456789abcdef0123456789abcdef")
	now = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
)

func TestAFreshSignatureVerifies(t *testing.T) {
	exp := now.Add(time.Hour).Unix()
	sig := Sign(key, "sub-1", exp)
	if err := Verify(key, "sub-1", exp, sig, now); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if err := Verify(key, "sub-1", 0, Sign(key, "sub-1", 0), now); err != nil {
		t.Fatalf("a link without expiry was refused: %v", err)
	}
}

func TestASignatureIsBoundToItsSubscriptionAndExpiry(t *testing.T) {
	exp := now.Add(time.Hour).Unix()
	sig := Sign(key, "sub-1", exp)
	if err := Verify(key, "sub-2", exp, sig, now); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("another subscription accepted the signature: %v", err)
	}
	if err := Verify(key, "sub-1", exp+3600, sig, now); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("an extended expiry was accepted: %v", err)
	}
	if err := Verify(key, "sub-1", 0, sig, now); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("dropping the expiry was accepted: %v", err)
	}
	if err := Verify([]byte("another key another key another!"), "sub-1", exp, sig, now); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("a different key accepted the signature: %v", err)
	}
	if err := Verify(key, "sub-1", exp, "", now); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("an empty signature was accepted: %v", err)
	}
}

func TestAnExpiredLinkIsToldApartFromAForgedOne(t *testing.T) {
	exp := now.Add(-time.Minute).Unix()
	if err := Verify(key, "sub-1", exp, Sign(key, "sub-1", exp), now); !errors.Is(err, ErrExpired) {
		t.Fatalf("err = %v, want ErrExpired", err)
	}
}

func TestAnEmptyKeyNeverVerifies(t *testing.T) {
	if err := Verify(nil, "sub-1", 0, Sign(nil, "sub-1", 0), now); err == nil {
		t.Fatal("a link signed with no key was accepted")
	}
}
