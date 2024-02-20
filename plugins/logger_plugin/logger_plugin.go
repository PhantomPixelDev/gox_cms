package logger_plugin

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	// import print color package
)

type LoggerPlugin struct{}

func (p *LoggerPlugin) Setup(app *fiber.App, db *gorm.DB) error {
	fmt.Println("LoggerPlugin setup")
	app.Use(func(c *fiber.Ctx) error {
		// Print in different colors for different log levels
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
	fmt.Println("LoggerPlugin teardown")
	return nil
}

func (p *LoggerPlugin) Name() string {
	return "LoggerPlugin"
}
