package sstp

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
)

// Wire constants of [MS-SSTP].
const (
	httpMethod = "SSTP_DUPLEX_POST"
	httpURI    = "/sra_{BA195980-CD49-458b-9E23-C84EE0ADCD75}/"

	version      = 0x10
	flagControl  = 0x01
	maxPacket    = 4095
	nonceSize    = 32
	headerSize   = 4
	ctrlHdrSize  = 8
	attrHdrSize  = 4
	protoPPP     = 0x0001
	hashSHA1     = 0x01
	hashSHA256   = 0x02
	attrProtoID  = 0x01
	attrStatus   = 0x02
	attrBindReq  = 0x04
	statusNoErr  = 0
	statusBadVal = 4
	statusBadFrm = 7
)

// Control message types.
const (
	msgConnectRequest = 0x0001
	msgConnectAck     = 0x0002
	msgConnectNak     = 0x0003
	msgConnected      = 0x0004
	msgAbort          = 0x0005
	msgDisconnect     = 0x0006
	msgDisconnectAck  = 0x0007
	msgEchoRequest    = 0x0008
	msgEchoResponse   = 0x0009
)

var errBadPacket = errors.New("sstp: malformed packet")

// packet is one SSTP packet: a control message or a PPP data frame.
type packet struct {
	control bool
	// For a control message, the type and its attributes; for data, the frame.
	msg   uint16
	attrs []attr
	data  []byte
}

type attr struct {
	id    byte
	value []byte
}

func readPacket(r *bufio.Reader) (*packet, error) {
	var h [headerSize]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	if h[0] != version {
		return nil, errBadPacket
	}
	length := int(binary.BigEndian.Uint16(h[2:4]) & 0x0fff)
	if length < headerSize {
		return nil, errBadPacket
	}
	body := make([]byte, length-headerSize)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	if h[1]&flagControl == 0 {
		return &packet{data: body}, nil
	}
	if len(body) < 4 {
		return nil, errBadPacket
	}
	p := &packet{control: true, msg: binary.BigEndian.Uint16(body[0:2])}
	count := int(binary.BigEndian.Uint16(body[2:4]))
	rest := body[4:]
	for range count {
		if len(rest) < attrHdrSize {
			return nil, errBadPacket
		}
		n := int(binary.BigEndian.Uint16(rest[2:4]))
		if n < attrHdrSize || n > len(rest) {
			return nil, errBadPacket
		}
		p.attrs = append(p.attrs, attr{id: rest[1], value: rest[attrHdrSize:n]})
		rest = rest[n:]
	}
	return p, nil
}

func header(control bool, length int) []byte {
	h := []byte{version, 0, byte(length >> 8), byte(length)}
	if control {
		h[1] = flagControl
	}
	return h
}

func dataPacket(frame []byte) []byte {
	return append(header(false, headerSize+len(frame)), frame...)
}

func controlPacket(msg uint16, attrs ...attr) []byte {
	size := ctrlHdrSize
	for _, a := range attrs {
		size += attrHdrSize + len(a.value)
	}
	out := header(true, size)
	out = binary.BigEndian.AppendUint16(out, msg)
	out = binary.BigEndian.AppendUint16(out, uint16(len(attrs)))
	for _, a := range attrs {
		out = append(out, 0, a.id)
		out = binary.BigEndian.AppendUint16(out, uint16(attrHdrSize+len(a.value)))
		out = append(out, a.value...)
	}
	return out
}

// connectAck asks the client for crypto binding with the given nonce.
func connectAck(nonce []byte) []byte {
	value := append([]byte{0, 0, 0, hashSHA1 | hashSHA256}, nonce...)
	return controlPacket(msgConnectAck, attr{id: attrBindReq, value: value})
}

func statusInfo(attrID byte, status uint32) attr {
	value := []byte{0, 0, 0, attrID}
	value = binary.BigEndian.AppendUint32(value, status)
	return attr{id: attrStatus, value: value}
}
