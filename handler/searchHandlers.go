package handlers

import (
	"strings"

	"goxcms/model"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

const siteSearchLimit = 20

// minSearchQuery is the shortest query worth running. Below it the LIKE can
// only match almost everything, and a leading-wildcard LIKE on an unindexed
// content column is a full table scan — so a one-character query is a free way
// to make the database work.
const minSearchQuery = 2

// SearchSite searches published posts and pages by title and content. The
// query is LIKE-escaped so % and _ match literally.
func SearchSite(c *fiber.Ctx, db *gorm.DB) error {
	query := strings.TrimSpace(c.Query("q"))
	if len([]rune(query)) < minSearchQuery {
		// Still render the page, just without querying.
		return RenderSite(c, "search", fiber.Map{
			"Title":      "Search",
			"Query":      query,
			"TooShort":   true,
			"IsAdmin":    c.Locals("isAdmin"),
			"IsLoggedIn": c.Locals("isLoggedin"),
			"Settings":   c.Locals("Settings"),
		})
	}

	var posts []model.Post
	var pages []model.CustomPage
	if query != "" {
		pattern := likePattern(query)
		db.Preload("Categories").Preload("Tags").
			Where("published = ? AND (title LIKE ? ESCAPE '\\' OR content LIKE ? ESCAPE '\\')", true, pattern, pattern).
			Order("created_at desc").
			Limit(siteSearchLimit).
			Find(&posts)
		db.Where("published = ? AND (title LIKE ? ESCAPE '\\' OR content LIKE ? ESCAPE '\\')", true, pattern, pattern).
			Order("title ASC").
			Limit(siteSearchLimit).
			Find(&pages)
	}

	return RenderSite(c, "search", fiber.Map{
		"Title":      "Search",
		"Query":      query,
		"Posts":      posts,
		"Pages":      pages,
		"IsAdmin":    c.Locals("isAdmin"),
		"IsLoggedIn": c.Locals("isLoggedin"),
		"Settings":   c.Locals("Settings"),
	})
}
