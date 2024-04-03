package handlers

import (
	"goxcms/model"
	"math"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
	"gorm.io/gorm"
)

func RenderCustomPage(c *fiber.Ctx, db *gorm.DB, app *fiber.App, slug string, engine *html.Engine) {
	var customPage model.CustomPage
	result := db.Where("slug = ?", slug).First(&customPage)
	if result.Error != nil {
		c.Status(fiber.StatusNotFound)
		c.SendString("Custom Page not found" + result.Error.Error())
		return
	}

	app.Get("/"+slug, func(c *fiber.Ctx) error {
		/// reload the engine to reflect changes
		engine.Load()

		return c.Render("custom/"+customPage.Template, fiber.Map{
			"Title":   customPage.Title,
			"Content": customPage.Content,
		})
	})

	/// reload the engine to reflect changes

}

func AddCustomPage(c *fiber.Ctx, db *gorm.DB, app *fiber.App, engine *html.Engine) error {
	title := c.FormValue("title")
	content := c.FormValue("content")
	slug := c.FormValue("slug")
	template := c.FormValue("template")

	if title == "" || content == "" || slug == "" || template == "" { // Check if required fields are missing
		return c.SendString("Missing required fields: title, content, slug, template")
	}

	var existingPage model.CustomPage
	result := db.Where("slug = ? OR title = ?", slug, title).First(&existingPage)
	if result.Error == nil {
		return c.SendString("Slug or title alreasdy exists")
	}

	customPage := model.CustomPage{
		Title:    title,
		Content:  content,
		Slug:     slug,
		Template: template,
	}

	//RenderCustomPage(c, db, app, slug, engine)

	result = db.Create(&customPage)
	if result.Error != nil {
		ShowToastError(c, "Error adding custom page: "+result.Error.Error())
		return c.Status(fiber.StatusInternalServerError).SendString(result.Error.Error())
	}

	return ShowToast(c, "Custom Page Added - Restart server to see changes")

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
	content := c.FormValue("content")
	slug := c.FormValue("slug")
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

	customPage := model.CustomPage{
		Title:    title,
		Content:  content,
		Slug:     slug,
		Template: template,
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
