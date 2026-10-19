package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/util/s3lite"
)

// BackupResult is how one destination of a scheduled backup went.
type BackupResult struct {
	Destination string `json:"destination" example:"s3"`
	Name        string `json:"name" example:"panel.example.com_2026-10-17_030000.db.gz"`
	Pruned      int    `json:"pruned" example:"1"`
	Error       string `json:"error,omitempty" example:""`
}

// BackupConfig is where scheduled backups go and how many of each are kept.
type BackupConfig struct {
	Keep     int
	LocalDir string
	S3       s3lite.Client
	S3Prefix string
}

func (c BackupConfig) s3Configured() bool {
	return c.S3.Endpoint != "" && c.S3.Bucket != ""
}

func (s *SettingService) GetBackupSchedule() (string, error) {
	return s.getString("backupSchedule")
}

func (s *SettingService) BackupConfig() BackupConfig {
	str := func(key string) string {
		v, _ := s.getString(key)
		return strings.TrimSpace(v)
	}
	keep, _ := s.getInt("backupKeep")
	return BackupConfig{
		Keep:     keep,
		LocalDir: str("backupLocalDir"),
		S3: s3lite.Client{
			Endpoint: str("backupS3Endpoint"), Region: str("backupS3Region"), Bucket: str("backupS3Bucket"),
			AccessKey: str("backupS3AccessKey"), SecretKey: str("backupS3SecretKey"),
		},
		S3Prefix: str("backupS3Prefix"),
	}
}

// ScheduledBackupService writes a gzipped database backup to a local folder
// and/or an S3-compatible bucket, keeping the newest backupKeep in each.
type ScheduledBackupService struct {
	settings SettingService
	server   ServerService
}

// backupName matches the files this service writes: <host>_<date>_<time>.<db|dump>.gz[.enc].
var backupName = regexp.MustCompile(`^(.+)_\d{4}-\d{2}-\d{2}_\d{6}\.(db|dump)\.gz(\.enc)?$`)

// staleBackups picks, among names, this host's backups beyond the newest keep;
// anything else in the folder or bucket is never touched.
func staleBackups(names []string, current string, keep int) []string {
	m := backupName.FindStringSubmatch(current)
	if keep <= 0 || m == nil {
		return nil
	}
	var ours []string
	for _, n := range names {
		if mm := backupName.FindStringSubmatch(n); mm != nil && mm[1] == m[1] {
			ours = append(ours, n)
		}
	}
	sort.Strings(ours)
	if len(ours) <= keep {
		return nil
	}
	return ours[:len(ours)-keep]
}

func (s *ScheduledBackupService) Run(ctx context.Context) ([]BackupResult, error) {
	cfg := s.settings.BackupConfig()
	if cfg.LocalDir == "" && !cfg.s3Configured() {
		return nil, errors.New("no backup destination is configured")
	}
	data, err := s.server.GetDb()
	if err != nil {
		return nil, fmt.Errorf("read database: %w", err)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	name, payload, err := ProtectBackup(s.server.BackupFilename("")+".gz", buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("encrypt backup: %w", err)
	}

	var results []BackupResult
	if cfg.LocalDir != "" {
		results = append(results, backupToDir(cfg.LocalDir, name, payload, cfg.Keep))
	}
	if cfg.s3Configured() {
		results = append(results, backupToS3(ctx, cfg, name, payload))
	}
	return results, nil
}

func backupToDir(dir, name string, gz []byte, keep int) BackupResult {
	res := BackupResult{Destination: "local", Name: name}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		res.Error = err.Error()
		return res
	}
	tmp := filepath.Join(dir, "."+name+".tmp")
	if err := os.WriteFile(tmp, gz, 0o600); err != nil {
		res.Error = err.Error()
		return res
	}
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		_ = os.Remove(tmp)
		res.Error = err.Error()
		return res
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() {
			names = append(names, e.Name())
		}
	}
	for _, old := range staleBackups(names, name, keep) {
		if err := os.Remove(filepath.Join(dir, old)); err == nil {
			res.Pruned++
		}
	}
	return res
}

func backupToS3(ctx context.Context, cfg BackupConfig, name string, gz []byte) BackupResult {
	res := BackupResult{Destination: "s3", Name: name}
	client := cfg.S3
	if err := client.Put(ctx, cfg.S3Prefix+name, gz); err != nil {
		res.Error = err.Error()
		return res
	}
	objects, err := client.List(ctx, cfg.S3Prefix)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	var names []string
	for _, o := range objects {
		if rest, ok := strings.CutPrefix(o.Key, cfg.S3Prefix); ok && !strings.Contains(rest, "/") {
			names = append(names, rest)
		}
	}
	for _, old := range staleBackups(names, name, cfg.Keep) {
		if err := client.Delete(ctx, cfg.S3Prefix+old); err != nil {
			res.Error = err.Error()
			return res
		}
		res.Pruned++
	}
	return res
}
