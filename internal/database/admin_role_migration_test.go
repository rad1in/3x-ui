package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// A panel from before roles has one users row and no role column. It must come
// back as an owner with the same password hash, or the admin is locked out.
func TestInitDBMigratesLegacyAdminToOwner(t *testing.T) {
	t.Setenv("XUI_DB_JOURNAL_MODE", "")
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	const hash = "$2a$10$legacyhashlegacyhashlegacyhashlegacyhashlegacyhash12"
	if _, err := raw.Exec(`CREATE TABLE users (id integer primary key autoincrement, username text, password text, login_epoch integer default 0)`); err != nil {
		t.Fatalf("create legacy users: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO users(username, password, login_epoch) VALUES ('boss', ?, 3)`, hash); err != nil {
		t.Fatalf("seed legacy user: %v", err)
	}
	raw.Close()

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })

	var users []model.User
	if err := GetDB().Find(&users).Error; err != nil {
		t.Fatalf("read users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("users = %d, want the one legacy admin", len(users))
	}
	u := users[0]
	if u.Username != "boss" || u.Password != hash || u.LoginEpoch != 3 {
		t.Fatalf("legacy admin changed: %+v", u)
	}
	if u.Role != model.AdminRoleOwner {
		t.Fatalf("legacy admin role = %q, want %q", u.Role, model.AdminRoleOwner)
	}
	if u.ScopeInboundIds != "[]" || u.ScopeGroups != "[]" {
		t.Fatalf("legacy admin scope = %q/%q, want empty arrays", u.ScopeInboundIds, u.ScopeGroups)
	}
}
