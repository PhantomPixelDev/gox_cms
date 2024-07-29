package plugin_system

import (
	"encoding/json"
	"fmt"
	handlers "goxcms/handler"
	"goxcms/model"
	"log"

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

		db.Create(&model.Plugin{Name: plugin.Name(), Author: plugin.Author(), Version: plugin.Version(), Enabled: plugin.Enabled(db), Settings: string(settingsJSON)})
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

func EnableDisablePlugin(pluginName string, db *gorm.DB) error {
	pluginDB := model.Plugin{}
	db.Where("name = ?", pluginName).First(&pluginDB)
	if pluginDB.ID == 0 {
		return nil
	}
	/// change value in database and then reload the plugin if enabled
	pluginDB.Enabled = !pluginDB.Enabled
	db.Save(&pluginDB)
	println("Plugin enabled: ", pluginDB.Enabled)
	/// reset app store from app store (cache) and reload the plugin

	return nil
}

// / add route to enable/disable plugin
func AddPluginManagerRoutes(app *fiber.App, db *gorm.DB) {
	app.Get("/admin/plugins/enable/:name", handlers.IsAdmin, handlers.IsLoggedIn, enableDisablePluginHandler(db))
}

func enableDisablePluginHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		pluginName := c.Params("name")
		fmt.Println("Plugin name: ", pluginName)

		EnableDisablePlugin(pluginName, db)

		plugin := GetPluginByName(pluginName)
		if plugin == nil {
			fmt.Println("Plugin not found")
			return c.SendStatus(fiber.StatusNotFound)
		}

		buttonText := "Enable"
		if plugin.Enabled(db) {
			buttonText = "Disable"
		}

		buttonClass := "btn btn-success mt-2 btn-plugin"
		if plugin.Enabled(db) {
			buttonClass = "btn btn-danger mt-2 btn-plugin"
		}

		htmxResponse := fmt.Sprintf(`<button id="plugin-%s" class="%s" hx-get="/admin/plugins/enable/%s" hx-trigger="click" hx-headers='{"X-No-Cache": "true"}' hx-swap="outerHTML">%s</button>`, pluginName, buttonClass, pluginName, buttonText)

		handlers.ShowToast(c, "Plugin "+buttonText+"d successfully, restart the server to see changes")

		return c.Status(fiber.StatusOK).SendString(htmxResponse)
	}
}
