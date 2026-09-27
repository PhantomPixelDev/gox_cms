package handlers

import (
	"errors"
	"html/template"

	"goxcms/model"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Fragment handlers render a template with no layout, for HTMX swaps.
//
// Each one reuses the data assembly of its full-page counterpart. That is the
// whole point: when the two built their data separately, a key added to the
// page handler never reached the fragment, and the template rendered "0" or
// nothing instead of failing, so the bug only appeared as a subtly empty
// panel in one interaction.

// FragmentPostList renders a page of the blog as a bare fragment.
func FragmentPostList(c *fiber.Ctx, db *gorm.DB, view, title, pageKey string, scope *postScope) error {
	page := fragPage(c, pageKey)
	data, err := postListData(db, view, title, page, scope)
	if err != nil {
		return c.Status(500).SendString("Could not load posts")
	}
	return RenderFragment(c, view, data)
}

// fragPage reads a positive page number from the query, clamped so a hostile
// or stale ?page=99999999 cannot become an enormous OFFSET. clampPage already
// does this for path parameters, but a fragment URL is generated inside an
// existing page and its page number arrives as a query string.
func fragPage(c *fiber.Ctx, key string) int {
	return clampPage(c.Query(key))
}

// FragmentCategoryList renders one category's posts as a bare fragment.
func FragmentCategoryList(c *fiber.Ctx, db *gorm.DB) error {
	slug := c.Params("slug")
	var category model.Category
	if err := db.Where("slug = ?", slug).First(&category).Error; err != nil || category.ID == 0 {
		return RenderNotFound(c)
	}
	scope := &postScope{
		join:   "JOIN post_categories ON post_categories.post_id = posts.id",
		clause: "post_categories.category_id = ? AND posts.published = ?",
		args:   []any{category.ID, true},
		slug:   category.Slug,
		name:   category.Name,
		base:   "/blog/category/" + category.Slug,
	}
	return FragmentPostList(c, db, "blog/blog_category", category.Name, "page", scope)
}

// FragmentTagList renders one tag's posts as a bare fragment.
func FragmentTagList(c *fiber.Ctx, db *gorm.DB) error {
	slug := c.Params("slug")
	var tag model.Tag
	if err := db.Where("slug = ?", slug).First(&tag).Error; err != nil || tag.ID == 0 {
		return RenderNotFound(c)
	}
	scope := &postScope{
		join:   "JOIN post_tags ON post_tags.post_id = posts.id",
		clause: "post_tags.tag_id = ? AND posts.published = ?",
		args:   []any{tag.ID, true},
		slug:   tag.Slug,
		name:   tag.Name,
		base:   "/blog/tag/" + tag.Slug,
	}
	return FragmentPostList(c, db, "blog/blog_tag", tag.Name, "page", scope)
}

// FragmentSearch renders search results as a bare fragment, for a search box
// that updates as the visitor types.
func FragmentSearch(c *fiber.Ctx, db *gorm.DB) error {
	data, err := searchData(c, db)
	if err != nil {
		return c.Status(500).SendString("Search failed")
	}
	return RenderFragment(c, "search", data)
}

// FragmentPost renders a single post, without the site chrome.
func FragmentPost(c *fiber.Ctx, db *gorm.DB) error {
	data, err := postPageData(c, db)
	if err != nil {
		return notFoundOrServer(c, err)
	}
	return RenderFragment(c, "blog/blog_post", data)
}

// FragmentComments renders just a post's comments, which is what the comment
// form swaps in after a comment is added: re-rendering the whole post would
// throw away the visitor's scroll position and re-run every post script.
func FragmentComments(c *fiber.Ctx, db *gorm.DB) error {
	data, err := postPageData(c, db)
	if err != nil {
		return notFoundOrServer(c, err)
	}
	// The comment list is a shared partial, not a theme template: Go templates
	// cannot compute a template name, so a per-theme copy could not be included
	// from the theme's post template. See views/partials/comments.html.
	return RenderSharedFragment(c, "comments", fiber.Map{
		"Post":       data["Post"],
		"Comments":   data["Comments"],
		"Title":      data["Title"],
		"IsAdmin":    c.Locals("isAdmin"),
		"IsLoggedIn": c.Locals("isLoggedin"),
		"Settings":   c.Locals("Settings"),
	})
}

// FragmentCustomPage renders a custom page body as a bare fragment.
func FragmentCustomPage(c *fiber.Ctx, db *gorm.DB) error {
	slug := c.Params("slug")

	var page model.CustomPage
	// published = true, and parameterised: a draft page must not be reachable
	// through the fragment endpoint either.
	if err := db.Where("slug = ? AND published = ?", slug, true).First(&page).Error; err != nil || page.ID == 0 {
		return RenderNotFound(c)
	}

	// The page templates read .Title and .Content directly rather than a
	// wrapper object, so a fragment has to supply the same shape. Anything else
	// renders an empty page rather than failing, which is why this mirrors
	// SetupCustomPageRoutes exactly.
	return RenderFragment(c, "page/"+CustomPageTemplate(page.Template), fiber.Map{
		"Title":   page.Title,
		"Content": template.HTML(page.Content),
		// Not used by the built-in templates, but a custom page template may
		// reach for them.
		"CustomPage": page,
		"IsAdmin":    c.Locals("isAdmin"),
		"IsLoggedIn": c.Locals("isLoggedin"),
		"Settings":   c.Locals("Settings"),
	})
}

// notFoundOrServer maps a lookup error to the right status: a missing post is
// a 404, a database failure is a 500. Returning 500 for "no such post" makes
// broken links look like server faults in the logs.
func notFoundOrServer(c *fiber.Ctx, err error) error {
	if errors.Is(err, errNotFound) {
		return RenderNotFound(c)
	}
	return c.Status(500).SendString("Could not load post")
}
