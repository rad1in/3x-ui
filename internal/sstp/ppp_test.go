package sstp

import (
	"bytes"
	"testing"
)

// chapResponseFrame builds a client's MS-CHAPv2 Response: 49 octets of value
// (peer challenge, reserved, NT response, flags) followed by the name.
func chapResponseFrame(name string) []byte {
	value := make([]byte, 49)
	body := append([]byte{49}, value...)
	body = append(body, name...)
	length := 4 + len(body)
	return append([]byte{0xff, 0x03, 0xc2, 0x23, chapResponse, 0x07, byte(length >> 8), byte(length)}, body...)
}

func TestPPPProtocolReadsEveryHeaderForm(t *testing.T) {
	for name, tc := range map[string]struct {
		frame []byte
		proto uint16
		rest  []byte
	}{
		"full header":        {[]byte{0xff, 0x03, 0x00, 0x21, 0x45, 0x00}, protoIPv4, []byte{0x45, 0x00}},
		"address compressed": {[]byte{0x00, 0x21, 0x45, 0x00}, protoIPv4, []byte{0x45, 0x00}},
		"protocol compress":  {[]byte{0xff, 0x03, 0x21, 0x45, 0x00}, 0x21, []byte{0x45, 0x00}},
		"chap":               {[]byte{0xff, 0x03, 0xc2, 0x23, 0x03, 0x01}, protoCHAP, []byte{0x03, 0x01}},
	} {
		proto, rest, ok := pppProtocol(tc.frame)
		if !ok || proto != tc.proto || !bytes.Equal(rest, tc.rest) {
			t.Errorf("%s: proto %#04x rest %x ok %v; want %#04x %x", name, proto, rest, ok, tc.proto, tc.rest)
		}
	}
	for _, frame := range [][]byte{nil, {0xff, 0x03}, {0x00}} {
		if _, _, ok := pppProtocol(frame); ok {
			t.Errorf("frame %x parsed", frame)
		}
	}
}

func TestChapPacketReadsTheNameOfAResponse(t *testing.T) {
	_, payload, _ := pppProtocol(chapResponseFrame("alice@example.com"))
	code, name, ok := chapPacket(payload)
	if !ok || code != chapResponse || name != "alice@example.com" {
		t.Fatalf("chap = %d %q %v", code, name, ok)
	}
	// Padding after the packet's own length is not part of the name.
	code, name, ok = chapPacket(append(payload, 0x00, 0x00))
	if !ok || code != chapResponse || name != "alice@example.com" {
		t.Fatalf("with padding: chap = %d %q %v", code, name, ok)
	}
	if code, _, ok := chapPacket([]byte{chapSuccess, 0x07, 0x00, 0x04}); !ok || code != chapSuccess {
		t.Fatalf("success packet parsed as %d %v", code, ok)
	}
	for _, bad := range [][]byte{nil, {2, 1, 0, 3}, {2, 1, 0, 9, 49}, {2, 1, 0, 6, 200, 1}} {
		if _, _, ok := chapPacket(bad); ok {
			t.Errorf("malformed packet %x parsed", bad)
		}
	}
}
