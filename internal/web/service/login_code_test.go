package service

import (
	"errors"
	"regexp"
	"testing"
	"time"
)

func newCodes() (*LoginCodeService, *time.Time) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	s := NewLoginCodeService()
	s.now = func() time.Time { return now }
	return s, &now
}

func TestACodeHasSixDigitsAndWorksOnce(t *testing.T) {
	s, _ := newCodes()
	code, err := s.Issue(7)
	if err != nil || !regexp.MustCompile(`^\d{6}$`).MatchString(code) {
		t.Fatalf("code %q, %v; want six digits", code, err)
	}
	if !s.Verify(7, code) {
		t.Fatal("the right code was refused")
	}
	if s.Verify(7, code) {
		t.Fatal("the same code worked twice")
	}
}

func TestACodeBelongsToOneAdmin(t *testing.T) {
	s, _ := newCodes()
	code, _ := s.Issue(7)
	if s.Verify(8, code) {
		t.Fatal("another admin used the code")
	}
	if !s.Verify(7, code) {
		t.Fatal("a failed attempt by someone else burned the code")
	}
}

func TestFiveWrongGuessesBurnTheCode(t *testing.T) {
	s, _ := newCodes()
	code, _ := s.Issue(7)
	for range 5 {
		if s.Verify(7, "000000") && code != "000000" {
			t.Fatal("a wrong code was accepted")
		}
	}
	if s.Verify(7, code) {
		t.Fatal("the code survived five wrong guesses, so it can be brute-forced")
	}
}

func TestACodeExpires(t *testing.T) {
	s, now := newCodes()
	code, _ := s.Issue(7)
	*now = now.Add(6 * time.Minute)
	if s.Verify(7, code) {
		t.Fatal("an expired code was accepted")
	}
}

func TestCodesCannotBeRequestedInQuickSuccession(t *testing.T) {
	s, now := newCodes()
	first, _ := s.Issue(7)
	if _, err := s.Issue(7); !errors.Is(err, ErrCodeTooSoon) {
		t.Fatalf("err = %v, want ErrCodeTooSoon", err)
	}
	*now = now.Add(31 * time.Second)
	second, err := s.Issue(7)
	if err != nil {
		t.Fatalf("a code after the wait: %v", err)
	}
	if first != second && s.Verify(7, first) {
		t.Fatal("the older code still works after a new one was issued")
	}
}
