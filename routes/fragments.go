package routes

import (
	"strconv"

	handlers "goxcms/handler"
	"goxcms/model"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// setupFragmentRoutes registers the layout-less endpoints that HTMX swaps
// into an already-rendered page.
//
// Why these exist: hx-get on a normal page URL injects a second complete
// <html> document into the current one. The browser tolerates it badly, and
// the result is a page with nested <head> and <body> elements, duplicated
// scripts, and a title that flickers. Themes therefore could not do partial
// updates at all before this, which is most of why a theme needed a framework.
//
// A fragment renders the same template with NO layout, so the response is
// exactly the markup to swap in. Every fragment endpoint reuses the same data
// assembly as its full-page counterpart, so the two cannot drift apart.
func setupFragmentRoutes(app *fiber.App, db *gorm.DB) {
	// The navigation, on its own.
	app.Get("/frag/menu", func(c *fiber.Ctx) error {
		return handlers.GetPrimaryMenuRender(c, db)
	})

	// Paginated post lists. The page comes from the query string rather than
	// the path, because the fragment URL is generated inside an existing page.
	app.Get("/frag/blog", func(c *fiber.Ctx) error {
		return handlers.FragmentPostList(c, db, "blog/blog", "Blog", "page", nil)
	})

	app.Get("/frag/blog/category/:slug", func(c *fiber.Ctx) error {
		return handlers.FragmentCategoryList(c, db)
	})

	app.Get("/frag/blog/tag/:slug", func(c *fiber.Ctx) error {
		return handlers.FragmentTagList(c, db)
	})

	// Search results, for a search box that updates as you type.
	app.Get("/frag/search", func(c *fiber.Ctx) error {
		return handlers.FragmentSearch(c, db)
	})

	// A single post, without the surrounding chrome.
	app.Get("/frag/post/:slug", func(c *fiber.Ctx) error {
		return handlers.FragmentPost(c, db)
	})

	// Just the comments, for a post whose comments are refreshed after one is
	// added. This is the fragment the comment form actually swaps.
	app.Get("/frag/comments/:slug", func(c *fiber.Ctx) error {
		return handlers.FragmentComments(c, db)
	})

	// A custom page body, for swapping between pages without a full load.
	app.Get("/frag/page/:slug", func(c *fiber.Ctx) error {
		return handlers.FragmentCustomPage(c, db)
	})
}

// fragPage reads a positive page number from a query parameter, clamped so a
// hostile or stale ?page=99999999 cannot become a huge OFFSET.
func fragPage(c *fiber.Ctx, key string) int {
	n, err := strconv.Atoi(c.Query(key))
	if err != nil || n < 1 {
		return 1
	}
	return minInt(n, 10000)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ = model.Post{}
