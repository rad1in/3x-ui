package sstp

// SSTP carries bare PPP frames while pppd talks asynchronous HDLC (RFC 1662)
// on its terminal, so every frame is wrapped and unwrapped at this boundary.
const (
	hdlcFlag   = 0x7e
	hdlcEscape = 0x7d
	hdlcTrans  = 0x20
	fcsInit    = 0xffff
	fcsGood    = 0xf0b8
	// maxFrame is the largest PPP frame an SSTP data packet can carry.
	maxFrame = 4091
)

var fcsTable = func() (t [256]uint16) {
	for i := range t {
		c := uint16(i)
		for range 8 {
			if c&1 == 1 {
				c = c>>1 ^ 0x8408
			} else {
				c >>= 1
			}
		}
		t[i] = c
	}
	return t
}()

func fcs16(fcs uint16, p []byte) uint16 {
	for _, b := range p {
		fcs = fcs>>8 ^ fcsTable[(fcs^uint16(b))&0xff]
	}
	return fcs
}

// hdlcEncode frames one PPP frame, escaping every control character so the
// peer's async control character map does not matter.
func hdlcEncode(frame []byte) []byte {
	fcs := fcs16(fcsInit, frame) ^ fcsInit
	out := make([]byte, 0, len(frame)+len(frame)/4+8)
	out = append(out, hdlcFlag)
	put := func(b byte) {
		if b < 0x20 || b == hdlcFlag || b == hdlcEscape {
			out = append(out, hdlcEscape, b^hdlcTrans)
			return
		}
		out = append(out, b)
	}
	for _, b := range frame {
		put(b)
	}
	put(byte(fcs))
	put(byte(fcs >> 8))
	return append(out, hdlcFlag)
}

// hdlcDecoder rebuilds PPP frames from the byte stream pppd writes. Frames
// with a bad checksum or too long are dropped, as the kernel would.
type hdlcDecoder struct {
	buf     []byte
	escaped bool
	toss    bool
}

func (d *hdlcDecoder) feed(p []byte, emit func(frame []byte)) {
	for _, b := range p {
		switch {
		case b == hdlcFlag:
			frame, ok := d.take()
			if ok {
				emit(frame)
			}
		case b == hdlcEscape:
			d.escaped = true
		default:
			if d.escaped {
				b ^= hdlcTrans
				d.escaped = false
			}
			if d.toss {
				continue
			}
			if len(d.buf) >= maxFrame+2 {
				d.toss = true
				continue
			}
			d.buf = append(d.buf, b)
		}
	}
}

// take ends the frame in progress at a flag.
func (d *hdlcDecoder) take() ([]byte, bool) {
	buf, bad := d.buf, d.toss || d.escaped
	d.buf, d.escaped, d.toss = d.buf[:0], false, false
	if len(buf) == 0 && !bad {
		return nil, false // idle flag between frames
	}
	if bad || len(buf) <= 2 || fcs16(fcsInit, buf) != fcsGood {
		return nil, false
	}
	return append([]byte(nil), buf[:len(buf)-2]...), true
}
