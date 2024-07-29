package database

import (
	"log"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/dbresolver"

	"goxcms/model" // Update with your actual project path

	"github.com/spf13/viper"
)

// initDB initializes and returns a *gorm.DB instance.
func InitDB() *gorm.DB {
	var db *gorm.DB
	var err error
	databaseDriver := viper.GetString("database.driver")

	config := &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
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

	db.Use(dbresolver.Register(dbresolver.Config{
		Replicas: []gorm.Dialector{db.Dialector},
		Policy:   dbresolver.RandomPolicy{},
	}))

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

	addForeignKeyConstraints(db)

	return db
}

func addForeignKeyConstraints(db *gorm.DB) {
	constraints := []struct {
		Table1  string
		Column1 string
		Table2  string
		Column2 string
	}{
		{"Post", "CategoryID", "Category", "ID"},
		{"Post", "TagID", "Tag", "ID"},
		{"Post", "UserID", "User", "ID"},
		{"Post", "MenuID", "Menu", "ID"},
		{"Post", "MenuItemID", "MenuItem", "ID"},
		{"Post", "CustomPageID", "CustomPage", "ID"},
		{"Category", "PostID", "Post", "ID"},
		{"Category", "TagID", "Tag", "ID"},
		{"Comment", "UserID", "User", "ID"},
		{"Comment", "PostID", "Post", "ID"},
		{"Tag", "PostID", "Post", "ID"},
		{"Menu", "MenuItemID", "MenuItem", "ID"},
		{"MenuItem", "MenuID", "Menu", "ID"},
		{"CustomPage", "PostID", "Post", "ID"},
		{"File", "PostID", "Post", "ID"},
		{"BasicWebsiteInfo", "PostID", "Post", "ID"},
		{"User", "RoleID", "Role", "ID"},
		{"User", "PostID", "Post", "ID"},
		{"User", "CommentID", "Comment", "ID"},
		{"Role", "UserID", "User", "ID"},
	}

	for _, constraint := range constraints {
		err := db.Migrator().CreateConstraint(constraint.Table1, constraint.Column1)
		if err != nil {
			log.Printf("Error creating constraint %s.%s: %v", constraint.Table1, constraint.Column1, err)
		}
		err = db.Migrator().CreateConstraint(constraint.Table2, constraint.Column2)
		if err != nil {
			log.Printf("Error creating constraint %s.%s: %v", constraint.Table2, constraint.Column2, err)
		}
	}
}
