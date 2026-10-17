package cliclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"
)

const tokenName = "x-ui client (CLI)"

// Endpoint is where the local panel listens, as its settings say.
type Endpoint struct {
	Listen   string
	Port     int
	BasePath string
	CertFile string
}

// dialAddr turns a wildcard listen address into the loopback one of its family.
func (e Endpoint) dialAddr() string {
	host := e.Listen
	switch ip := net.ParseIP(host); {
	case host == "":
		host = "127.0.0.1"
	case ip != nil && ip.IsUnspecified() && ip.To4() != nil:
		host = "127.0.0.1"
	case ip != nil && ip.IsUnspecified():
		host = "::1"
	}
	return net.JoinHostPort(host, strconv.Itoa(e.Port))
}

// Client builds the base URL and HTTP client for the endpoint. Over TLS it
// trusts the panel's own certificate and checks it under the name it carries.
func (e Endpoint) Client() (base string, client *http.Client, err error) {
	addr := e.dialAddr()
	basePath := "/" + strings.Trim(e.BasePath, "/")
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
		},
	}
	client = &http.Client{Transport: transport, Timeout: 90 * time.Second}
	if e.CertFile == "" {
		return "http://" + addr + strings.TrimSuffix(basePath, "/"), client, nil
	}
	pemBytes, err := os.ReadFile(e.CertFile)
	if err != nil {
		return "", nil, fmt.Errorf("read panel certificate: %w", err)
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	var leaf *x509.Certificate
	for rest := pemBytes; ; {
		var block *pem.Block
		if block, rest = pem.Decode(rest); block == nil {
			break
		}
		cert, perr := x509.ParseCertificate(block.Bytes)
		if perr != nil {
			continue
		}
		if leaf == nil {
			leaf = cert
		}
		roots.AddCert(cert)
	}
	if leaf == nil {
		return "", nil, errors.New("panel certificate file holds no certificate")
	}
	name := ""
	switch {
	case len(leaf.DNSNames) > 0:
		name = strings.Replace(leaf.DNSNames[0], "*", "x", 1)
	case len(leaf.IPAddresses) > 0:
		name = leaf.IPAddresses[0].String()
	default:
		return "", nil, errors.New("panel certificate names no host")
	}
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	_, port, _ := net.SplitHostPort(addr)
	return "https://" + net.JoinHostPort(name, port) + strings.TrimSuffix(basePath, "/"), client, nil
}

// LocalEndpoint reads the panel's listen settings, with the XUI_PORT override the server honors.
func LocalEndpoint(settings *service.SettingService) (Endpoint, error) {
	var e Endpoint
	var err error
	if e.Listen, err = settings.GetListen(); err != nil {
		return e, err
	}
	if e.Port, err = settings.GetPort(); err != nil {
		return e, err
	}
	if p, ok, perr := config.GetPortOverride(); ok && perr == nil {
		e.Port = p
	}
	if e.BasePath, err = settings.GetBasePath(); err != nil {
		return e, err
	}
	cert, _ := settings.GetCertFile()
	key, _ := settings.GetKeyFile()
	if cert != "" && key != "" {
		if _, lerr := tls.LoadX509KeyPair(cert, key); lerr == nil {
			e.CertFile = cert
		}
	}
	return e, nil
}

// SessionToken mints a short-lived admin token for one CLI run; the returned
// func deletes it. A token left by an interrupted run is replaced here.
func SessionToken() (string, func(), error) {
	tokens := &panel.ApiTokenService{}
	removeOld := func() {
		list, err := tokens.List()
		if err != nil {
			return
		}
		for _, t := range list {
			if t.Name == tokenName {
				_ = tokens.Delete(t.Id)
			}
		}
	}
	removeOld()
	created, err := tokens.Create(tokenName, "admin", time.Now().Add(15*time.Minute).UnixMilli())
	if err != nil {
		return "", func() {}, err
	}
	return created.Token, removeOld, nil
}
