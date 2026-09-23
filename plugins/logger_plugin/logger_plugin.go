package logger_plugin

import (
	"goxcms/model"
	"strconv"

	"github.com/fatih/color"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
	"gorm.io/gorm"
)

const (
	PluginName = "LoggerPlugin"
	Author     = "Ashba22"
	Version    = "1.0"
)

// LoggerPlugin prints a coloured line per request. Add it to
// plugin_system.PluginList and enable it from the admin panel to use it.
type LoggerPlugin struct{}

func (p *LoggerPlugin) Setup(app *fiber.App, db *gorm.DB, engine *html.Engine) error {
	app.Use(func(c *fiber.Ctx) error {
		color.Cyan("Path: %s", c.Path())
		color.Green("Method: %s", c.Method())
		color.Yellow("Connection: %s", c.Context().RemoteAddr())
		color.Blue("User-Agent: %s", c.Get("User-Agent"))
		color.Magenta("Referer: %s", c.Get("Referer"))

		return c.Next()
	})

	return nil
}

func (p *LoggerPlugin) Teardown() error {
	return nil
}

func (p *LoggerPlugin) Name() string {
	return PluginName
}

func (p *LoggerPlugin) Author() string {
	return Author
}

func (p *LoggerPlugin) Version() string {
	return Version
}

func (p *LoggerPlugin) DefaultSettings() map[string]string {
	return map[string]string{}
}

func (p *LoggerPlugin) Settings(db *gorm.DB) map[string]string {
	return map[string]string{
		"Enabled": strconv.FormatBool(p.Enabled(db)),
	}
}

func (p *LoggerPlugin) Enabled(db *gorm.DB) bool {
	plugin := &model.Plugin{}
	db.Where("name = ?", PluginName).First(plugin)
	return plugin.Enabled
}
