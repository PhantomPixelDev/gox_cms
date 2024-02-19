package handlers

import (
	"goxcms/model"
	"math"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func AddCustomPage(c *fiber.Ctx, db *gorm.DB, app *fiber.App) error {
	title := c.FormValue("title")
	content := c.FormValue("content")
	slug := c.FormValue("slug")

	if title == "" || content == "" || slug == "" {
		return c.SendString("Missing required fields: title, content, slug")
	}

	// Check if slug or title already exists
	var existingPage model.CustomPage
	result := db.Where("slug = ? OR title = ?", slug, title).First(&existingPage)
	if result.Error == nil {
		return c.SendString("Slug or title already exists")
	}

	customPage := model.CustomPage{
		Title:   title,
		Content: content,
		Slug:    slug,
	}

	result = db.Create(&customPage)
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(result.Error.Error())
	}

	return ShowToastError(c, "Custom Page Added - Restart server to see changes")

}

func SearchCustomPages(c *fiber.Ctx, db *gorm.DB) error {

	page := c.Query("page", "1")
	pageSize := 10 // Default page size
	searchQuery := c.Query("query", "")

	pageInt, err := strconv.Atoi(page)

	if err != nil || pageInt < 1 {
		pageInt = 1
	}

	var custom_pages []model.CustomPage
	db.Where("title LIKE ?", "%"+searchQuery+"%").
		Limit(pageSize).
		Offset((pageInt - 1) * pageSize).
		Find(&custom_pages)

	// Calculate total pages
	var count int64
	db.Model(&model.CustomPage{}).
		Where("title LIKE ?", "%"+searchQuery+"%").
		Count(&count)
	totalPages := int(math.Ceil(float64(count) / float64(pageSize)))

	return c.Render("admin/admin-custom-page-table", fiber.Map{
		"CustomPages": custom_pages,
		"TotalPages":  totalPages,
		"CurrentPage": pageInt,

		"SearchQuery": searchQuery,
	})
}
func EditCustomPage(c *fiber.Ctx, db *gorm.DB) error {
	id := c.FormValue("id")
	title := c.FormValue("title")
	content := c.FormValue("content")
	slug := c.FormValue("slug")

	// convert id to int
	idInt, err := strconv.Atoi(id)
	if err != nil {
		return c.SendString("Invalid ID")
	}

	if idInt == 0 {
		return c.SendString("No ID provided")
	}

	if id == "" || title == "" || content == "" || slug == "" {
		return c.SendString("Missing required fields: id, title, content, slug")
	}

	customPage := model.CustomPage{
		Title:   title,
		Content: content,
		Slug:    slug,
	}

	result := db.Model(&model.CustomPage{}).Where("id = ?", id).Updates(customPage)
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(result.Error.Error())
	}

	return ShowToastError(c, "Custom Page Updated")
}

func DeleteCustomPage(c *fiber.Ctx, db *gorm.DB) error {
	id, err := c.ParamsInt("id")

	if err != nil {
		return c.SendString("Invalid ID")
	}

	if id == 0 {
		return c.SendString("No ID provided")
	}

	result := db.Delete(&model.CustomPage{}, id)
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(result.Error.Error())
	}

	return ShowToastError(c, "Custom Page Deleted")
}
