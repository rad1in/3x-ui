package webauthn_test

import (
	"errors"
	"testing"

	. "github.com/mhsanaei/3x-ui/v3/internal/util/webauthn"
	"github.com/mhsanaei/3x-ui/v3/internal/util/webauthn/webauthntest"
)

func exp(challenge []byte) Expect {
	return Expect{Challenge: challenge, Origin: webauthntest.Origin, RPID: webauthntest.RPID}
}

func TestRegistrationRoundTripPerAlgorithm(t *testing.T) {
	for _, alg := range []int{AlgES256, AlgEdDSA, AlgRS256} {
		s := webauthntest.New(t, alg)
		chal := webauthntest.RandBytes(t, 32)
		cdj, att := s.Register(chal, webauthntest.Origin, webauthntest.RPID, FlagUserPresent)
		cred, err := VerifyRegistration(cdj, att, exp(chal))
		if err != nil {
			t.Fatalf("alg %d: %v", alg, err)
		}
		if string(cred.ID) != string(s.CredID) || string(cred.PublicKey) != string(s.COSE) {
			t.Fatalf("alg %d: credential not returned intact", alg)
		}
		acdj, ad, sig := s.Assert(chal, webauthntest.Origin, webauthntest.RPID, FlagUserPresent)
		n, _, err := VerifyAssertion(acdj, ad, sig, cred.PublicKey, cred.SignCount, exp(chal))
		if err != nil || n != 1 {
			t.Fatalf("alg %d: assertion n=%d err=%v", alg, n, err)
		}
	}
}

func TestRegistrationRejections(t *testing.T) {
	chal := webauthntest.RandBytes(t, 32)
	cases := []struct {
		name    string
		origin  string
		rpID    string
		expOrig string
		expChal []byte
		flags   byte
		wantUV  bool
		want    error
	}{
		{"wrong origin", "https://evil.example.com", webauthntest.RPID, webauthntest.Origin, chal, FlagUserPresent, false, ErrOriginMismatch},
		{"wrong challenge", webauthntest.Origin, webauthntest.RPID, webauthntest.Origin, webauthntest.RandBytes(t, 32), FlagUserPresent, false, ErrChallengeMismatch},
		{"wrong rp id", webauthntest.Origin, "evil.example.com", webauthntest.Origin, chal, FlagUserPresent, false, ErrRPIDMismatch},
		{"no user presence", webauthntest.Origin, webauthntest.RPID, webauthntest.Origin, chal, 0, false, ErrUserNotPresent},
		{"uv required but absent", webauthntest.Origin, webauthntest.RPID, webauthntest.Origin, chal, FlagUserPresent, true, ErrUserNotVerified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := webauthntest.New(t, AlgES256)
			cdj, att := s.Register(chal, tc.origin, tc.rpID, tc.flags)
			e := exp(tc.expChal)
			e.Origin, e.RequireUV = tc.expOrig, tc.wantUV
			if _, err := VerifyRegistration(cdj, att, e); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestAssertionRejections(t *testing.T) {
	s := webauthntest.New(t, AlgES256)
	chal := webauthntest.RandBytes(t, 32)
	cdj0, att := s.Register(chal, webauthntest.Origin, webauthntest.RPID, FlagUserPresent)
	cred, err := VerifyRegistration(cdj0, att, exp(chal))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("wrong origin", func(t *testing.T) {
		cdj, ad, sig := s.Assert(chal, "https://evil.example.com", webauthntest.RPID, FlagUserPresent)
		_, _, err := VerifyAssertion(cdj, ad, sig, cred.PublicKey, 0, exp(chal))
		if !errors.Is(err, ErrOriginMismatch) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("wrong challenge", func(t *testing.T) {
		cdj, ad, sig := s.Assert(webauthntest.RandBytes(t, 32), webauthntest.Origin, webauthntest.RPID, FlagUserPresent)
		_, _, err := VerifyAssertion(cdj, ad, sig, cred.PublicKey, 0, exp(chal))
		if !errors.Is(err, ErrChallengeMismatch) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("signature from another key", func(t *testing.T) {
		other := webauthntest.New(t, AlgES256)
		cdj, ad, sig := other.Assert(chal, webauthntest.Origin, webauthntest.RPID, FlagUserPresent)
		_, _, err := VerifyAssertion(cdj, ad, sig, cred.PublicKey, 0, exp(chal))
		if !errors.Is(err, ErrBadSignature) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("tampered authenticator data", func(t *testing.T) {
		cdj, ad, sig := s.Assert(chal, webauthntest.Origin, webauthntest.RPID, FlagUserPresent)
		ad[len(ad)-1] ^= 0xff
		_, _, err := VerifyAssertion(cdj, ad, sig, cred.PublicKey, 0, exp(chal))
		if !errors.Is(err, ErrBadSignature) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("uv required but absent", func(t *testing.T) {
		cdj, ad, sig := s.Assert(chal, webauthntest.Origin, webauthntest.RPID, FlagUserPresent)
		e := exp(chal)
		e.RequireUV = true
		_, _, err := VerifyAssertion(cdj, ad, sig, cred.PublicKey, 0, e)
		if !errors.Is(err, ErrUserNotVerified) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestAssertionCounterRegression(t *testing.T) {
	s := webauthntest.New(t, AlgES256)
	chal := webauthntest.RandBytes(t, 32)
	cdj, ad, sig := s.Assert(chal, webauthntest.Origin, webauthntest.RPID, FlagUserPresent)
	n, _, err := VerifyAssertion(cdj, ad, sig, s.COSE, 0, exp(chal))
	if err != nil || n != 1 {
		t.Fatalf("first use: n=%d err=%v", n, err)
	}
	// A cloned authenticator replaying counter 1, or an old one, must be refused.
	for _, stored := range []uint32{1, 5} {
		if _, _, err := VerifyAssertion(cdj, ad, sig, s.COSE, stored, exp(chal)); !errors.Is(err, ErrCounterRegression) {
			t.Fatalf("stored %d: got %v", stored, err)
		}
	}
}

func TestAssertionZeroCounterAuthenticator(t *testing.T) {
	s := webauthntest.New(t, AlgES256)
	chal := webauthntest.RandBytes(t, 32)
	cdj, ad, sig := s.AssertWithCounter(chal, webauthntest.Origin, webauthntest.RPID, FlagUserPresent|FlagUserVerified)
	n, uv, err := VerifyAssertion(cdj, ad, sig, s.COSE, 0, exp(chal))
	if err != nil || n != 0 || !uv {
		t.Fatalf("synced passkey with counter 0: n=%d uv=%v err=%v", n, uv, err)
	}
}

func TestMalformedInputsAreRejectedNotPanicking(t *testing.T) {
	chal := webauthntest.RandBytes(t, 32)
	cdj := webauthntest.ClientDataJSON(t, "webauthn.create", chal, webauthntest.Origin)
	for _, att := range [][]byte{nil, {0xa1}, {0xff}, webauthntest.CBORBytes([]byte("x")), webauthntest.CBORMap(webauthntest.CBORText("authData"), webauthntest.CBORBytes([]byte{1, 2}))} {
		if _, err := VerifyRegistration(cdj, att, exp(chal)); err == nil {
			t.Fatalf("accepted %x", att)
		}
	}
}
