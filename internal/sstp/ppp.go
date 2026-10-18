package sstp

import "encoding/binary"

const (
	protoIPv4 = 0x0021
	protoIPv6 = 0x0057
	protoCHAP = 0xc223
)

// CHAP codes (RFC 1994).
const (
	chapResponse = 2
	chapSuccess  = 3
)

// pppProtocol splits a PPP frame into its protocol and payload, whether or not
// the address/control and protocol-field compressions were negotiated.
func pppProtocol(frame []byte) (proto uint16, payload []byte, ok bool) {
	if len(frame) >= 2 && frame[0] == 0xff && frame[1] == 0x03 {
		frame = frame[2:]
	}
	if len(frame) == 0 {
		return 0, nil, false
	}
	if frame[0]&1 == 1 {
		return uint16(frame[0]), frame[1:], true
	}
	if len(frame) < 2 {
		return 0, nil, false
	}
	return binary.BigEndian.Uint16(frame), frame[2:], true
}

func isUserData(proto uint16) bool { return proto == protoIPv4 || proto == protoIPv6 }

// chapPacket reads the code and, for a Response, the peer's name from a CHAP
// payload: code, id, length, then for a Response the value and the name.
func chapPacket(payload []byte) (code byte, name string, ok bool) {
	if len(payload) < 4 {
		return 0, "", false
	}
	code = payload[0]
	length := int(binary.BigEndian.Uint16(payload[2:4]))
	if length < 4 || length > len(payload) {
		return 0, "", false
	}
	body := payload[4:length]
	if code != chapResponse {
		return code, "", true
	}
	if len(body) < 1 || len(body) < 1+int(body[0]) {
		return 0, "", false
	}
	return code, string(body[1+int(body[0]):]), true
}
