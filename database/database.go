package database

import (
	"log"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"goxcms/model"

	"github.com/spf13/viper"
)

// InitDB initializes and returns a *gorm.DB instance.
func InitDB() *gorm.DB {
	var db *gorm.DB
	var err error
	databaseDriver := viper.GetString("database.driver")

	config := &gorm.Config{
		// Foreign keys are enforced: deletes must clear dependents first
		// (see the transactional delete handlers), or the database refuses.
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
		dsn := viper.GetString("database.sqlite.dsn")
		db, err = gorm.Open(sqlite.Open(dsn), config)
	default:
		log.Fatalf("Unsupported database driver: %s", databaseDriver)
	}

	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
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
