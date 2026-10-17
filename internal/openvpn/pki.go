package openvpn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"strings"
	"time"
)

const pkiValidity = 20 * 365 * 24 * time.Hour

// PKI is one inbound's own certificate authority, server identity and
// tls-crypt key, all PEM. Clients log in with a password, so they need no cert.
type PKI struct {
	CA         string `json:"ca"`
	ServerCert string `json:"serverCert"`
	ServerKey  string `json:"serverKey"`
	TLSCrypt   string `json:"tlsCrypt"`
}

func (p PKI) Complete() bool {
	return p.CA != "" && p.ServerCert != "" && p.ServerKey != "" && p.TLSCrypt != ""
}

func NewPKI(name string) (PKI, error) {
	now := time.Now().Add(-time.Hour)
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return PKI{}, err
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          randomSerial(),
		Subject:               pkix.Name{CommonName: name + " CA"},
		NotBefore:             now,
		NotAfter:              now.Add(pkiValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return PKI{}, err
	}
	srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return PKI{}, err
	}
	// remote-cert-tls server on the client checks this key usage and EKU.
	srvTmpl := &x509.Certificate{
		SerialNumber: randomSerial(),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    now,
		NotAfter:     now.Add(pkiValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyAgreement,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return PKI{}, err
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTmpl, caCert, &srvKey.PublicKey, caKey)
	if err != nil {
		return PKI{}, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(srvKey)
	if err != nil {
		return PKI{}, err
	}
	tlsCrypt, err := newStaticKey()
	if err != nil {
		return PKI{}, err
	}
	return PKI{
		CA:         string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})),
		ServerCert: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srvDER})),
		ServerKey:  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})),
		TLSCrypt:   tlsCrypt,
	}, nil
}

func randomSerial() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	return n
}

// newStaticKey writes the 2048-bit key `openvpn --genkey tls-crypt` would.
func newStaticKey() (string, error) {
	key := make([]byte, 256)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("-----BEGIN OpenVPN Static key V1-----\n")
	for i := 0; i < len(key); i += 16 {
		b.WriteString(hex.EncodeToString(key[i : i+16]))
		b.WriteByte('\n')
	}
	b.WriteString("-----END OpenVPN Static key V1-----\n")
	return b.String(), nil
}
