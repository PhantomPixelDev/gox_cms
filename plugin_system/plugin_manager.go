package plugin_system

import (
	"encoding/json"
	"errors"
	handlers "goxcms/handler"
	"goxcms/model"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
	"gorm.io/gorm"
)

var plugins []Plugin

func RegisterPlugin(plugin Plugin, db *gorm.DB) {
	/// add plugin to Database if not exists already and then add to plugins array if not exists already
	pluginDB := model.Plugin{}
	db.Where("name = ?", plugin.Name()).First(&pluginDB)
	if pluginDB.ID == 0 {
		default_settings := plugin.DefaultSettings()
		converted_settings := make(map[string]string)
		for key, value := range default_settings {
			converted_settings[key] = value
		}

		settingsJSON, err := json.Marshal(converted_settings)
		if err != nil {
			log.Printf("Error marshaling settings: %v", err)
			return
		}

		if err := db.Create(&model.Plugin{Name: plugin.Name(), Author: plugin.Author(), Version: plugin.Version(), Enabled: plugin.Enabled(db), Settings: string(settingsJSON)}).Error; err != nil {
			if !isDupPlugin(err) {
				log.Printf("Error registering plugin %s: %v", plugin.Name(), err)
			}
		}
	}

	for _, p := range plugins {
		if p.Name() == plugin.Name() {
			return
		}
	}

	plugins = append(plugins, plugin)
}

func InitializePlugins(app *fiber.App, db *gorm.DB, engine *html.Engine) {
	for _, plugin := range plugins {
		/// Initialize plugin that are in the database and are enabled only
		pluginDB := model.Plugin{}
		db.Where("name = ?", plugin.Name()).First(&pluginDB)
		if pluginDB.Enabled {
			if err := plugin.Setup(app, db, engine); err != nil {
				log.Printf("Error setting up plugin %s: %v", plugin.Name(), err)
			}
		}
	}
}

func GetPlugins() []Plugin {
	/// get plugins from the database and return them
	return plugins
}

func TeardownPlugins() {
	for _, plugin := range plugins {
		if err := plugin.Teardown(); err != nil {
			log.Printf("Error tearing down plugin %s: %v", plugin.Name(), err)
		}
	}
}

func GetPluginByName(pluginName string) Plugin {
	for _, plugin := range plugins {
		if plugin.Name() == pluginName {
			return plugin
		}
	}
	return nil
}

// errPluginNotFound is returned when toggling a plugin with no database row.
var errPluginNotFound = errors.New("plugin not found")

func isDupPlugin(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "Duplicate entry")
}

func EnableDisablePlugin(pluginName string, db *gorm.DB) error {
	pluginDB := model.Plugin{}
	db.Where("name = ?", pluginName).First(&pluginDB)
	if pluginDB.ID == 0 {
		return errPluginNotFound
	}
	/// change value in database and then reload the plugin if enabled
	pluginDB.Enabled = !pluginDB.Enabled
	return db.Save(&pluginDB).Error
}

// AddPluginManagerRoutes registers the admin-only plugin toggle endpoint. It is
// a POST because it changes state.
func AddPluginManagerRoutes(app *fiber.App, db *gorm.DB) {
	app.Post("/admin/plugins/enable/:name", handlers.IsAdmin, enableDisablePluginHandler(db))
}

func enableDisablePluginHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		pluginName := c.Params("name")

		plugin := GetPluginByName(pluginName)
		if plugin == nil {
			return c.SendStatus(fiber.StatusNotFound)
		}

		if err := EnableDisablePlugin(pluginName, db); err != nil {
			if err == errPluginNotFound {
				return c.SendStatus(fiber.StatusNotFound)
			}
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to update plugin")
		}

		enabled := plugin.Enabled(db)
		action := "disabled"
		if enabled {
			action = "enabled"
		}
		handlers.ShowToast(c, "Plugin "+action+" successfully, restart the server to see changes")

		return c.Render("partials/plugin-button", fiber.Map{
			"Name":    plugin.Name(),
			"Enabled": enabled,
		})
	}
}
