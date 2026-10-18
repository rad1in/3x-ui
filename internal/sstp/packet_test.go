package sstp

import (
	"bufio"
	"bytes"
	"testing"
)

func TestConnectAckHasTheWireLayoutOfMSSSTP(t *testing.T) {
	nonce := bytes.Repeat([]byte{0xab}, nonceSize)
	got := connectAck(nonce)
	want := append([]byte{
		0x10, 0x01, 0x00, 0x30, // version, control, length 48
		0x00, 0x02, 0x00, 0x01, // Call Connect Ack, one attribute
		0x00, 0x04, 0x00, 0x28, // Crypto Binding Request attribute, length 40
		0x00, 0x00, 0x00, 0x03, // reserved, hash bitmask SHA1|SHA256
	}, nonce...)
	if !bytes.Equal(got, want) {
		t.Fatalf("connect ack =\n% x\nwant\n% x", got, want)
	}
}

func TestReadPacketParsesAClientsConnectRequestAndDataFrames(t *testing.T) {
	request := []byte{
		0x10, 0x01, 0x00, 0x0e, // control, length 14
		0x00, 0x01, 0x00, 0x01, // Call Connect Request, one attribute
		0x00, 0x01, 0x00, 0x06, 0x00, 0x01, // Encapsulated Protocol ID = PPP
	}
	data := append([]byte{0x10, 0x00, 0x00, 0x08}, 0xff, 0x03, 0xc0, 0x21)
	r := bufio.NewReader(bytes.NewReader(append(append([]byte(nil), request...), data...)))

	p, err := readPacket(r)
	if err != nil || !p.control || p.msg != msgConnectRequest || !requestsPPP(p) {
		t.Fatalf("connect request parsed as %+v, %v", p, err)
	}
	p, err = readPacket(r)
	if err != nil || p.control || !bytes.Equal(p.data, []byte{0xff, 0x03, 0xc0, 0x21}) {
		t.Fatalf("data packet parsed as %+v, %v", p, err)
	}
}

func TestReadPacketRejectsMalformedInput(t *testing.T) {
	for name, wire := range map[string][]byte{
		"wrong version":            {0x11, 0x00, 0x00, 0x08, 1, 2, 3, 4},
		"length below the header":  {0x10, 0x00, 0x00, 0x03},
		"attribute overruns":       {0x10, 0x01, 0x00, 0x0c, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x40},
		"attributes missing":       {0x10, 0x01, 0x00, 0x08, 0x00, 0x01, 0x00, 0x02},
		"control shorter than hdr": {0x10, 0x01, 0x00, 0x06, 0x00, 0x01},
	} {
		if p, err := readPacket(bufio.NewReader(bytes.NewReader(wire))); err == nil {
			t.Errorf("%s: parsed as %+v", name, p)
		}
	}
}

func TestStatusInfoCarriesTheAttributeAndStatus(t *testing.T) {
	got := controlPacket(msgAbort, statusInfo(0, statusBadFrm))
	want := []byte{
		0x10, 0x01, 0x00, 0x14,
		0x00, 0x05, 0x00, 0x01,
		0x00, 0x02, 0x00, 0x0c,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("abort =\n% x\nwant\n% x", got, want)
	}
}
