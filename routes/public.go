package routes

import (
	handlers "goxcms/handler"
	"goxcms/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// setupPublicRoutes registers the pages anyone can see.
func setupPublicRoutes(app *fiber.App, db *gorm.DB) {
	app.Get("/", func(c *fiber.Ctx) error {

		return c.Render("index", fiber.Map{
			"Title":      "GoX CMS - HomePage",
			"IsLoggedIn": c.Locals("isLoggedin"),
			"IsAdmin":    c.Locals("isAdmin"),
			"Settings":   c.Locals("Settings"),
		}, "main")
	})

	app.Get("/get-primary-menu", func(c *fiber.Ctx) error {
		return handlers.GetPrimaryMenuRender(c, db)
	})

	app.Post("/add-comment", handlers.IsLoggedIn, func(c *fiber.Ctx) error {
		return handlers.AddComment(c, db)
	})

	app.Get("/blog/:page?", func(c *fiber.Ctx) error {
		return handlers.BlogPage(c, db)
	})

	app.Get("/blog/post/:slug", func(c *fiber.Ctx) error {
		return handlers.BlogPostPage(c, db)
	})

	app.Get("/blog/category/:slug/:page?", func(c *fiber.Ctx) error {
		return handlers.BlogCategoryPage(c, db)
	})

	app.Get("/blog/tag/:slug/:page?", func(c *fiber.Ctx) error {
		return handlers.BlogTagPage(c, db)
	})

	app.Get("/sitemap.xml", func(c *fiber.Ctx) error {
		c.Type("xml", "utf-8")
		return c.Send(utils.BuildSitemap(db))
	})

	app.Get("/search", func(c *fiber.Ctx) error {
		return handlers.SearchSite(c, db)
	})

	// Health check for Docker/Caddy/load balancers. Anonymous on purpose.
	app.Get("/healthz", func(c *fiber.Ctx) error {
		if sqlDB, err := db.DB(); err != nil || sqlDB.Ping() != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "unhealthy"})
		}
		return c.JSON(fiber.Map{"status": "ok"})
	})
}
