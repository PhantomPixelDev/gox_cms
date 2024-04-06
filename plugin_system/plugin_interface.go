package plugin_system

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
	"gorm.io/gorm"
)

//// import db gorm etc

type Plugin interface {
	Setup(app *fiber.App, db *gorm.DB, engine *html.Engine) error
	Teardown() error
	Name() string
	Author() string
	Version() string
	Settings(db *gorm.DB) map[string]string
	DefaultSettings() map[string]string
	Enabled(db *gorm.DB) bool
}
