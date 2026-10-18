package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestParseMySQLDSN(t *testing.T) {
	cases := []struct {
		name, dsn                 string
		user, pass, addr, db, tls string
	}{
		{"url", "mysql://xui:s3cret@db.example:3307/panel?tls=skip-verify", "xui", "s3cret", "db.example:3307", "panel", "skip-verify"},
		{"url default port", "mariadb://xui:p@10.0.0.5/panel", "xui", "p", "10.0.0.5:3306", "panel", ""},
		{"url escaped password", "mysql://xui:p%40ss%2Fw@h:3306/panel", "xui", "p@ss/w", "h:3306", "panel", ""},
		{"url ipv6", "mysql://xui:p@[::1]:3306/panel", "xui", "p", "[::1]:3306", "panel", ""},
		{"native", "xui:p@tcp(127.0.0.1:3306)/panel?tls=false", "xui", "p", "127.0.0.1:3306", "panel", "false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := ParseMySQLDSN(tc.dsn)
			if err != nil {
				t.Fatalf("ParseMySQLDSN(%q): %v", tc.dsn, err)
			}
			if cfg.User != tc.user || cfg.Passwd != tc.pass || cfg.Addr != tc.addr || cfg.DBName != tc.db || cfg.TLSConfig != tc.tls {
				t.Fatalf("got user=%q pass=%q addr=%q db=%q tls=%q", cfg.User, cfg.Passwd, cfg.Addr, cfg.DBName, cfg.TLSConfig)
			}
			if cfg.Collation != "utf8mb4_bin" {
				t.Fatalf("collation = %q, want utf8mb4_bin", cfg.Collation)
			}
			if mode := cfg.Params["sql_mode"]; !strings.Contains(mode, "ANSI_QUOTES") || !strings.Contains(mode, "NO_BACKSLASH_ESCAPES") {
				t.Fatalf("sql_mode = %q, want ANSI_QUOTES and NO_BACKSLASH_ESCAPES", mode)
			}
		})
	}
	if _, err := ParseMySQLDSN("mysql://xui:p@h:3306/"); err == nil || !strings.Contains(err.Error(), "database name") {
		t.Fatalf("DSN without a database: err = %v, want missing database name", err)
	}
}

func TestDSNKind(t *testing.T) {
	cases := map[string]string{
		"postgres://u:p@h:5432/db?sslmode=disable":          DialectPostgres,
		"host=127.0.0.1 user=u password=p dbname=db":        DialectPostgres,
		"mysql://u:p@h:3306/db":                             DialectMySQL,
		"MariaDB://u:p@h/db":                                DialectMySQL,
		"u:p@tcp(127.0.0.1:3306)/db?parseTime=true":         DialectMySQL,
		"u:p@unix(/run/mysqld/mysqld.sock)/db":              DialectMySQL,
		"postgresql://u:p@tcp-proxy.example:5432/db?x=tcp(": DialectPostgres,
	}
	for dsn, want := range cases {
		if got := DSNKind(dsn); got != want {
			t.Errorf("DSNKind(%q) = %q, want %q", dsn, got, want)
		}
	}
}

func mysqlTestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("XUI_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set XUI_TEST_MYSQL_DSN to a reachable MySQL/MariaDB to run this test")
	}
	return dsn
}

func dropMySQLPanelTables(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	conn, err := openServerDB(DialectMySQL, dsn)
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := conn.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := conn.Migrator().DropTable(migrationModels()...); err != nil {
		t.Fatalf("drop tables: %v", err)
	}
	return conn
}

