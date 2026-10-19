package backupcrypt

import (
	"bytes"
	"errors"
	"testing"
)

func TestARoundTripReturnsTheOriginal(t *testing.T) {
	plain := []byte("SQLite format 3\x00 pretend database")
	blob, err := Encrypt("correct horse", plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blob, plain) || bytes.Contains(blob, []byte("SQLite")) {
		t.Fatal("the plaintext shows through the encrypted file")
	}
	got, err := Decrypt("correct horse", blob)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("Decrypt = %q, %v; want the original", got, err)
	}
}

func TestAWrongPassphraseIsToldApartFromACorruptFile(t *testing.T) {
	blob, _ := Encrypt("right", []byte("data"))
	if _, err := Decrypt("wrong", blob); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("err = %v, want ErrWrongPassphrase", err)
	}
	if _, err := Decrypt("right", []byte("not an encrypted backup")); !errors.Is(err, ErrNotEncrypted) {
		t.Fatalf("err = %v, want ErrNotEncrypted", err)
	}
}

func TestATamperedBackupIsRejected(t *testing.T) {
	blob, _ := Encrypt("pw", bytes.Repeat([]byte("x"), 64))
	blob[len(blob)-1] ^= 1
	if _, err := Decrypt("pw", blob); err == nil {
		t.Fatal("a flipped bit went unnoticed")
	}
}

func TestEveryEncryptionIsDifferent(t *testing.T) {
	a, _ := Encrypt("pw", []byte("same"))
	b, _ := Encrypt("pw", []byte("same"))
	if bytes.Equal(a, b) {
		t.Fatal("the same input produced the same file, so salt and nonce are not random")
	}
}

func TestOnlyOurFilesAreRecognisedAsEncrypted(t *testing.T) {
	blob, _ := Encrypt("pw", []byte("d"))
	if !IsEncrypted(blob) {
		t.Fatal("an encrypted backup was not recognised")
	}
	if IsEncrypted([]byte("SQLite format 3\x00")) || IsEncrypted(nil) {
		t.Fatal("a plain file was taken for an encrypted one")
	}
}

func TestAnEmptyPassphraseCannotEncrypt(t *testing.T) {
	if _, err := Encrypt("", []byte("d")); err == nil {
		t.Fatal("an empty passphrase was accepted")
	}
}
