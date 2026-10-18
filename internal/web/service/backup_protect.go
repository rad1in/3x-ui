package service

import "github.com/mhsanaei/3x-ui/v3/internal/util/backupcrypt"

// EncryptedBackupSuffix ends the name of a backup sealed with the passphrase.
const EncryptedBackupSuffix = ".enc"

// ProtectBackup seals a backup file with the configured passphrase and adds
// the .enc suffix; with no passphrase it returns both unchanged.
func ProtectBackup(filename string, data []byte) (string, []byte, error) {
	cfg, err := (&SecurityConfigService{}).Get()
	if err != nil {
		return "", nil, err
	}
	if cfg.BackupPassphrase == "" {
		return filename, data, nil
	}
	sealed, err := backupcrypt.Encrypt(cfg.BackupPassphrase, data)
	if err != nil {
		return "", nil, err
	}
	return filename + EncryptedBackupSuffix, sealed, nil
}