func seedMigrationSource(t *testing.T, seed func(src *gorm.DB)) string {
	t.Helper()
	srcPath := filepath.Join(t.TempDir(), "x-ui.db")
	src, err := gorm.Open(sqlite.Open(srcPath), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, m := range migrationModels() {
		if err := src.AutoMigrate(m); err != nil {
			t.Fatalf("automigrate %T: %v", m, err)
		}
	}
	seed(src)
	if sqlDB, err := src.DB(); err == nil {
		_ = sqlDB.Close()
	}
	return srcPath
}

// A MySQL TEXT column stops at 64 KiB and a _ci collation folds case; either
// would corrupt a SQLite panel's data on its way into MySQL.
func TestMigrateData_MySQLRoundTrip(t *testing.T) {
	dsn := mysqlTestDSN(t)
	dst := dropMySQLPanelTables(t, dsn)

	bigOutbounds := strings.Repeat("x", 100*1024)
	srcPath := seedMigrationSource(t, func(src *gorm.DB) {
		if err := src.Create(&model.User{Username: "keep", Password: "pw"}).Error; err != nil {
			t.Fatalf("seed user: %v", err)
		}
		links := make([]model.ClientInbound, 0, 600)
		for i := 1; i <= 600; i++ {
			links = append(links, model.ClientInbound{ClientId: i, InboundId: 1})
		}
		if err := src.CreateInBatches(links, 200).Error; err != nil {
			t.Fatalf("seed client_inbounds: %v", err)
		}
		if err := src.Create([]*model.ClientRecord{{Email: "on@example.com"}, {Email: "off@example.com"}}).Error; err != nil {
			t.Fatalf("seed clients: %v", err)
		}
		if err := src.Model(&model.ClientRecord{}).Where("email = ?", "off@example.com").Update("enable", false).Error; err != nil {
			t.Fatalf("disable client: %v", err)
		}
		if err := src.Create(&model.OutboundSubscription{Remark: "big", Url: "https://example.com/s", LastFetchedOutbounds: bigOutbounds}).Error; err != nil {
			t.Fatalf("seed subscription: %v", err)
		}
	})

	if err := MigrateData(srcPath, dsn); err != nil {
		t.Fatalf("MigrateData: %v", err)
	}

	var links int64
	if err := dst.Model(&model.ClientInbound{}).Count(&links).Error; err != nil || links != 600 {
		t.Fatalf("client_inbounds = %d (err %v), want 600", links, err)
	}
	var off model.ClientRecord
	if err := dst.Where("email = ?", "off@example.com").First(&off).Error; err != nil || off.Enable {
		t.Fatalf("disabled client after migration: enable=%v err=%v", off.Enable, err)
	}
	var sub model.OutboundSubscription
	if err := dst.Where("remark = ?", "big").First(&sub).Error; err != nil || sub.LastFetchedOutbounds != bigOutbounds {
		t.Fatalf("100 KiB lastFetchedOutbounds came back %d bytes (err %v)", len(sub.LastFetchedOutbounds), err)
	}
	var folded int64
	if err := dst.Model(&model.User{}).Where("username = ?", "KEEP").Count(&folded).Error; err != nil || folded != 0 {
		t.Fatalf("username lookup folded case: %d rows (err %v), want 0", folded, err)
	}
	next := model.User{Username: "after", Password: "pw"}
	if err := dst.Create(&next).Error; err != nil || next.Id <= 1 {
		t.Fatalf("insert after copy: id=%d err=%v, want an id past the copied rows", next.Id, err)
	}

	exportPath := filepath.Join(t.TempDir(), "export.db")
	if err := ExportServerDBToSQLite(DialectMySQL, dsn, exportPath); err != nil {
		t.Fatalf("ExportServerDBToSQLite: %v", err)
	}
	exported, err := gorm.Open(sqlite.Open(exportPath), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer func() {
		if sqlDB, err := exported.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	var users int64
	if err := exported.Model(&model.User{}).Count(&users).Error; err != nil || users != 2 {
		t.Fatalf("exported users = %d (err %v), want 2", users, err)
	}
}

// The MySQL clear runs as DELETEs inside the copy transaction (TRUNCATE would
// commit implicitly), so a failed copy must leave the previous rows in place.
func TestMigrateData_MySQLFailedCopyLeavesDestinationUntouched(t *testing.T) {
	dsn := mysqlTestDSN(t)
	dst := dropMySQLPanelTables(t, dsn)

	seedUser := func(username string) func(*gorm.DB) {
		return func(src *gorm.DB) {
			if err := src.Create(&model.User{Username: username, Password: "pw"}).Error; err != nil {
				t.Fatalf("seed user: %v", err)
			}
		}
	}
	if err := MigrateData(seedMigrationSource(t, seedUser("keep-me")), dsn); err != nil {
		t.Fatalf("seed destination: %v", err)
	}
	broken := seedMigrationSource(t, func(src *gorm.DB) {
		seedUser("evil")(src)
		if err := src.Exec("DROP TABLE outbound_subscriptions").Error; err != nil {
			t.Fatalf("break source: %v", err)
		}
	})
	if err := MigrateData(broken, dsn); err == nil || !strings.Contains(err.Error(), "OutboundSubscription") {
		t.Fatalf("MigrateData on a source missing outbound_subscriptions: err = %v, want a copy error naming it", err)
	}

	var names []string
	if err := dst.Model(&model.User{}).Order("id").Pluck("username", &names).Error; err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(names) != 1 || names[0] != "keep-me" {
		t.Fatalf("users after failed migration = %v, want [keep-me]", names)
	}
}

// Every panel start re-runs initModels; on MySQL the second start takes the
// settled fast path and must not re-issue DDL that fails on existing objects.
func TestInitDB_MySQLRestartsOnExistingSchema(t *testing.T) {
	dsn := mysqlTestDSN(t)
	dropMySQLPanelTables(t, dsn)
	t.Setenv("XUI_DB_TYPE", "mysql")
	t.Setenv("XUI_DB_DSN", dsn)

	for start := 1; start <= 2; start++ {
		if err := InitDB(filepath.Join(t.TempDir(), "unused.db")); err != nil {
			t.Fatalf("InitDB start %d: %v", start, err)
		}
		if err := CloseDB(); err != nil {
			t.Fatalf("CloseDB: %v", err)
		}
	}
	if err := InitDB(filepath.Join(t.TempDir(), "unused.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	var users int64
	if err := db.Model(&model.User{}).Count(&users).Error; err != nil || users != 1 {
		t.Fatalf("users after three starts = %d (err %v), want the single seeded admin", users, err)
	}
	if !isMariaDB() && !db.Migrator().HasIndex(&model.ClientRecord{}, "idx_clients_email_lower") {
		t.Fatal("MySQL 8 lost the LOWER(email) functional index")
	}
}
