package controller

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/util/backupcrypt"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func upload(data []byte) memoryFile { return memoryFile{bytes.NewReader(data)} }

func readAll(t *testing.T, f io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTheManualBackupDownloadIsEncryptedWhenAPassphraseIsSet(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	if err := (&service.SecurityConfigService{}).Save(&service.SecurityConfig{BackupPassphrase: "dl-pass"}); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "http://panel.example.com/panel/api/server/getDb", nil)
	(&ServerController{}).getDb(c)

	if cd := w.Header().Get("Content-Disposition"); !strings.HasSuffix(cd, ".db.enc") {
		t.Fatalf("Content-Disposition = %q, want an .enc file", cd)
	}
	plain, err := backupcrypt.Decrypt("dl-pass", w.Body.Bytes())
	if err != nil || !bytes.HasPrefix(plain, []byte("SQLite format 3")) {
		t.Fatalf("the download does not decrypt to a SQLite file: %v", err)
	}
}

func TestAPlainUploadPassesThroughUntouched(t *testing.T) {
	got, err := openBackupUpload(upload([]byte("SQLite format 3\x00rows")), "")
	if err != nil || readAll(t, got) != "SQLite format 3\x00rows" {
		t.Fatalf("plain upload changed: %v", err)
	}
	if got, err := openBackupUpload(upload([]byte("ab")), ""); err != nil || readAll(t, got) != "ab" {
		t.Fatalf("a file shorter than the marker changed: %v", err)
	}
}

func TestAnEncryptedUploadIsOpenedWithTheSuppliedPassphrase(t *testing.T) {
	blob, _ := backupcrypt.Encrypt("typed in", []byte("the database"))
	got, err := openBackupUpload(upload(blob), "typed in")
	if err != nil || readAll(t, got) != "the database" {
		t.Fatalf("decrypt: %v", err)
	}
	if _, err := openBackupUpload(upload(blob), "mistyped"); !errors.Is(err, backupcrypt.ErrWrongPassphrase) {
		t.Fatalf("err = %v, want ErrWrongPassphrase", err)
	}
}

func TestAnEncryptedUploadFallsBackToTheConfiguredPassphraseAndUngzips(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	if err := (&service.SecurityConfigService{}).Save(&service.SecurityConfig{BackupPassphrase: "configured"}); err != nil {
		t.Fatal(err)
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write([]byte("scheduled backup body"))
	_ = zw.Close()
	blob, _ := backupcrypt.Encrypt("configured", gz.Bytes())

	got, err := openBackupUpload(upload(blob), "")
	if err != nil || readAll(t, got) != "scheduled backup body" {
		t.Fatalf("restore of a scheduled encrypted backup: %v", err)
	}
}
