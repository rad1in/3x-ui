package database

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// An upgraded panel keeps its nodes and gets the failover columns switched off,
// with the default delay ready for when an admin turns it on.
func TestNodeFailoverColumnsAreAddedWithDefaultsToLegacyNodes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	legacy, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	for _, ddl := range []string{
		"CREATE TABLE `nodes` (`id` integer PRIMARY KEY AUTOINCREMENT,`name` text,`address` text,`port` integer,`enable` numeric DEFAULT true)",
		"INSERT INTO nodes (name, address, port, enable) VALUES ('old-node', 'n.example', 2053, 1)",
	} {
		if err := legacy.Exec(ddl).Error; err != nil {
			t.Fatalf("legacy setup: %v", err)
		}
	}
	sqlDB, err := legacy.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB over legacy schema: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })

	var n model.Node
	if err := GetDB().Where("name = ?", "old-node").First(&n).Error; err != nil {
		t.Fatalf("legacy node lost or unreadable: %v", err)
	}
	if n.FailoverNodeId != 0 || n.FailoverAfterSec != 120 {
		t.Fatalf("failover = node %d after %ds, want off with a 120s delay", n.FailoverNodeId, n.FailoverAfterSec)
	}
}
