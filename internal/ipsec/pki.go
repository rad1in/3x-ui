package ipsec

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"strings"
	"time"
)

const (
	caValidity     = 20 * 365 * 24 * time.Hour
	serverValidity = 10 * 365 * 24 * time.Hour
	// RSA rather than ECDSA: Windows' built-in client and strongSwan builds
	// without the openssl plugin both refuse ECDSA server certificates.
	rsaBits = 2048
)

// oidIKEIntermediate is the EKU older macOS and Windows IKEv2 clients look for.
var oidIKEIntermediate = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 8, 2, 2}

// PKI is one IPsec server's CA and its server identity, all PEM. The CA key
// stays so the server cert can be reissued without breaking installed profiles.
type PKI struct {
	CA         string `json:"ca"`
	CAKey      string `json:"caKey"`
	ServerCert string `json:"serverCert"`
	ServerKey  string `json:"serverKey"`
}

func (p PKI) Complete() bool {
	return p.CA != "" && p.CAKey != "" && p.ServerCert != "" && p.ServerKey != ""
}

// PKIFor returns certificates whose server cert names address: cur unchanged
// when it already does, else a server cert reissued from cur's CA (or a new CA).
func PKIFor(cur PKI, address string) (PKI, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return PKI{}, errors.New("ipsec: the server address is empty")
	}
	if cur.Complete() && serverCertNames(cur.ServerCert, address) {
		return cur, nil
	}
	caCert, caKey, err := parseCA(cur)
	if err != nil {
		return PKI{}, err
	}
	next := cur
	if caCert == nil {
		if caCert, caKey, next, err = newCA(); err != nil {
			return PKI{}, err
		}
	}
	srvKey, err := rsa.GenerateKey(rand.Reader, rsaBits)
	if err != nil {
		return PKI{}, err
	}
	now := time.Now().Add(-time.Hour)
	tmpl := &x509.Certificate{
		SerialNumber:       randomSerial(),
		Subject:            pkix.Name{CommonName: address},
		NotBefore:          now,
		NotAfter:           now.Add(serverValidity),
		KeyUsage:           x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:        []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		UnknownExtKeyUsage: []asn1.ObjectIdentifier{oidIKEIntermediate},
	}
	if ip := net.ParseIP(address); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{address}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &srvKey.PublicKey, caKey)
	if err != nil {
		return PKI{}, err
	}
	next.ServerCert = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	next.ServerKey = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(srvKey)}))
	return next, nil
}

func newCA() (*x509.Certificate, *rsa.PrivateKey, PKI, error) {
	key, err := rsa.GenerateKey(rand.Reader, rsaBits)
	if err != nil {
		return nil, nil, PKI{}, err
	}
	now := time.Now().Add(-time.Hour)
	tmpl := &x509.Certificate{
		SerialNumber:          randomSerial(),
		Subject:               pkix.Name{CommonName: "x-ui IPsec CA"},
		NotBefore:             now,
		NotAfter:              now.Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, PKI{}, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, PKI{}, err
	}
	return cert, key, PKI{
		CA:    string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		CAKey: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})),
	}, nil
}

// parseCA returns a nil cert, and no error, when cur carries no CA yet.
func parseCA(cur PKI) (*x509.Certificate, *rsa.PrivateKey, error) {
	if cur.CA == "" || cur.CAKey == "" {
		return nil, nil, nil
	}
	cert, err := CACert(cur)
	if err != nil {
		return nil, nil, err
	}
	block, _ := pem.Decode([]byte(cur.CAKey))
	if block == nil {
		return nil, nil, errors.New("ipsec: the CA key is not PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

// CACert is the certificate clients install to trust the server.
func CACert(p PKI) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(p.CA))
	if block == nil {
		return nil, errors.New("ipsec: the CA certificate is not PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}

func serverCertNames(certPEM, address string) bool {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	return cert.VerifyHostname(address) == nil
}

func randomSerial() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	return n
}
