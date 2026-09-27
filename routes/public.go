package routes

import (
	handlers "goxcms/handler"
	"goxcms/model"
	"goxcms/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// RenderSite renders a public view from the template set the site has
// configured. Exposed from the handlers package so the route layer (and
// custom pages, which live here) can use the same resolver.
func RenderSite(c *fiber.Ctx, name string, data fiber.Map) error {
	return handlers.RenderSite(c, name, data)
}

// setupPublicRoutes registers the pages anyone can see.
func setupPublicRoutes(app *fiber.App, db *gorm.DB) {
	app.Get("/", func(c *fiber.Ctx) error {

		// Recent posts are passed in the render data rather than fetched by
		// the LatestPostsPlugin: that plugin is disabled by default and emits
		// Bootstrap markup, which the "simple" template set cannot use.
		recent := []model.Post{}
		db.Where("published = ?", true).Order("created_at desc").Limit(5).Find(&recent)

		return RenderSite(c, "index", fiber.Map{
			"Title":      "GoX CMS - HomePage",
			"Posts":      recent,
			"IsLoggedIn": c.Locals("isLoggedin"),
			"IsAdmin":    c.Locals("isAdmin"),
			"Settings":   c.Locals("Settings"),
		})
	})

	app.Get("/get-primary-menu", func(c *fiber.Ctx) error {
		return handlers.GetPrimaryMenuRender(c, db)
	})

	// Layout-less fragments for HTMX swaps.
	//
	// hx-get on a normal page URL injects a whole second <html> document into
	// the current one, which is why themes could not do partial updates before
	// this. A fragment renders the same template with no layout, so the
	// response is exactly the markup to swap in.
	setupFragmentRoutes(app, db)

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
