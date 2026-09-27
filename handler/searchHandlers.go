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
	data, err := searchData(c, db)
	if err != nil {
		return c.Status(500).SendString("Search failed")
	}
	return RenderSite(c, "search", data)
}

// searchData assembles the data for the search page. Shared with
// /frag/search, so the two cannot disagree.
func searchData(c *fiber.Ctx, db *gorm.DB) (fiber.Map, error) {
	query := strings.TrimSpace(c.Query("q"))
	base := fiber.Map{
		"Title":      "Search",
		"Query":      query,
		"IsAdmin":    c.Locals("isAdmin"),
		"IsLoggedIn": c.Locals("isLoggedin"),
		"Settings":   c.Locals("Settings"),
	}

	if len([]rune(query)) < minSearchQuery {
		// Render the page, but do not query. A leading-wildcard LIKE on an
		// unindexed content column is a full table scan, so a one-character
		// query is a free way to make the database work.
		base["TooShort"] = true
		base["Posts"] = []model.Post{}
		base["Pages"] = []model.CustomPage{}
		return base, nil
	}

	pattern := likePattern(query)
	var posts []model.Post
	if err := db.Preload("Categories").Preload("Tags").
		Where("published = ? AND (title LIKE ? ESCAPE '\\' OR content LIKE ? ESCAPE '\\')", true, pattern, pattern).
		Order("created_at DESC").
		Limit(siteSearchLimit).
		Find(&posts).Error; err != nil {
		return nil, err
	}

	var pages []model.CustomPage
	if err := db.Where("published = ? AND (title LIKE ? ESCAPE '\\' OR content LIKE ? ESCAPE '\\')", true, pattern, pattern).
		Order("title ASC").
		Limit(siteSearchLimit).
		Find(&pages).Error; err != nil {
		return nil, err
	}

	// Always present, so a theme can range over them without a nil check. The
	// page and the fragment previously differed here, and ranging over a nil
	// slice is silently an empty list rather than an error.
	base["Posts"] = posts
	base["Pages"] = pages
	base["TooShort"] = false
	return base, nil
}
