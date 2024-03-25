package handlers

import (
	"encoding/json"
	"goxcms/model"
	"strconv"

	"math"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func BlogCategoryPage(c *fiber.Ctx, db *gorm.DB) error {
	slug := c.Params("slug")
	page := c.Params("page")

	if slug == "" {
		return c.Redirect("/blog")
	}

	if page == "" {
		page = "1"
	}

	pageNumber, err := strconv.Atoi(page)
	if err != nil || pageNumber < 1 {
		pageNumber = 1
	}

	postsPerPage := 5

	offset := (pageNumber - 1) * postsPerPage

	var category model.Category
	result := db.Where("Slug = ?", slug).First(&category)
	if result.Error != nil || category.ID == 0 {
		return c.Status(404).Render("404", fiber.Map{
			"Title":    "404",
			"Settings": c.Locals("Settings"),
		}, "main")
	}

	var posts []model.Post
	db.Joins("JOIN post_categories ON post_categories.post_id = posts.id").
		Where("post_categories.category_id = ? AND posts.published = ?", category.ID, true).
		Order("posts.created_at desc").
		Limit(postsPerPage).
		Offset(offset).
		Find(&posts)

	var totalPosts int64
	db.Model(&model.Post{}).
		Joins("JOIN post_categories ON post_categories.post_id = posts.id").
		Where("post_categories.category_id = ? AND posts.published = ?", category.ID, true).
		Count(&totalPosts)

	totalPages := int(math.Ceil(float64(totalPosts) / float64(postsPerPage)))

	if pageNumber > totalPages {
		return c.Redirect("/blog/category/" + slug + "/1")
	}

	var totalPagesArray []int
	for i := 1; i <= totalPages; i++ {
		totalPagesArray = append(totalPagesArray, i)

	}

	return c.Render("blog/blog_category", fiber.Map{
		"Title":         category.Name,
		"Posts":         posts,
		"Slug":          category.Slug,
		"IsAdmin":       c.Locals("isAdmin"),
		"IsLoggedIn":    c.Locals("isLoggedin"),
		"TotalPages":    totalPagesArray,
		"TotalPagesInt": totalPages,
		"NextPage":      pageNumber + 1,
		"PrevPage":      pageNumber - 1,
		"CurrentPage":   pageNumber,
		"Settings":      c.Locals("Settings"),
	}, "main")
}

func AddCategory(c *fiber.Ctx, db *gorm.DB) error {
	name := c.FormValue("category_name")
	slug := c.FormValue("category_slug")

	var category model.Category
	db.Where("name = ?", name).Or("slug = ?", slug).First(&category)

	if category.ID != 0 {
		if category.Name == name {
			return ShowToastError(c, "Category name already exists")
		} else if category.Slug == slug {
			return ShowToastError(c, "Category slug already exists")
		}
	}

	db.Create(&model.Category{
		Name: name,
		Slug: slug,
	})

	message := map[string]string{"showToast": "Category added successfully"}
	messageBytes, _ := json.Marshal(message)
	c.Set("HX-Trigger", string(messageBytes))

	c.Status(fiber.StatusOK)

	return nil
}

func DeleteCategory(c *fiber.Ctx, db *gorm.DB) error {
	id := c.Query("id")
	var category model.Category

	// Find the category
	if err := db.First(&category, id).Error; err != nil {
		// Handle the error if the category is not found
		return ShowToastError(c, "Category not found")
	}

	if err := db.Exec("DELETE FROM post_categories WHERE category_id = ?", category.ID).Error; err != nil {
		// Handle the error if deleting the associated records fails
		return ShowToastError(c, "Error deleting associated records")
	}

	if err := db.Delete(&category).Error; err != nil {
		// Handle the error if deleting the category fails
		return ShowToastError(c, "Error deleting category")
	}

	return ShowToastError(c, "Category deleted successfully")
}

func SearchCategories(c *fiber.Ctx, db *gorm.DB) error {
	// Get the page number and search query from the query parameters
	page := c.Query("page", "1")
	pageSize := 10 // Default page size
	searchQuery := c.Query("query")

	// Convert page string to int
	pageInt, err := strconv.Atoi(page)
	if err != nil || pageInt < 1 {
		pageInt = 1
	}

	// Search for categories with pagination
	var categories []model.Category
	db.Where("name LIKE ?", "%"+searchQuery+"%").
		Limit(pageSize).
		Offset((pageInt - 1) * pageSize).
		Find(&categories)

	// Count the number of posts for each category
	for i := range categories {
		var count int64
		db.Model(&model.Post{}).
			Joins("join post_categories on post_categories.post_id = posts.id").
			Where("post_categories.category_id = ?", categories[i].ID).
			Count(&count)

		categories[i].PostsCount = int(count)
	}

	var totalMatchingCount int64
	db.Model(&model.Category{}).
		Where("name LIKE ?", "%"+searchQuery+"%").
		Count(&totalMatchingCount)
	totalPages := int(math.Ceil(float64(totalMatchingCount) / float64(pageSize)))

	return c.Render("admin/table/category-table", fiber.Map{
		"Categories":  categories,
		"TotalPages":  totalPages,
		"CurrentPage": pageInt,

		"SearchQuery": searchQuery,
	})
}
