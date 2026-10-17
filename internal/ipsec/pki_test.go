package ipsec

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func parseCert(t *testing.T, p string) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode([]byte(p))
	if block == nil {
		t.Fatalf("not PEM: %q", p)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

// verifyServer checks the server cert the way iOS and Windows do: chained to
// the CA, named after the address the client dials, and fit for server auth.
func verifyServer(t *testing.T, p PKI, address string) error {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AddCert(parseCert(t, p.CA))
	_, err := parseCert(t, p.ServerCert).Verify(x509.VerifyOptions{
		DNSName: address, Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	return err
}

func TestServerCertNamesTheAddressClientsDial(t *testing.T) {
	for _, address := range []string{"vpn.example.com", "203.0.113.7"} {
		t.Run(address, func(t *testing.T) {
			p, err := PKIFor(PKI{}, address)
			if err != nil {
				t.Fatal(err)
			}
			if err := verifyServer(t, p, address); err != nil {
				t.Fatalf("server cert does not verify for %s: %v", address, err)
			}
			if err := verifyServer(t, p, "other.example.com"); err == nil {
				t.Fatal("server cert verified for an address it was not issued for")
			}
		})
	}
}

// Installed profiles trust the CA, so a new address must not mint a new one.
func TestChangingTheAddressKeepsTheCA(t *testing.T) {
	first, err := PKIFor(PKI{}, "203.0.113.7")
	if err != nil {
		t.Fatal(err)
	}
	same, err := PKIFor(first, "203.0.113.7")
	if err != nil {
		t.Fatal(err)
	}
	if same != first {
		t.Fatal("an unchanged address reissued the certificates")
	}
	moved, err := PKIFor(first, "vpn.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if moved.CA != first.CA || moved.CAKey != first.CAKey {
		t.Fatal("a new address replaced the CA")
	}
	if err := verifyServer(t, moved, "vpn.example.com"); err != nil {
		t.Fatalf("reissued server cert does not verify: %v", err)
	}
}
