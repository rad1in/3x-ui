package cliclient

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func selfSigned(t *testing.T, host string) (tls.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host},
		DNSNames: []string{host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// The panel's certificate names its domain, not 127.0.0.1; the CLI dials
// loopback yet still verifies the certificate under that name.
func TestEndpointVerifiesThePanelCertificateOverLoopback(t *testing.T) {
	cert, certPEM := selfSigned(t, "panel.example.com")
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/secret/panel/api/ping" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("pong"))
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	portNum, _ := strconv.Atoi(port)

	dir := t.TempDir()
	own := filepath.Join(dir, "own.pem")
	other := filepath.Join(dir, "other.pem")
	_, otherPEM := selfSigned(t, "panel.example.com")
	if err := os.WriteFile(own, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, otherPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	get := func(certFile string) (string, error) {
		base, client, err := Endpoint{Listen: "0.0.0.0", Port: portNum, BasePath: "/secret/", CertFile: certFile}.Client()
		if err != nil {
			return "", err
		}
		if want := "https://panel.example.com:" + port + "/secret"; base != want {
			t.Fatalf("base = %q, want %q", base, want)
		}
		resp, err := client.Get(base + "/panel/api/ping")
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		buf := make([]byte, 4)
		n, _ := resp.Body.Read(buf)
		return string(buf[:n]), nil
	}
	if body, err := get(own); err != nil || body != "pong" {
		t.Fatalf("with the panel's own certificate: %q, %v", body, err)
	}
	if _, err := get(other); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("a certificate the panel does not serve was trusted: err = %v", err)
	}
}

func TestEndpointDialsLoopbackForWildcardListens(t *testing.T) {
	for listen, want := range map[string]string{"": "127.0.0.1:2053", "0.0.0.0": "127.0.0.1:2053", "::": "[::1]:2053", "10.0.0.5": "10.0.0.5:2053"} {
		if got := (Endpoint{Listen: listen, Port: 2053}).dialAddr(); got != want {
			t.Errorf("listen %q dials %q, want %q", listen, got, want)
		}
	}
}
