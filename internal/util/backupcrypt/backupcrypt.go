// Package backupcrypt seals database backups with a passphrase: AES-256-GCM
// under a scrypt-derived key, so a stolen backup file reveals nothing.
package backupcrypt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"

	"golang.org/x/crypto/scrypt"
)

const (
	saltLen = 16
	keyLen  = 32
	scryptN = 1 << 15
	scryptR = 8
	scryptP = 1
)

// magic starts every encrypted backup; the digit is the format version.
var magic = []byte("3XUIBK1\n")

var (
	ErrNotEncrypted    = errors.New("the file is not an encrypted 3x-ui backup")
	ErrWrongPassphrase = errors.New("wrong passphrase, or the backup is damaged")
)

func gcm(passphrase string, salt []byte) (cipher.AEAD, error) {
	key, err := scrypt.Key([]byte(passphrase), salt, scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// IsEncrypted reports whether data starts with the encrypted-backup marker.
func IsEncrypted(data []byte) bool { return bytes.HasPrefix(data, magic) }

// Encrypt returns marker, salt, nonce and the sealed data. The marker and salt
// are authenticated, so editing either breaks decryption.
func Encrypt(passphrase string, plain []byte) ([]byte, error) {
	if passphrase == "" {
		return nil, errors.New("a backup passphrase is required")
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	aead, err := gcm(passphrase, salt)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	header := append(append([]byte{}, magic...), salt...)
	out := append(header, nonce...)
	return aead.Seal(out, nonce, plain, header), nil
}

// Decrypt reverses Encrypt.
func Decrypt(passphrase string, blob []byte) ([]byte, error) {
	if !IsEncrypted(blob) {
		return nil, ErrNotEncrypted
	}
	headerLen := len(magic) + saltLen
	if len(blob) < headerLen+12 {
		return nil, ErrNotEncrypted
	}
	header := blob[:headerLen]
	aead, err := gcm(passphrase, header[len(magic):])
	if err != nil {
		return nil, err
	}
	rest := blob[headerLen:]
	if len(rest) < aead.NonceSize() {
		return nil, ErrNotEncrypted
	}
	plain, err := aead.Open(nil, rest[:aead.NonceSize()], rest[aead.NonceSize():], header)
	if err != nil {
		return nil, ErrWrongPassphrase
	}
	return plain, nil
}
