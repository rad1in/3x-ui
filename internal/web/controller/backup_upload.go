package controller

import (
	"bytes"
	"compress/gzip"
	"io"
	"mime/multipart"

	"github.com/mhsanaei/3x-ui/v3/internal/util/backupcrypt"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

type memoryFile struct{ *bytes.Reader }

func (memoryFile) Close() error { return nil }

// openBackupUpload hands a plain upload straight back and unseals an encrypted
// one, with the supplied passphrase or else the configured one. A scheduled
// backup is gzipped under its seal, so that layer is removed too.
func openBackupUpload(file multipart.File, passphrase string) (multipart.File, error) {
	head := make([]byte, 8)
	n, err := io.ReadFull(file, head)
	if _, serr := file.Seek(0, io.SeekStart); serr != nil {
		return nil, serr
	}
	if err != nil && n < len(head) || !backupcrypt.IsEncrypted(head) {
		return file, nil
	}
	blob, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	if passphrase == "" {
		cfg, err := (&service.SecurityConfigService{}).Get()
		if err != nil {
			return nil, err
		}
		passphrase = cfg.BackupPassphrase
	}
	plain, err := backupcrypt.Decrypt(passphrase, blob)
	if err != nil {
		return nil, err
	}
	if len(plain) > 2 && plain[0] == 0x1f && plain[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(plain))
		if err != nil {
			return nil, err
		}
		if plain, err = io.ReadAll(zr); err != nil {
			return nil, err
		}
	}
	return memoryFile{bytes.NewReader(plain)}, nil
}
