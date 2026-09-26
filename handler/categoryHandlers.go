package handlers

import (
	"encoding/json"
	"goxcms/model"
	"strconv"

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

	totalPages := pageCount(totalPosts, postsPerPage)

	if totalPosts > 0 && pageNumber > totalPages {
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

	if err := db.Create(&model.Category{
		Name: name,
		Slug: slug,
	}).Error; err != nil {
		if isDupKeyError(err) {
			return ShowToastError(c, "Category name or slug already exists")
		}
		return ShowToastError(c, "Could not create category")
	}

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
		ShowToastError(c, "Category not found")
		return c.Status(fiber.StatusNotFound).SendString("Category not found")
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM post_categories WHERE category_id = ?", category.ID).Error; err != nil {
			return err
		}
		return tx.Delete(&category).Error
	}); err != nil {
		ShowToastError(c, "Error deleting category")
		return c.Status(fiber.StatusInternalServerError).SendString("Error deleting category")
	}

	return ShowToast(c, "Category deleted successfully")
}

func SearchCategories(c *fiber.Ctx, db *gorm.DB) error {
	// Get the page number and search query from the query parameters
	pageSize := 10 // Default page size
	searchQuery := c.Query("query")

	// Convert page string to int
	pageInt := queryPage(c)

	// Search for categories with pagination
	var categories []model.Category
	db.Where("name LIKE ? ESCAPE '\\'", likePattern(searchQuery)).
		Limit(pageSize).
		Offset((pageInt - 1) * pageSize).
		Find(&categories)

	// Count posts per category in one query instead of one per row.
	if len(categories) > 0 {
		ids := make([]uint, 0, len(categories))
		for i := range categories {
			ids = append(ids, categories[i].ID)
		}
		type catCount struct {
			CategoryID uint
			Total      int64
		}
		var counts []catCount
		db.Model(&model.Post{}).
			Select("post_categories.category_id as category_id, COUNT(*) as total").
			Joins("join post_categories on post_categories.post_id = posts.id").
			Where("post_categories.category_id IN ?", ids).
			Group("post_categories.category_id").
			Scan(&counts)
		byID := make(map[uint]int, len(counts))
		for _, cc := range counts {
			byID[cc.CategoryID] = int(cc.Total)
		}
		for i := range categories {
			categories[i].PostsCount = byID[categories[i].ID]
		}
	}

	var totalMatchingCount int64
	db.Model(&model.Category{}).
		Where("name LIKE ? ESCAPE '\\'", likePattern(searchQuery)).
		Count(&totalMatchingCount)
	totalPages := pageCount(totalMatchingCount, pageSize)

	return c.Render("admin/table/category-table", fiber.Map{
		"Categories":  categories,
		"TotalPages":  totalPages,
		"CurrentPage": pageInt,

		"SearchQuery": searchQuery,
	})
}
