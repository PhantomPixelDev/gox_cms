package handlers

import (
	"goxcms/model"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// customPageTemplates lists the views/page templates a custom page may use.
var customPageTemplates = map[string]bool{
	"page":           true,
	"page_sidebar":   true,
	"page_fullwidth": true,
}

// CustomPageTemplate returns name if it is an allowed page template, and the
// default "page" template otherwise.
func CustomPageTemplate(name string) string {
	if customPageTemplates[name] {
		return name
	}
	return "page"
}

// normalizePageSlug trims surrounding slashes and whitespace from a slug.
func normalizePageSlug(slug string) string {
	return strings.Trim(strings.TrimSpace(slug), "/")
}

func AddCustomPage(c *fiber.Ctx, db *gorm.DB) error {
	title := c.FormValue("title")
	content := SanitizeRichHTML(c.FormValue("content"))
	slug := normalizePageSlug(c.FormValue("slug"))
	template := c.FormValue("template")

	if title == "" || content == "" || slug == "" || template == "" {
		return c.SendString("Missing required fields: title, content, slug, template")
	}

	if !customPageTemplates[template] {
		return c.SendString("Unknown template: " + template)
	}

	var existingPage model.CustomPage
	result := db.Where("slug = ? OR title = ?", slug, title).First(&existingPage)
	if result.Error == nil {
		return c.SendString("Slug or title already exists: " + slug)
	}

	customPage := model.CustomPage{
		Title:    title,
		Content:  content,
		Slug:     slug,
		Template: template,
	}

	result = db.Create(&customPage)
	if result.Error != nil {
		if isDupKeyError(result.Error) {
			return c.SendString("Slug or title already exists: " + slug)
		}
		ShowToastError(c, "Error adding custom page")
		return c.Status(fiber.StatusInternalServerError).SendString("Error adding custom page")
	}

	return ShowToast(c, "Custom Page Added")

}

func SearchCustomPages(c *fiber.Ctx, db *gorm.DB) error {

	pageSize := 10 // Default page size
	searchQuery := c.Query("query", "")

	pageInt := queryPage(c)

	var custom_pages []model.CustomPage
	db.Where("title LIKE ? ESCAPE '\\'", likePattern(searchQuery)).
		Limit(pageSize).
		Offset((pageInt - 1) * pageSize).
		Find(&custom_pages)

	// Calculate total pages
	var count int64
	db.Model(&model.CustomPage{}).
		Where("title LIKE ? ESCAPE '\\'", likePattern(searchQuery)).
		Count(&count)
	totalPages := pageCount(count, pageSize)

	return c.Render("admin/table/custom-page-table", fiber.Map{
		"CustomPages": custom_pages,
		"TotalPages":  totalPages,
		"CurrentPage": pageInt,

		"SearchQuery": searchQuery,
	})
}
func EditCustomPage(c *fiber.Ctx, db *gorm.DB) error {
	id := c.FormValue("id")
	title := c.FormValue("title")
	content := SanitizeRichHTML(c.FormValue("content"))
	slug := normalizePageSlug(c.FormValue("slug"))
	template := c.FormValue("template")

	// convert id to int
	idInt, err := strconv.Atoi(id)
	if err != nil {
		return c.SendString("Invalid ID")
	}

	if idInt == 0 {
		return c.SendString("No ID provided")
	}

	if id == "" || title == "" || content == "" || slug == "" || template == "" {
		return c.SendString("Missing required fields: id, title, content, slug, template")
	}

	if !customPageTemplates[template] {
		return c.SendString("Unknown template: " + template)
	}

	var existingPage model.CustomPage
	if err := db.Where("slug = ? AND id <> ?", slug, idInt).First(&existingPage).Error; err == nil {
		return c.SendString("Slug already exists: " + slug)
	}

	customPage := model.CustomPage{
		Title:    title,
		Content:  content,
		Slug:     slug,
		Template: template,
	}

	result := db.Model(&model.CustomPage{}).Where("id = ?", id).Updates(customPage)
	if result.Error != nil {
		if isDupKeyError(result.Error) {
			return c.SendString("Slug already exists: " + slug)
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Error updating custom page")
	}

	return ShowToast(c, "Custom Page Updated")
}

func DeleteCustomPage(c *fiber.Ctx, db *gorm.DB) error {
	id, err := c.ParamsInt("id")

	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid ID")
	}

	result := db.Delete(&model.CustomPage{}, id)
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Could not delete custom page")
	}
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).SendString("Custom page not found")
	}

	return ShowToast(c, "Custom Page Deleted")
}
