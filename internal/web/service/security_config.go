package service

import (
	"encoding/json"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/crypto/nodetoken"
)

// SecurityConfig groups the login and data protections that do not depend on
// two-factor codes. It is one JSON setting that only owners may change.
// BackupPassphrase is a secret: stored encrypted, served blank, and kept when a
// save leaves it blank (ClearBackupPassphrase empties it on purpose).
type SecurityConfig struct {
	CaptchaEnable         bool   `json:"captchaEnable"`
	NewLocationAlert      bool   `json:"newLocationAlert"`
	BackupPassphrase      string `json:"backupPassphrase"`
	ClearBackupPassphrase bool   `json:"clearBackupPassphrase,omitempty"`
}

// SecurityConfigView is the configuration as the admin API serves it: secrets
// are blank and the Has flags say which are set.
type SecurityConfigView struct {
	SecurityConfig
	HasBackupPassphrase bool `json:"hasBackupPassphrase"`
}

const securityConfigKey = "securityConfig"

func backupPassphraseAAD() []byte { return []byte("settings/securityConfig/backupPassphrase") }

// SecurityConfigService reads and writes the security configuration.
type SecurityConfigService struct {
	settings SettingService
}

// Get returns the configuration with secrets in the clear, for the server's
// own use. Never hand this to a response.
func (s *SecurityConfigService) Get() (*SecurityConfig, error) {
	cfg := &SecurityConfig{}
	raw, err := s.settings.getString(securityConfigKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return cfg, err
	}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, err
	}
	cfg.BackupPassphrase, err = nodetoken.DecryptBound(backupPassphraseAAD(), cfg.BackupPassphrase)
	return cfg, err
}

func (s *SecurityConfigService) GetView() (*SecurityConfigView, error) {
	cfg, err := s.Get()
	if err != nil {
		return nil, err
	}
	view := &SecurityConfigView{HasBackupPassphrase: cfg.BackupPassphrase != ""}
	cfg.BackupPassphrase = ""
	view.SecurityConfig = *cfg
	return view, nil
}

func (s *SecurityConfigService) Save(cfg *SecurityConfig) error {
	existing, err := s.Get()
	if err != nil {
		return err
	}
	stored := *cfg
	stored.ClearBackupPassphrase = false
	stored.BackupPassphrase = strings.TrimSpace(stored.BackupPassphrase)
	switch {
	case cfg.ClearBackupPassphrase:
		stored.BackupPassphrase = ""
	case stored.BackupPassphrase == "":
		stored.BackupPassphrase = existing.BackupPassphrase
	}
	if stored.BackupPassphrase != "" {
		if stored.BackupPassphrase, err = nodetoken.EncryptBound(backupPassphraseAAD(), stored.BackupPassphrase); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(&stored)
	if err != nil {
		return err
	}
	return s.settings.saveSetting(securityConfigKey, string(raw))
}
