// Package dbtest opens throwaway panel databases for tests. Migrating a new
// SQLite file costs ~850ms under -race; copying a migrated template ~130ms.
package dbtest

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database"

	_ "github.com/go-sql-driver/mysql"
)

var migrated struct {
	once sync.Once
	data []byte
	err  error
}

// InitDB opens a new, fully migrated panel database at path and closes it when
// t ends. Reopen an existing file with database.InitDB instead.
func InitDB(t testing.TB, path string) {
	t.Helper()
	switch config.GetDBKind() {
	case "postgres":
	case "mysql":
		dropMySQLTables(t)
	default:
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("dbtest.InitDB would overwrite existing %s; reopen it with database.InitDB", path)
		}
		data, err := migratedTemplate()
		if err != nil {
			t.Fatalf("build template database: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("create database dir: %v", err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("copy template database: %v", err)
		}
	}
	if err := database.InitDB(path); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
}

func migratedTemplate() ([]byte, error) {
	migrated.once.Do(func() {
		dir, err := os.MkdirTemp("", "xui-dbtest-")
		if err != nil {
			migrated.err = err
			return
		}
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, "template.db")
		if err := database.InitDB(path); err != nil {
			migrated.err = err
			return
		}
		// Closing the last connection checkpoints the WAL into the main file.
		if err := database.CloseDB(); err != nil {
			migrated.err = err
			return
		}
		migrated.data, migrated.err = os.ReadFile(path)
	})
	return migrated.data, migrated.err
}

// dropMySQLTables empties the shared MySQL database so each test starts from a
// fresh schema, as the per-test SQLite file does.
func dropMySQLTables(t testing.TB) {
	t.Helper()
	cfg, err := database.ParseMySQLDSN(config.GetDBDSN())
	if err != nil {
		t.Fatalf("parse XUI_DB_DSN: %v", err)
	}
	conn, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	defer conn.Close()
	rows, err := conn.Query("SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE()")
	if err != nil {
		t.Fatalf("list MySQL tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan MySQL table name: %v", err)
		}
		tables = append(tables, "`"+name+"`")
	}
	rows.Close()
	if len(tables) == 0 {
		return
	}
	if _, err := conn.Exec("DROP TABLE IF EXISTS " + strings.Join(tables, ", ")); err != nil {
		t.Fatalf("drop MySQL tables: %v", err)
	}
}
