package webauthn

import (
	"errors"
	"math"
)

const cborMaxDepth = 8

var errCBOR = errors.New("webauthn: malformed CBOR")

// decodeCBOR reads one data item and returns it with the bytes it used. It
// handles only what WebAuthn emits: definite lengths, no floats.
// Maps decode to map[any]any with int64 or string keys.
func decodeCBOR(b []byte) (any, int, error) {
	return decodeCBORItem(b, 0)
}

func decodeCBORItem(b []byte, depth int) (any, int, error) {
	if depth > cborMaxDepth || len(b) == 0 {
		return nil, 0, errCBOR
	}
	major := b[0] >> 5
	info := b[0] & 0x1f
	n, hdr, err := cborArg(b, info)
	if err != nil {
		return nil, 0, err
	}
	switch major {
	case 0:
		if n > math.MaxInt64 {
			return nil, 0, errCBOR
		}
		return int64(n), hdr, nil
	case 1:
		if n > math.MaxInt64 {
			return nil, 0, errCBOR
		}
		return -1 - int64(n), hdr, nil
	case 2, 3:
		if n > uint64(len(b)-hdr) {
			return nil, 0, errCBOR
		}
		raw := b[hdr : hdr+int(n)]
		if major == 3 {
			return string(raw), hdr + int(n), nil
		}
		return append([]byte(nil), raw...), hdr + int(n), nil
	case 4:
		if n > uint64(len(b)) {
			return nil, 0, errCBOR
		}
		out := make([]any, 0, n)
		used := hdr
		for range n {
			v, c, err := decodeCBORItem(b[used:], depth+1)
			if err != nil {
				return nil, 0, err
			}
			out = append(out, v)
			used += c
		}
		return out, used, nil
	case 5:
		if n > uint64(len(b)) {
			return nil, 0, errCBOR
		}
		out := make(map[any]any, n)
		used := hdr
		for range n {
			k, c, err := decodeCBORItem(b[used:], depth+1)
			if err != nil {
				return nil, 0, err
			}
			used += c
			switch k.(type) {
			case int64, string:
			default:
				return nil, 0, errCBOR
			}
			v, c, err := decodeCBORItem(b[used:], depth+1)
			if err != nil {
				return nil, 0, err
			}
			used += c
			out[k] = v
		}
		return out, used, nil
	case 7:
		switch info {
		case 20:
			return false, hdr, nil
		case 21:
			return true, hdr, nil
		case 22:
			return nil, hdr, nil
		}
	}
	return nil, 0, errCBOR
}

// cborArg decodes the length/value argument that follows the initial byte.
func cborArg(b []byte, info byte) (uint64, int, error) {
	switch {
	case info < 24:
		return uint64(info), 1, nil
	case info > 27:
		return 0, 0, errCBOR
	}
	size := 1 << (info - 24)
	if len(b) < 1+size {
		return 0, 0, errCBOR
	}
	var n uint64
	for _, x := range b[1 : 1+size] {
		n = n<<8 | uint64(x)
	}
	return n, 1 + size, nil
}
