package sstp

import (
	"bytes"
	"testing"
)

// 0x906e is the published check value of CRC-16/X-25, the PPP frame check sequence.
func TestFCSMatchesThePublishedCheckValue(t *testing.T) {
	if got := fcs16(fcsInit, []byte("123456789")) ^ fcsInit; got != 0x906e {
		t.Fatalf("fcs = %#04x, want 0x906e", got)
	}
}

func decodeAll(d *hdlcDecoder, chunks ...[]byte) [][]byte {
	var frames [][]byte
	for _, c := range chunks {
		d.feed(c, func(f []byte) { frames = append(frames, f) })
	}
	return frames
}

func TestHDLCRoundTripEscapesWhatNeedsEscaping(t *testing.T) {
	// A frame holding the flag, the escape and control characters, as LCP options do.
	frame := []byte{0xff, 0x03, 0xc0, 0x21, 0x01, 0x7e, 0x7d, 0x00, 0x1f, 0x20, 0x7f, 0xff}
	wire := hdlcEncode(frame)
	if wire[0] != hdlcFlag || wire[len(wire)-1] != hdlcFlag {
		t.Fatalf("frame is not delimited by flags: % x", wire)
	}
	for _, b := range wire[1 : len(wire)-1] {
		if b == hdlcFlag || b < 0x20 && b != 0 {
			t.Fatalf("an unescaped special byte %#02x inside the frame: % x", b, wire)
		}
	}
	if got := decodeAll(&hdlcDecoder{}, wire); len(got) != 1 || !bytes.Equal(got[0], frame) {
		t.Fatalf("round trip = %x, want %x", got, frame)
	}
}

func TestHDLCDecoderHandlesSplitInputAndBackToBackFrames(t *testing.T) {
	a := []byte{0xff, 0x03, 0xc0, 0x21, 0x09, 0x01, 0x00, 0x04}
	b := []byte{0xff, 0x03, 0x00, 0x21, 0x45, 0x00}
	// Frames share a flag, as pppd writes them, with idle flags in between.
	wire := append(append(hdlcEncode(a), hdlcEncode(b)[1:]...), hdlcFlag, hdlcFlag)
	for cut := 1; cut < len(wire); cut++ {
		got := decodeAll(&hdlcDecoder{}, wire[:cut], wire[cut:])
		if len(got) != 2 || !bytes.Equal(got[0], a) || !bytes.Equal(got[1], b) {
			t.Fatalf("cut at %d: frames = %x", cut, got)
		}
	}
}

func TestHDLCDecoderDropsDamagedFrames(t *testing.T) {
	good := []byte{0xff, 0x03, 0xc0, 0x21, 0x01, 0x01, 0x00, 0x04}
	damaged := hdlcEncode(good)
	damaged[3] ^= 0x40 // flip a bit inside the frame
	oversize := hdlcEncode(bytes.Repeat([]byte{0x41}, maxFrame+10))
	tail := hdlcEncode(good)
	got := decodeAll(&hdlcDecoder{}, damaged, oversize, tail)
	if len(got) != 1 || !bytes.Equal(got[0], good) {
		t.Fatalf("frames = %x; want only the intact one after the damaged and oversize ones", got)
	}
}
