package database

import (
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"goxcms/model"

	"github.com/spf13/viper"
)

const (
	// defaultSQLiteDSN matches the shipped config files, so a missing dsn
	// lands in a path .gitignore already covers rather than the repo root.
	defaultSQLiteDSN = "./data/database.sqlite"
	// sqliteMaxOpenConns is deliberately small: WAL allows concurrent readers
	// but only one writer, and every write is short.
	sqliteMaxOpenConns = 8
)

// InitDB initializes and returns a *gorm.DB instance.
// sqlitePragmas are appended to every SQLite DSN.
//
// foreign_keys is the important one: SQLite defaults it to OFF, and
// mattn/go-sqlite3 only issues the PRAGMA when the DSN asks for it, so without
// this the foreign keys GORM creates are inert. WAL lets readers run while a
// writer holds the write lock, and busy_timeout makes concurrent writers wait
// instead of failing with SQLITE_BUSY.
var sqlitePragmas = []string{
	"_foreign_keys=on",
	"_journal_mode=WAL",
	"_busy_timeout=5000",
	"_synchronous=NORMAL",
}

// SQLiteDSN appends the pragmas to a configured DSN, skipping any the operator
// already set so config/config.yaml stays authoritative. Exported for tests.
func SQLiteDSN(dsn string) string {
	return sqliteDSN(dsn)
}

// sqliteDSN appends the pragmas to a configured DSN, skipping any the operator
// already set so config/config.yaml stays authoritative.
func sqliteDSN(dsn string) string {
	if dsn == "" {
		dsn = defaultSQLiteDSN
	}
	if strings.HasPrefix(dsn, "file:") {
		// Already a URI: add to its query string.
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		for _, p := range sqlitePragmas {
			if strings.Contains(dsn, p) {
				continue
			}
			dsn += sep + p
			sep = "&"
		}
		return dsn
	}
	// Plain path: switch to the URI form, which is what the driver parses
	// the parameters out of.
	path := dsn
	if i := strings.IndexAny(path, "?"); i >= 0 {
		path = path[:i]
	}
	return "file:" + path + "?" + strings.Join(sqlitePragmas, "&")
}

// configureSQLite applies the pool limits and verifies that foreign keys are
// really on. With WAL a single writer is still serialised, but readers no
// longer block, so a small pool with idle reuse is the right shape: the
// previous unlimited default opened fresh connections per request and
// collided on the write lock.
func configureSQLite(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(sqliteMaxOpenConns)
	sqlDB.SetMaxIdleConns(sqliteMaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Hour)

	var fk int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&fk).Error; err != nil {
		return err
	}
	if fk != 1 {
		return fmt.Errorf("sqlite: foreign key enforcement is off (PRAGMA foreign_keys=%d); "+
			"check the database.sqlite.dsn value", fk)
	}
	return nil
}

func InitDB() *gorm.DB {
	var db *gorm.DB
	var err error
	databaseDriver := viper.GetString("database.driver")

	config := &gorm.Config{
		// This only makes GORM *create* the foreign keys. Whether they are
		// then enforced at runtime is driver-specific — see configureSQLite
		// below, which is what actually turns enforcement on for SQLite.
		DisableForeignKeyConstraintWhenMigrating: false,
		Logger:                                   logger.Default.LogMode(logger.Silent),
	}

	switch databaseDriver {
	case "mysql":
		dsn := viper.GetString("database.mysql.dsn")
		db, err = gorm.Open(mysql.Open(dsn), config)
	case "postgres":
		dsn := viper.GetString("database.postgres.dsn")
		db, err = gorm.Open(postgres.Open(dsn), config)
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(sqliteDSN(viper.GetString("database.sqlite.dsn"))), config)
	default:
		log.Fatalf("Unsupported database driver: %s", databaseDriver)
	}

	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if databaseDriver == "sqlite" {
		if err := configureSQLite(db); err != nil {
			log.Fatalf("Database configuration: %v", err)
		}
	}

	err = db.AutoMigrate(
		&model.User{},
		&model.Post{},
		&model.Category{},
		&model.Tag{},
		&model.Menu{},
		&model.MenuItem{},
		&model.BasicWebsiteInfo{},
		&model.CustomPage{},
		&model.File{},
		&model.Comment{},
		&model.Role{},
		&model.Plugin{},
	)

	if err != nil {
		log.Fatalf("Failed to auto migrate: %v", err)
	}

	return db
}
