package routes

import (
	handlers "goxcms/handler"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"gorm.io/gorm"
)

// setupAuthRoutes registers registration, login and logout.
func setupAuthRoutes(app *fiber.App, db *gorm.DB, store *session.Store) {
	app.Get("/register", func(c *fiber.Ctx) error {

		if handlers.IsTrue(c, "isLoggedin") {
			return c.Redirect("/")
		}

		return c.Render("register", fiber.Map{
			"Title":    "Register",
			"Settings": c.Locals("Settings"),
		}, "main")
	})

	app.Post("/register", handlers.Register(db))

	app.Get("/login", func(c *fiber.Ctx) error {

		if handlers.IsTrue(c, "isLoggedin") {
			return c.Redirect("/")
		}

		return c.Render("login", fiber.Map{
			"Title":    "Login",
			"Settings": c.Locals("Settings"),
		}, "main")
	})

	app.Post("/login", handlers.Login(db, store))

	app.Post("/logout", func(c *fiber.Ctx) error {

		return handlers.Logout(c)
	})
}
