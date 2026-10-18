package service

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/crypto/nodetoken"
	"github.com/mhsanaei/3x-ui/v3/internal/util/backupcrypt"
)

func setBackupPassphrase(t *testing.T, pass string) {
	t.Helper()
	if err := (&SecurityConfigService{}).Save(&SecurityConfig{BackupPassphrase: pass}); err != nil {
		t.Fatal(err)
	}
}

func TestBackupPassphraseIsStoredEncryptedAndServedBlank(t *testing.T) {
	setupBulkDB(t)
	codec, err := nodetoken.NewCodec(nodetoken.ModeRequired, &nodetoken.Keyring{ActiveID: "k1", Keys: map[string][32]byte{"k1": {4, 5, 6}}})
	if err != nil {
		t.Fatal(err)
	}
	nodetoken.Init(codec)
	t.Cleanup(func() {
		off, _ := nodetoken.NewCodec(nodetoken.ModeOff, nil)
		nodetoken.Init(off)
	})
	cfgSvc := &SecurityConfigService{}
	setBackupPassphrase(t, "open sesame")

	raw, _ := cfgSvc.settings.getString(securityConfigKey)
	if strings.Contains(raw, "open sesame") || !strings.Contains(raw, "enc:v1:") {
		t.Fatalf("the passphrase is stored in the clear: %s", raw)
	}
	view, err := cfgSvc.GetView()
	if err != nil || view.BackupPassphrase != "" || !view.HasBackupPassphrase {
		t.Fatalf("view = %+v, %v; want a blank passphrase flagged as set", view, err)
	}
	cfg, _ := cfgSvc.Get()
	if cfg.BackupPassphrase != "open sesame" {
		t.Fatalf("Get lost the passphrase: %q", cfg.BackupPassphrase)
	}

	if err := cfgSvc.Save(&SecurityConfig{CaptchaEnable: true}); err != nil {
		t.Fatal(err)
	}
	if cfg, _ = cfgSvc.Get(); cfg.BackupPassphrase != "open sesame" || !cfg.CaptchaEnable {
		t.Fatalf("a save with a blank passphrase must keep it: %+v", cfg)
	}
	if err := cfgSvc.Save(&SecurityConfig{ClearBackupPassphrase: true}); err != nil {
		t.Fatal(err)
	}
	if cfg, _ = cfgSvc.Get(); cfg.BackupPassphrase != "" {
		t.Fatalf("the passphrase survived being cleared: %q", cfg.BackupPassphrase)
	}
}

func TestProtectBackupEncryptsOnlyWhenAPassphraseIsSet(t *testing.T) {
	setupBulkDB(t)
	name, data, err := ProtectBackup("host_2026-01-01_000000.db", []byte("secret rows"))
	if err != nil || name != "host_2026-01-01_000000.db" || string(data) != "secret rows" {
		t.Fatalf("without a passphrase: %q, %q, %v; want the file untouched", name, data, err)
	}
	setBackupPassphrase(t, "pw")
	name, data, err = ProtectBackup("host_2026-01-01_000000.db", []byte("secret rows"))
	if err != nil || name != "host_2026-01-01_000000.db.enc" || bytes.Contains(data, []byte("secret rows")) {
		t.Fatalf("with a passphrase: %q, %v; want an .enc file without the plaintext", name, err)
	}
	if plain, err := backupcrypt.Decrypt("pw", data); err != nil || string(plain) != "secret rows" {
		t.Fatalf("the protected backup does not decrypt: %q, %v", plain, err)
	}
}

func TestEncryptedBackupsAreStillPrunedByTheirHost(t *testing.T) {
	current := "h_2026-01-03_000000.db.gz.enc"
	names := []string{"h_2026-01-01_000000.db.gz.enc", "h_2026-01-02_000000.db.gz", current, "other_2026-01-01_000000.db.gz.enc", "notes.txt"}
	got := staleBackups(names, current, 2)
	if len(got) != 1 || got[0] != "h_2026-01-01_000000.db.gz.enc" {
		t.Fatalf("stale = %v, want only this host's oldest encrypted backup", got)
	}
}

func TestScheduledBackupsAreEncryptedWhenAPassphraseIsSet(t *testing.T) {
	setupBulkDB(t)
	setBackupPassphrase(t, "pw")
	dir := t.TempDir()
	if err := (&SettingService{}).saveSetting("backupLocalDir", dir); err != nil {
		t.Fatal(err)
	}
	results, err := (&ScheduledBackupService{}).Run(t.Context())
	if err != nil || len(results) != 1 || results[0].Error != "" {
		t.Fatalf("Run = %+v, %v", results, err)
	}
	if !strings.HasSuffix(results[0].Name, ".gz.enc") {
		t.Fatalf("backup name %q, want a .gz.enc file", results[0].Name)
	}
}
