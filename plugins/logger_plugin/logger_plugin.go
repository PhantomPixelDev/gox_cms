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

	/// add path loaderio-422dc4c70ddacc89acd6a63f82d42134 and return loaderio-422dc4c70ddacc89acd6a63f82d42134 to verify domain
	app.Get("/loaderio-422dc4c70ddacc89acd6a63f82d42134", func(c *fiber.Ctx) error {
		return c.SendString("loaderio-422dc4c70ddacc89acd6a63f82d42134")
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
