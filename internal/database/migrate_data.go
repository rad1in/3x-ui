package database

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"reflect"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// migrationModels is the FK-aware order in which tables are created and copied
// during `x-ui migrate-db --dsn` (SQLite → PostgreSQL/MySQL data migration) and
// in related tests.
//
// Important: When adding a new top-level model (like OutboundSubscription),
// you must add it here **in addition to** allModels() in internal/database/db.go;
// TestMigrationModelsMatchPanelModels fails when the two lists drift apart.
// This list is used for:
//   - Creating the destination schema during cross-DB migration
//   - Truncating tables
//   - Copying data row-by-row
//   - Resyncing Postgres sequences after bulk insert
//
// DumpSQLite / RestoreSQLite are schema-introspective (they read sqlite_master)
// so they do not need manual updates.
func migrationModels() []any {
	return []any{
		&model.User{},
		&model.Setting{},
		&model.HistoryOfSeeders{},
		&model.Node{},
		&model.ApiToken{},
		&model.Inbound{},
		&xray.ClientTraffic{},
		&model.OutboundTraffics{},
		&model.InboundClientIps{},
		&model.ClientRecord{},
		&model.ClientInbound{},
		&model.ClientHwid{},
		&model.SubShortLink{},
		&model.SubFetch{},
		&model.ClientExternalLink{},
		&model.ClientGroup{},
		&model.InboundFallback{},
		&model.Host{},
		&model.NodeClientTraffic{},
		&model.NodeClientIp{},
		&model.ClientGlobalTraffic{},
		&model.NodePendingReset{},
		&model.OutboundSubscription{},
		&model.SubBalancer{},
		&model.TuicTrafficReceipt{},
		&model.AuditLog{},
		&model.AdminSession{},
		&model.SponsorRecord{},
		&model.HostedPanel{},
		&model.ClientWarning{},
		&model.ClientDepletion{},
		&model.ClientTemplate{},
		&model.ClientDailyUsage{},
		&model.ClientTrial{},
		&model.Plan{},
		&model.Order{},
		&model.DiscountCode{},
		&model.WalletEntry{},
		&model.ReferralCode{},
	}
}

// MigrateData is MigrateDataTo with the destination kind inferred from dstDSN,
// for `x-ui migrate-db --dsn`.
func MigrateData(srcPath, dstDSN string) error {
	return MigrateDataTo(srcPath, DSNKind(dstDSN), dstDSN)
}

// DSNKind names the server database a DSN targets: DialectMySQL for a mysql://
// / mariadb:// URL or a native MySQL DSN, otherwise DialectPostgres.
func DSNKind(dsn string) string {
	if IsMySQLDSN(dsn) {
		return DialectMySQL
	}
	return DialectPostgres
}

