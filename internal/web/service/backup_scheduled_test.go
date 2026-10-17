package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
)

// fakeS3 keeps objects in memory and answers the three calls the backup makes.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	auth    []string
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	key := strings.TrimPrefix(r.URL.Path, "/bucket/")
	switch r.Method {
	case http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		f.objects[key] = body
	case http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	case http.MethodGet:
		type content struct {
			Key  string `xml:"Key"`
			Size int    `xml:"Size"`
		}
		var page struct {
			XMLName  xml.Name  `xml:"ListBucketResult"`
			Contents []content `xml:"Contents"`
		}
		var keys []string
		for k := range f.objects {
			if strings.HasPrefix(k, r.URL.Query().Get("prefix")) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			page.Contents = append(page.Contents, content{Key: k, Size: len(f.objects[k])})
		}
		_ = xml.NewEncoder(w).Encode(page)
	}
}

func (f *fakeS3) keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var keys []string
	for k := range f.objects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestScheduledBackupWritesBothDestinationsAndKeepsTheNewest(t *testing.T) {
	setupConflictDB(t)
	s3 := &fakeS3{objects: map[string][]byte{"backups/someone-else.txt": []byte("keep me")}}
	srv := httptest.NewServer(s3)
	defer srv.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "unrelated.db.gz"), []byte("not ours"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := &SettingService{}
	for k, v := range map[string]string{
		"webDomain": "panel.example.com", "backupKeep": "2", "backupLocalDir": dir,
		"backupS3Endpoint": srv.URL, "backupS3Region": "auto", "backupS3Bucket": "bucket",
		"backupS3AccessKey": "AKID", "backupS3SecretKey": "SECRET", "backupS3Prefix": "backups/",
	} {
		if err := settings.setString(k, v); err != nil {
			t.Fatal(err)
		}
	}
	backups := &ScheduledBackupService{}
	var last []BackupResult
	for i := 0; i < 3; i++ {
		if i > 0 {
			time.Sleep(1100 * time.Millisecond) // names carry the second
		}
		results, err := backups.Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		last = results
	}
	if len(last) != 2 || last[0].Error != "" || last[1].Error != "" || last[0].Pruned != 1 || last[1].Pruned != 1 {
		t.Fatalf("last run = %+v; want both destinations written and one old backup pruned from each", last)
	}

	local := dirNames(t, dir)
	if len(local) != 3 || local[2] != "unrelated.db.gz" || !strings.HasPrefix(local[0], "panel.example.com_") {
		t.Fatalf("local dir = %v; want the 2 newest backups and the foreign file untouched", local)
	}
	remote := s3.keys()
	if len(remote) != 3 || remote[2] != "backups/someone-else.txt" || remote[1] != "backups/"+last[1].Name {
		t.Fatalf("bucket = %v; want the 2 newest backups under the prefix and the foreign object", remote)
	}
	if !strings.Contains(s3.auth[0], "Credential=AKID/") || !strings.Contains(s3.auth[0], "/auto/s3/aws4_request") {
		t.Fatalf("requests were not signed for the configured key and region: %q", s3.auth[0])
	}

	raw, err := os.ReadFile(filepath.Join(dir, local[1]))
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	db, _ := io.ReadAll(zr)
	if !bytes.HasPrefix(db, []byte("SQLite format 3")) {
		t.Fatalf("backup does not unpack to a SQLite database: %q", db[:min(16, len(db))])
	}
}

func TestScheduledBackupWithoutDestinationIsAnError(t *testing.T) {
	setupConflictDB(t)
	if _, err := (&ScheduledBackupService{}).Run(context.Background()); err == nil || !strings.Contains(err.Error(), "no backup destination") {
		t.Fatalf("err = %v", err)
	}
}

func TestBackupSettingsAreValidatedOnSave(t *testing.T) {
	cases := []struct {
		dir, endpoint, wantErr string
	}{
		{"/var/backups/x-ui/", "https://acc.r2.cloudflarestorage.com", ""},
		{"backups", "", "absolute path"},
		{"", "ftp://example.org", "S3 endpoint is invalid"},
	}
	for _, c := range cases {
		s := &entity.AllSetting{BackupLocalDir: c.dir, BackupS3Endpoint: c.endpoint}
		err := validateSettingsURLs(s)
		if (c.wantErr == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), c.wantErr)) {
			t.Errorf("dir %q endpoint %q: err = %v, want %q", c.dir, c.endpoint, err, c.wantErr)
		}
		if err == nil && c.dir != "" && s.BackupLocalDir != filepath.Clean(c.dir) {
			t.Errorf("dir saved as %q", s.BackupLocalDir)
		}
	}
}
