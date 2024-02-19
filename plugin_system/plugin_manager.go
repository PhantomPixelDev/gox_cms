package plugin_system

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

var plugins []Plugin

func RegisterPlugin(plugin Plugin) {
	plugins = append(plugins, plugin)
}

func InitializePlugins(app *fiber.App, db *gorm.DB) {
	for _, plugin := range plugins {
		if err := plugin.Setup(app, db); err != nil {
			log.Fatalf("Error setting up plugin %s: %v", plugin.Name(), err)
		}
	}
}

func TeardownPlugins() {
	for _, plugin := range plugins {
		if err := plugin.Teardown(); err != nil {
			log.Printf("Error tearing down plugin %s: %v", plugin.Name(), err)
		}
	}
}
