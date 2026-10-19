// Package captcha makes the panel's self-hosted login challenge: a small sum
// drawn as a noisy PNG, answered once. It needs no third-party service.
package captcha

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/big"
	"strings"
	"sync"
	"time"
)

const (
	scale    = 4
	glyphW   = 5
	glyphH   = 7
	gap      = 2
	padding  = 10
	jitter   = 6
	noiseDot = 220
)

// glyphs are 5x7 bitmaps for the characters a challenge uses.
var glyphs = map[rune][glyphH]string{
	'0': {".###.", "#...#", "#..##", "#.#.#", "##..#", "#...#", ".###."},
	'1': {"..#..", ".##..", "..#..", "..#..", "..#..", "..#..", ".###."},
	'2': {".###.", "#...#", "....#", "...#.", "..#..", ".#...", "#####"},
	'3': {".###.", "#...#", "....#", "..##.", "....#", "#...#", ".###."},
	'4': {"...#.", "..##.", ".#.#.", "#..#.", "#####", "...#.", "...#."},
	'5': {"#####", "#....", "####.", "....#", "....#", "#...#", ".###."},
	'6': {"..##.", ".#...", "#....", "####.", "#...#", "#...#", ".###."},
	'7': {"#####", "....#", "...#.", "..#..", ".#...", ".#...", ".#..."},
	'8': {".###.", "#...#", "#...#", ".###.", "#...#", "#...#", ".###."},
	'9': {".###.", "#...#", "#...#", ".####", "....#", "...#.", ".##.."},
	'+': {".....", "..#..", "..#..", "#####", "..#..", "..#..", "....."},
	'-': {".....", ".....", ".....", "#####", ".....", ".....", "....."},
	'x': {".....", "#...#", ".#.#.", "..#..", ".#.#.", "#...#", "....."},
	'=': {".....", ".....", "#####", ".....", "#####", ".....", "....."},
	'?': {".###.", "#...#", "....#", "...#.", "..#..", ".....", "..#.."},
}

func randInt(n int) (int, error) {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}

// newQuestion returns a sum such as "7 + 3" and its never-negative answer.
func newQuestion() (question, answer string, err error) {
	a, err := randInt(8)
	if err != nil {
		return "", "", err
	}
	b, err := randInt(8)
	if err != nil {
		return "", "", err
	}
	op, err := randInt(3)
	if err != nil {
		return "", "", err
	}
	a, b = a+2, b+1
	switch op {
	case 0:
		return fmt.Sprintf("%d + %d", a, b), fmt.Sprint(a + b), nil
	case 1:
		if b >= a {
			a, b = b+1, a
		}
		return fmt.Sprintf("%d - %d", a, b), fmt.Sprint(a - b), nil
	}
	b = b%4 + 2
	return fmt.Sprintf("%d x %d", a, b), fmt.Sprint(a * b), nil
}

// render draws the question followed by "= ?" with jittered glyphs, noise dots and strike lines.
func render(question string) ([]byte, error) {
	text := strings.ReplaceAll(question, " ", "") + "=?"
	cell := glyphW*scale + gap*scale
	width := len(text)*cell + padding*2
	height := glyphH*scale + padding*2 + jitter*2
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{245, 245, 245, 255})
		}
	}
	for i, r := range text {
		dy, err := randInt(jitter*2 + 1)
		if err != nil {
			return nil, err
		}
		shade, err := randInt(90)
		if err != nil {
			return nil, err
		}
		ink := color.RGBA{uint8(20 + shade), uint8(20 + shade/2), uint8(60 + shade), 255}
		for gy, row := range glyphs[r] {
			for gx, c := range row {
				if c != '#' {
					continue
				}
				for sy := range scale {
					for sx := range scale {
						img.Set(padding+i*cell+gx*scale+sx, padding+dy+gy*scale+sy, ink)
					}
				}
			}
		}
	}
	for range noiseDot {
		x, _ := randInt(width)
		y, _ := randInt(height)
		v, _ := randInt(160)
		img.Set(x, y, color.RGBA{uint8(v), uint8(v), uint8(v), 255})
	}
	for range 3 {
		y0, _ := randInt(height)
		y1, _ := randInt(height)
		for x := range width {
			y := y0 + (y1-y0)*x/width
			img.Set(x, y, color.RGBA{90, 90, 90, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Challenge is what the login page shows: an id to send back and the picture.
// Answer is for tests; handlers must send only ID and PNG.
type Challenge struct {
	ID     string
	PNG    []byte
	Answer string
}

type entry struct {
	answer  string
	expires time.Time
	seq     uint64
}

// Store holds the open challenges in memory, each good for one answer.
type Store struct {
	mu    sync.Mutex
	items map[string]entry
	ttl   time.Duration
	max   int
	seq   uint64
	now   func() time.Time
}

func NewStore(ttl time.Duration, max int) *Store {
	return &Store{items: map[string]entry{}, ttl: ttl, max: max, now: time.Now}
}

func (s *Store) size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

func (s *Store) issueForTest() (Challenge, string, error) {
	return s.issue()
}

// Issue creates a challenge; the oldest ones are dropped once the store is full.
func (s *Store) Issue() (Challenge, error) {
	ch, _, err := s.issue()
	return ch, err
}

func (s *Store) issue() (Challenge, string, error) {
	q, answer, err := newQuestion()
	if err != nil {
		return Challenge{}, "", err
	}
	pngBytes, err := render(q)
	if err != nil {
		return Challenge{}, "", err
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return Challenge{}, "", err
	}
	id := hex.EncodeToString(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for k, e := range s.items {
		if now.After(e.expires) {
			delete(s.items, k)
		}
	}
	for len(s.items) >= s.max {
		oldest, oldestSeq := "", ^uint64(0)
		for k, e := range s.items {
			if e.seq < oldestSeq {
				oldest, oldestSeq = k, e.seq
			}
		}
		delete(s.items, oldest)
	}
	s.seq++
	s.items[id] = entry{answer: answer, expires: now.Add(s.ttl), seq: s.seq}
	return Challenge{ID: id, PNG: pngBytes, Answer: answer}, answer, nil
}

// Verify consumes the challenge whether or not the answer is right, so a guess
// cannot be repeated against the same picture.
func (s *Store) Verify(id, answer string) bool {
	answer = strings.TrimSpace(answer)
	s.mu.Lock()
	e, ok := s.items[id]
	delete(s.items, id)
	s.mu.Unlock()
	return ok && answer != "" && s.now().Before(e.expires) && answer == e.answer
}
