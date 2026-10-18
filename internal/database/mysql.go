package database

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

// mysqlTableOptions makes every panel table compare strings byte-wise, as
// SQLite and PostgreSQL do; MySQL's default _ci collations fold case.
const mysqlTableOptions = "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin"

// mysqlIndexedStringSize fits any RFC 5321 email (254) and keeps two-column
// utf8mb4 keys under InnoDB's 3072-byte limit (DYNAMIC rows, MySQL 5.7+).
const mysqlIndexedStringSize = 255

// IsMySQLDSN reports whether dsn targets MySQL/MariaDB: a mysql:// or
// mariadb:// URL, or the driver's native user:pass@tcp(host:port)/db form.
func IsMySQLDSN(dsn string) bool {
	dsn = strings.TrimSpace(dsn)
	lower := strings.ToLower(dsn)
	if strings.HasPrefix(lower, "mysql://") || strings.HasPrefix(lower, "mariadb://") {
		return true
	}
	if strings.Contains(lower, "://") {
		return false
	}
	return strings.Contains(dsn, "@tcp(") || strings.Contains(dsn, "@unix(") || strings.HasPrefix(dsn, "tcp(")
}

// ParseMySQLDSN accepts a mysql:// / mariadb:// URL or a native driver DSN and
// returns the driver config with the session settings the panel's SQL relies on.
func ParseMySQLDSN(dsn string) (*mysqldriver.Config, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errors.New("MySQL DSN is empty")
	}
	native := dsn
	lower := strings.ToLower(dsn)
	if strings.HasPrefix(lower, "mysql://") || strings.HasPrefix(lower, "mariadb://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return nil, fmt.Errorf("parse MySQL DSN: %w", err)
		}
		host := u.Hostname()
		if host == "" {
			host = "127.0.0.1"
		}
		port := u.Port()
		if port == "" {
			port = "3306"
		}
		var userinfo string
		if u.User != nil {
			userinfo = u.User.Username()
			if pass, ok := u.User.Password(); ok {
				userinfo += ":" + pass
			}
			userinfo += "@"
		}
		native = userinfo + "tcp(" + net.JoinHostPort(host, port) + ")/" + strings.TrimPrefix(u.Path, "/")
		if u.RawQuery != "" {
			native += "?" + u.RawQuery
		}
	}
	cfg, err := mysqldriver.ParseDSN(native)
	if err != nil {
		return nil, fmt.Errorf("parse MySQL DSN: %w", err)
	}
	if cfg.DBName == "" {
		return nil, errors.New("MySQL DSN is missing a database name")
	}
	cfg.Collation = "utf8mb4_bin"
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	delete(cfg.Params, "charset")
	// Shared raw SQL quotes identifiers ("key") and treats backslash as a plain
	// character, as SQLite and PostgreSQL do; MySQL needs both modes for that.
	cfg.Params["sql_mode"] = "CONCAT(@@sql_mode, ',ANSI_QUOTES,NO_BACKSLASH_ESCAPES')"
	return cfg, nil
}

// mysqlDialector sizes indexed string columns as VARCHAR: upstream only does so
// for `index`/`unique` tags, so a `uniqueIndex` string became an unindexable LONGTEXT.
type mysqlDialector struct {
	*mysql.Dialector
}

func newMySQLDialector(cfg *mysqldriver.Config) gorm.Dialector {
	return mysqlDialector{Dialector: mysql.New(mysql.Config{DSNConfig: cfg}).(*mysql.Dialector)}
}

func (d mysqlDialector) DataTypeOf(field *schema.Field) string {
	if field.DataType == schema.String && field.Size == 0 && mysqlStringIsIndexed(field) {
		return fmt.Sprintf("varchar(%d)", mysqlIndexedStringSize)
	}
	// MySQL TEXT stops at 64 KiB; SQLite/PostgreSQL text is unbounded.
	if strings.EqualFold(string(field.DataType), "text") {
		return "longtext"
	}
	return d.Dialector.DataTypeOf(field)
}

func (d mysqlDialector) Migrator(db *gorm.DB) gorm.Migrator {
	return mysqlMigrator{mysql.Migrator{
		Migrator:  migrator.Migrator{Config: migrator.Config{DB: db, Dialector: d}},
		Dialector: *d.Dialector,
	}}
}

type mysqlMigrator struct {
	mysql.Migrator
}

// FullDataTypeOf writes a TEXT column's default as an expression: MySQL 8
// rejects a literal DEFAULT on TEXT/BLOB (error 1101), MariaDB takes either.
func (m mysqlMigrator) FullDataTypeOf(field *schema.Field) clause.Expr {
	expr := m.Migrator.FullDataTypeOf(field)
	if !strings.HasSuffix(strings.ToLower(m.Migrator.Migrator.DataTypeOf(field)), "text") {
		return expr
	}
	if i := strings.LastIndex(expr.SQL, " DEFAULT "); i >= 0 {
		expr.SQL = expr.SQL[:i] + " DEFAULT (" + expr.SQL[i+len(" DEFAULT "):] + ")"
	}
	return expr
}

// AlterColumn is mysql.Migrator's, routed through the FullDataTypeOf above;
// MySQL 8 reports an expression default differently, so AutoMigrate re-alters it.
func (m mysqlMigrator) AlterColumn(value any, name string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if stmt.Schema == nil {
			return fmt.Errorf("failed to look up field with name: %s", name)
		}
		field := stmt.Schema.LookUpField(name)
		if field == nil {
			return fmt.Errorf("failed to look up field with name: %s", name)
		}
		return m.DB.Exec("ALTER TABLE ? MODIFY COLUMN ? ?",
			m.CurrentTable(stmt), clause.Column{Name: field.DBName}, m.FullDataTypeOf(field)).Error
	})
}

func mysqlStringIsIndexed(field *schema.Field) bool {
	for _, tag := range []string{"INDEX", "UNIQUE", "UNIQUEINDEX"} {
		if _, ok := field.TagSettings[tag]; ok {
			return true
		}
	}
	return field.PrimaryKey
}

func isMariaDB() bool {
	d, ok := db.Dialector.(mysqlDialector)
	return ok && strings.Contains(d.ServerVersion, "MariaDB")
}
