package captcha

import (
	"bytes"
	"fmt"
	"image/png"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEveryGlyphLooksDifferent(t *testing.T) {
	seen := map[string]rune{}
	for r, rows := range glyphs {
		key := strings.Join(rows[:], "|")
		if other, dup := seen[key]; dup {
			t.Errorf("glyphs %q and %q have the same bitmap", r, other)
		}
		seen[key] = r
	}
	for _, r := range "0123456789+-x=?" {
		if _, ok := glyphs[r]; !ok {
			t.Errorf("no glyph for %q", r)
		}
	}
}

func TestQuestionsAreSolvableSmallSums(t *testing.T) {
	for range 500 {
		q, answer, err := newQuestion()
		if err != nil {
			t.Fatal(err)
		}
		var a, b int
		var op string
		if _, err := fmt.Sscanf(q, "%d %s %d", &a, &op, &b); err != nil {
			t.Fatalf("question %q does not parse: %v", q, err)
		}
		var want int
		switch op {
		case "+":
			want = a + b
		case "-":
			want = a - b
		case "x":
			want = a * b
		default:
			t.Fatalf("question %q has operator %q", q, op)
		}
		if want < 0 || strconv.Itoa(want) != answer {
			t.Fatalf("question %q: answer %q, want %d (never negative)", q, answer, want)
		}
	}
}

func TestRenderedChallengeIsADecodablePNGThatChangesEveryTime(t *testing.T) {
	first, err := render("7 + 3")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(first))
	if err != nil {
		t.Fatalf("not a PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() < 100 || b.Dy() < 30 {
		t.Fatalf("image is %dx%d, too small to read", b.Dx(), b.Dy())
	}
	second, err := render("7 + 3")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("two renders of the same question are identical, so one solved image solves every replay")
	}
}

func TestAnAnswerWorksOnceOnly(t *testing.T) {
	s := NewStore(time.Minute, 10)
	ch, answer, err := s.issueForTest()
	if err != nil {
		t.Fatal(err)
	}
	if !s.Verify(ch.ID, answer) {
		t.Fatal("the right answer was refused")
	}
	if s.Verify(ch.ID, answer) {
		t.Fatal("the same challenge was accepted twice")
	}
}

func TestAWrongAnswerBurnsTheChallenge(t *testing.T) {
	s := NewStore(time.Minute, 10)
	ch, answer, err := s.issueForTest()
	if err != nil {
		t.Fatal(err)
	}
	if s.Verify(ch.ID, "wrong") {
		t.Fatal("a wrong answer was accepted")
	}
	if s.Verify(ch.ID, answer) {
		t.Fatal("the challenge survived a wrong guess, so answers can be brute-forced")
	}
	if s.Verify(ch.ID, "") || s.Verify("", "") {
		t.Fatal("an empty answer was accepted")
	}
}

func TestAnswerIsMatchedIgnoringSpacesOnly(t *testing.T) {
	s := NewStore(time.Minute, 10)
	ch, answer, _ := s.issueForTest()
	if !s.Verify(ch.ID, " "+answer+" ") {
		t.Fatal("surrounding spaces made the right answer fail")
	}
}

func TestAChallengeExpires(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := NewStore(time.Minute, 10)
	s.now = func() time.Time { return now }
	ch, answer, _ := s.issueForTest()
	now = now.Add(2 * time.Minute)
	if s.Verify(ch.ID, answer) {
		t.Fatal("an expired challenge was accepted")
	}
}

func TestTheStoreStaysBounded(t *testing.T) {
	s := NewStore(time.Minute, 3)
	var firstID, firstAnswer string
	for i := range 10 {
		ch, answer, _ := s.issueForTest()
		if i == 0 {
			firstID, firstAnswer = ch.ID, answer
		}
	}
	if n := s.size(); n > 3 {
		t.Fatalf("store holds %d challenges, cap is 3", n)
	}
	if s.Verify(firstID, firstAnswer) {
		t.Fatal("the oldest challenge outlived the cap")
	}
}