// openServerDB opens a PostgreSQL or MySQL pool for a data copy, configured
// the way InitDB configures the panel's own connection.
func openServerDB(kind, dsn string) (*gorm.DB, error) {
	switch kind {
	case DialectPostgres:
		conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
		if err != nil {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
		return conn, nil
	case DialectMySQL:
		cfg, err := ParseMySQLDSN(dsn)
		if err != nil {
			return nil, err
		}
		conn, err := gorm.Open(newMySQLDialector(cfg), &gorm.Config{Logger: logger.Discard, DisableForeignKeyConstraintWhenMigrating: true})
		if err != nil {
			return nil, fmt.Errorf("open mysql: %w", err)
		}
		return conn.Set("gorm:table_options", mysqlTableOptions).Session(&gorm.Session{}), nil
	default:
		return nil, fmt.Errorf("unsupported destination database %q", kind)
	}
}

// MigrateDataTo copies the SQLite file at srcPath into a kind (postgres/mysql)
// database; clear and copy share one transaction, so a failure changes nothing.
func MigrateDataTo(srcPath, kind, dstDSN string) error {
	if _, err := os.Stat(srcPath); err != nil {
		return fmt.Errorf("source sqlite not found at %s: %w", srcPath, err)
	}
	if dstDSN == "" {
		return errors.New("destination DSN is required")
	}

	if err := os.MkdirAll(path.Dir(srcPath), 0o755); err != nil {
		return err
	}

	srcDSN := srcPath + "?_journal_mode=WAL&_busy_timeout=10000"
	src, err := gorm.Open(sqlite.Open(srcDSN), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return fmt.Errorf("open sqlite source: %w", err)
	}
	srcSQL, err := src.DB()
	if err != nil {
		return err
	}
	defer srcSQL.Close()

	dst, err := openServerDB(kind, dstDSN)
	if err != nil {
		return fmt.Errorf("destination: %w", err)
	}
	dstSQL, err := dst.DB()
	if err != nil {
		return err
	}
	defer dstSQL.Close()
	dstSQL.SetConnMaxLifetime(time.Hour)

	log.Println("Creating destination schema...")
	for _, m := range migrationModels() {
		if err := dst.AutoMigrate(m); err != nil {
			return fmt.Errorf("AutoMigrate %T: %w", m, err)
		}
	}

	totalRows := 0
	txErr := dst.Transaction(func(tx *gorm.DB) error {
		if kind == DialectPostgres {
			// AutoMigrate re-creates the legacy client_traffics -> inbounds foreign key,
			// but the running panel drops it (see dropLegacyForeignKeys) and tolerates
			// client_traffics rows whose inbound was deleted. Drop it here too so copying
			// such orphaned rows can't fail with an fk_inbounds_client_stats violation.
			if err := tx.Exec("ALTER TABLE client_traffics DROP CONSTRAINT IF EXISTS fk_inbounds_client_stats").Error; err != nil {
				return fmt.Errorf("drop legacy foreign key: %w", err)
			}
		}

		// A database a panel already started on holds a seeded admin (id=1) that
		// the copied explicit ids would collide with; only panel tables are cleared.
		if err := clearServerTables(tx, kind, migrationModels()); err != nil {
			return fmt.Errorf("clear destination tables: %w", err)
		}

		for _, m := range migrationModels() {
			n, err := copyTable(src, tx, m)
			if err != nil {
				return fmt.Errorf("copy %T: %w", m, err)
			}
			totalRows += n
			log.Printf("  %-32s %d rows", reflect.TypeOf(m).Elem().Name(), n)
		}
		return nil
	})
	if txErr != nil {
		return txErr
	}

	// setval is never rolled back by PostgreSQL, so sequences are resynced only
	// after the transaction has committed. InnoDB advances AUTO_INCREMENT itself.
	if kind == DialectPostgres {
		if err := resetPostgresSequences(dst); err != nil {
			log.Printf("warning: failed to reset some postgres sequences: %v", err)
		}
	}

	log.Printf("Migration complete: %d rows across %d tables.", totalRows, len(migrationModels()))
	log.Printf("Set XUI_DB_TYPE=%s and XUI_DB_DSN=... in /etc/default/x-ui, then restart x-ui.", kind)
	return nil
}

// ExportServerDBToSQLite is the reverse of MigrateDataTo: it writes a kind
// database into a fresh portable SQLite file at dstPath (overwritten).
func ExportServerDBToSQLite(kind, srcDSN, dstPath string) error {
	if srcDSN == "" {
		return errors.New("source DSN is required")
	}
	if err := os.MkdirAll(path.Dir(dstPath), 0o755); err != nil {
		return err
	}
	// Start from an empty file so AutoMigrate creates the canonical schema.
	if err := os.Remove(dstPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	src, err := openServerDB(kind, srcDSN)
	if err != nil {
		return fmt.Errorf("source: %w", err)
	}
	srcSQL, err := src.DB()
	if err != nil {
		return err
	}
	defer srcSQL.Close()

	// No WAL: keep all data in the main file so it is complete once closed.
	dst, err := gorm.Open(sqlite.Open(dstPath+"?_busy_timeout=10000"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return fmt.Errorf("open sqlite destination: %w", err)
	}
	dstSQL, err := dst.DB()
	if err != nil {
		return err
	}
	defer dstSQL.Close()

	return copyAllModels(src, dst)
}

// copyAllModels (re)creates the schema on dst and copies every migrated table
// from src to dst in FK-safe order. src/dst may be any gorm backend.
func copyAllModels(src, dst *gorm.DB) error {
	for _, m := range migrationModels() {
		if err := dst.AutoMigrate(m); err != nil {
			return fmt.Errorf("AutoMigrate %T: %w", m, err)
		}
	}
	for _, m := range migrationModels() {
		if _, err := copyTable(src, dst, m); err != nil {
			return fmt.Errorf("copy %T: %w", m, err)
		}
	}
	return nil
}

func copyTable(src, dst *gorm.DB, mdl any) (int, error) {
	const batchSize = 500

	sliceType := reflect.SliceOf(reflect.PointerTo(reflect.TypeOf(mdl).Elem()))

	stmt := &gorm.Statement{DB: src}
	if err := stmt.Parse(mdl); err != nil {
		return 0, err
	}
	order := strings.Join(stmt.Schema.PrimaryFieldDBNames, ", ")
	table := stmt.Schema.Table
	columns := stmt.Schema.DBNames

	ctx := context.Background()
	total := 0
	for offset := 0; ; offset += batchSize {
		batchPtr := reflect.New(sliceType)
		q := src.Model(mdl).Limit(batchSize).Offset(offset)
		if order != "" {
			q = q.Order(order)
		}
		if err := q.Find(batchPtr.Interface()).Error; err != nil {
			return total, err
		}
		slice := batchPtr.Elem()
		n := slice.Len()
		if n == 0 {
			break
		}

		rows := make([]map[string]any, n)
		for i := range n {
			rv := reflect.Indirect(slice.Index(i))
			row := make(map[string]any, len(columns))
			for _, name := range columns {
				value, _ := stmt.Schema.FieldsByDBName[name].ValueOf(ctx, rv)
				row[name] = value
			}
			rows[i] = row
		}

		if err := dst.Table(table).CreateInBatches(rows, 200).Error; err != nil {
			return total, err
		}
		total += n
		if n < batchSize {
			break
		}
	}
	return total, nil
}

// clearServerTables empties the migrated tables inside the caller's transaction;
// MySQL deletes instead of truncating because its TRUNCATE commits implicitly.
func clearServerTables(dst *gorm.DB, kind string, models []any) error {
	tables := make([]string, 0, len(models))
	for _, m := range models {
		stmt := &gorm.Statement{DB: dst}
		if err := stmt.Parse(m); err != nil {
			return err
		}
		tables = append(tables, `"`+stmt.Schema.Table+`"`)
	}
	if len(tables) == 0 {
		return nil
	}
	log.Println("Clearing destination tables...")
	if kind == DialectMySQL {
		for _, table := range tables {
			if err := dst.Exec("DELETE FROM " + table).Error; err != nil {
				return err
			}
		}
		return nil
	}
	return dst.Exec("TRUNCATE TABLE " + strings.Join(tables, ", ") + " RESTART IDENTITY CASCADE").Error
}

// resetPostgresSequences advances each migrated table's id sequence past MAX(id),
// otherwise the next INSERT-without-id would clash with copied rows.
func resetPostgresSequences(dst *gorm.DB) error {
	return resyncPostgresSequences(dst, migrationModels())
}

// resyncPostgresSequences sets each model's id sequence to MAX(id); idempotent. Id-less
// composite-PK tables are skipped — Postgres rejects MAX(id) at parse time and logs it (#5665).
func resyncPostgresSequences(db *gorm.DB, models []any) error {
	for _, m := range models {
		t, ok := tableWithIdColumn(db, m)
		if !ok {
			continue
		}
		// t comes from the trusted model set parsed by GORM, not user input, so
		// interpolating it as an identifier is safe. We ignore errors per-table.
		_ = db.Exec(
			`SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM "`+t+`"), 1), true)
			 WHERE pg_get_serial_sequence(?, 'id') IS NOT NULL`,
			t, t,
		).Error
	}
	return nil
}

// tableWithIdColumn resolves a model's table name and reports whether its GORM
// schema maps an "id" database column.
func tableWithIdColumn(db *gorm.DB, m any) (string, bool) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil {
		return "", false
	}
	if stmt.Schema == nil || stmt.Schema.LookUpField("id") == nil {
		return "", false
	}
	return stmt.Table, true
}

// PrepareSQLiteForMigration rejects SQLite files that are not a panel database
// before the caller causes any downtime, then AutoMigrates the panel schema
// onto the file so backups from older versions gain the newer tables and
// columns the row copy reads. Data-level upgrades are not needed here: they
// run dialect-agnostically on the destination via InitDB after the import.
func PrepareSQLiteForMigration(dbPath string) error {
	gdb, err := gorm.Open(sqlite.Open(dbPath+"?_busy_timeout=10000"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	for _, table := range []string{"users", "settings", "inbounds"} {
		if !sqliteTableExists(sqlDB, table) {
			return fmt.Errorf("not a 3x-ui panel database: required table %q is missing", table)
		}
	}
	for _, m := range migrationModels() {
		if err := gdb.AutoMigrate(m); err != nil && !isIgnorableDuplicateColumnErr(gdb, err, m) {
			return fmt.Errorf("upgrade panel schema for %T: %w", m, err)
		}
	}
	return nil
}
