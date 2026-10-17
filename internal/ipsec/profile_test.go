package ipsec

import (
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

// plistValues walks a plist and returns each <key>'s following scalar, plus
// every <data> payload, so the test reads the profile the way iOS would.
func plistValues(t *testing.T, doc []byte) (map[string][]string, [][]byte) {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(doc))
	dec.Strict = true
	values := map[string][]string{}
	var datas [][]byte
	var lastKey, elem string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("profile is not well-formed XML: %v", err)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			elem = v.Name.Local
		case xml.EndElement:
			elem = ""
		case xml.CharData:
			text := string(v)
			switch elem {
			case "key":
				lastKey = text
			case "string", "integer":
				values[lastKey] = append(values[lastKey], text)
			case "data":
				raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(text), ""))
				if err != nil {
					t.Fatalf("payload data is not base64: %v", err)
				}
				datas = append(datas, raw)
			}
		}
	}
	return values, datas
}

func testInstance(t *testing.T) Instance {
	t.Helper()
	pki, err := PKIFor(PKI{}, "vpn.example.com")
	if err != nil {
		t.Fatal(err)
	}
	return Instance{Id: 3, Tag: "in-500-udp", Address: "vpn.example.com", PKI: pki}
}

func TestMobileConfigCarriesTheLoginAndTrustsTheCA(t *testing.T) {
	inst := testInstance(t)
	password := `p<&>"'x`
	values, datas := plistValues(t, MobileConfig(inst, "ali", password, "My VPN"))

	for key, want := range map[string]string{
		"VPNType": "IKEv2", "RemoteAddress": "vpn.example.com", "RemoteIdentifier": "vpn.example.com",
		"AuthName": "ali", "AuthPassword": password, "ExtendedAuthEnabled": "1",
	} {
		if got := values[key]; len(got) != 1 || got[0] != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	ca, err := CACert(inst.PKI)
	if err != nil {
		t.Fatal(err)
	}
	if len(datas) != 1 || !bytes.Equal(datas[0], ca.Raw) {
		t.Fatal("the profile does not install the server's CA")
	}
}

func TestSSWANProfileForTheAndroidApp(t *testing.T) {
	inst := testInstance(t)
	var p struct {
		Type   string `json:"type"`
		Remote struct {
			Addr string `json:"addr"`
			ID   string `json:"id"`
			Cert string `json:"cert"`
		} `json:"remote"`
		Local struct {
			EAPID string `json:"eap_id"`
		} `json:"local"`
	}
	if err := json.Unmarshal(SSWAN(inst, "ali", "My VPN"), &p); err != nil {
		t.Fatal(err)
	}
	der, err := base64.StdEncoding.DecodeString(p.Remote.Cert)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil || !cert.IsCA {
		t.Fatalf("remote.cert is not the CA: %v", err)
	}
	if p.Type != "ikev2-eap" || p.Remote.Addr != "vpn.example.com" || p.Local.EAPID != "ali" {
		t.Fatalf("profile = %+v", p)
	}
}
