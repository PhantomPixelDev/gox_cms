package handlers

import (
	"html/template"
	"strings"
	"time"

	"goxcms/model"

	"github.com/gofiber/fiber/v2"
)

// PreviewPost renders unsaved post data through the real blog template in a
// new tab. Nothing is written to the database.
func PreviewPost(c *fiber.Ctx) error {
	title := strings.TrimSpace(c.FormValue("title"))
	if title == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Title is required for preview")
	}
	post := model.Post{
		Title:     title,
		Content:   SanitizeRichHTML(c.FormValue("content")),
		ImageURL:  strings.TrimSpace(c.FormValue("image")),
		CreatedAt: time.Now(),
	}

	return c.Render("blog/blog_post", fiber.Map{
		"UserID":     currentUserID(c),
		"Title":      post.Title,
		"Post":       post,
		"Content":    template.HTML(post.Content),
		"CreatedAt":  post.CreatedAt,
		"IsAdmin":    false,
		"IsLoggedIn": c.Locals("isLoggedin"),
		"Preview":    true,
		"Settings":   c.Locals("Settings"),
	}, "main")
}

// PreviewPage renders unsaved page data through the real page template.
// Nothing is written to the database.
func PreviewPage(c *fiber.Ctx) error {
	title := strings.TrimSpace(c.FormValue("title"))
	if title == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Title is required for preview")
	}

	return c.Render("page/page", fiber.Map{
		"Title":    title,
		"Content":  template.HTML(SanitizeRichHTML(c.FormValue("content"))),
		"Settings": c.Locals("Settings"),
	}, "main")
}
