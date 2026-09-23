package routes

import (
	"html/template"
	"strings"

	handlers "goxcms/handler"
	"goxcms/model"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/gofiber/template/html/v2"
	"gorm.io/gorm"
)

// SetupRoutes installs the auth and settings middleware and registers the
// public, auth and admin routes.
func SetupRoutes(app *fiber.App, db *gorm.DB, store *session.Store, engine *html.Engine) {
	app.Use(handlers.AuthStatusMiddleware(db))

	app.Use(func(c *fiber.Ctx) error {
		c.Locals("Settings", handlers.SiteSettings(db))
		return c.Next()
	})

	setupPublicRoutes(app, db)
	setupAuthRoutes(app, db, store)
	setupAdminRoutes(app, db, engine)
}

// SetupCustomPageRoutes serves custom pages by slug. It must be registered
// after every other route so it never shadows them, and it reads the page on
// each request so new and edited pages are live without a restart.
func SetupCustomPageRoutes(app *fiber.App, db *gorm.DB) {
	app.Get("/*", func(c *fiber.Ctx) error {
		slug := strings.Trim(c.Params("*"), "/")
		if slug == "" {
			return c.Next()
		}

		var customPage model.CustomPage
		if err := db.Where("slug = ?", slug).First(&customPage).Error; err != nil {
			return c.Next()
		}

		return c.Render("page/"+handlers.CustomPageTemplate(customPage.Template), fiber.Map{
			"Title":    customPage.Title,
			"Content":  template.HTML(customPage.Content),
			"Settings": c.Locals("Settings"),
		}, "main")
	})
}
