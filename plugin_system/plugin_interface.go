package plugin_system

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

//// import db gorm etc

type Plugin interface {
	Setup(app *fiber.App, db *gorm.DB) error
	Teardown() error
	Name() string
}
