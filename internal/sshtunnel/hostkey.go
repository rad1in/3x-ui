package sshtunnel

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"

	"golang.org/x/crypto/ssh"
)

// NewHostKey returns a fresh ed25519 host key as OpenSSH-format PEM.
func NewHostKey() (string, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	block, err := ssh.MarshalPrivateKey(priv, "x-ui ssh tunnel")
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(block)), nil
}

// Fingerprint is the SHA256 fingerprint clients show when they ask to trust
// the server; "" when the key does not parse.
func Fingerprint(hostKey string) string {
	signer, err := ssh.ParsePrivateKey([]byte(hostKey))
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(signer.PublicKey())
}
