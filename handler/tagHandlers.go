package handlers

import (
	"encoding/json"
	"goxcms/model"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"math"

	"gorm.io/gorm"
)

func BlogTagPage(c *fiber.Ctx, db *gorm.DB) error {
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

	var tag model.Tag
	result := db.Where("Slug = ?", slug).First(&tag)
	if result.Error != nil || tag.ID == 0 {
		return c.Status(404).Render("404", fiber.Map{
			"Title": "404",
		}, "main")
	}

	var posts []model.Post
	db.Joins("JOIN post_tags ON post_tags.post_id = posts.id").
		Where("post_tags.tag_id = ? AND posts.published = ?", tag.ID, true).
		Order("posts.created_at desc").
		Limit(postsPerPage).
		Offset(offset).
		Find(&posts)

	var totalPosts int64
	db.Model(&model.Post{}).
		Joins("JOIN post_tags ON post_tags.post_id = posts.id").
		Where("post_tags.tag_id = ? AND posts.published = ?", tag.ID, true).
		Count(&totalPosts)

	totalPages := int(math.Ceil(float64(totalPosts) / float64(postsPerPage)))

	if pageNumber > totalPages {
		return c.Redirect("/blog/tag/" + slug + "/1")
	}

	var totalPagesArray []int
	for i := 1; i <= totalPages; i++ {
		totalPagesArray = append(totalPagesArray, i)
	}

	return c.Render("blog/blog_tag", fiber.Map{
		"Title":         tag.Name,
		"Posts":         posts,
		"Slug":          tag.Slug,
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

func SearchTag(c *fiber.Ctx, db *gorm.DB) error {

	var tags []model.Tag
	searchQuery := c.Query("query")
	page := c.Query("page", "1")
	pageSize := 10 // Default page size

	// Convert page string to int
	pageInt, err := strconv.Atoi(page)
	if err != nil || pageInt < 1 {
		pageInt = 1
	}

	// Search for tags with pagination
	db.Where("name LIKE ?", "%"+searchQuery+"%").
		Limit(pageSize).
		Offset((pageInt - 1) * pageSize).
		Find(&tags)

	// Count the number of posts for each tag
	for i := range tags {
		var count int64
		db.Model(&model.Post{}).
			Joins("join post_tags on post_tags.post_id = posts.id").
			Where("post_tags.tag_id = ?", tags[i].ID).
			Count(&count)

		tags[i].PostsCount = int(count)
	}

	// Count total tags that match the search query for pagination
	var totalMatchingCount int64
	db.Model(&model.Tag{}).
		Where("name LIKE ?", "%"+searchQuery+"%").
		Count(&totalMatchingCount)
	totalPages := int(math.Ceil(float64(totalMatchingCount) / float64(pageSize)))

	return c.Render("admin/table/tag-table", fiber.Map{
		"Tags":        tags,
		"TotalPages":  totalPages,
		"CurrentPage": pageInt,
		"SearchQuery": searchQuery,
	})
}

func AddTag(c *fiber.Ctx, db *gorm.DB) error {
	name := c.FormValue("tag_name")
	slug := c.FormValue("tag_slug")

	var tag model.Tag
	db.Where("name = ?", name).Or("slug = ?", slug).First(&tag)

	if tag.ID != 0 {
		if tag.Name == name {
			return ShowToastError(c, "Tag name already exists")
		}
		if tag.Slug == slug {
			return ShowToastError(c, "Tag with the slug already exists")
		}
	}

	db.Create(&model.Tag{
		Name: name,
		Slug: slug,
	})

	message := map[string]string{"showToast": "Tag added successfully"}
	messageBytes, _ := json.Marshal(message)
	c.Set("HX-Trigger", string(messageBytes))
	c.Status(fiber.StatusOK)

	return nil

}

func DeleteTag(c *fiber.Ctx, db *gorm.DB) error {

	id := c.Query("id")

	var tag model.Tag

	if err := db.First(&tag, id).Error; err != nil {
		return ShowToastError(c, "Tag not found")
	}

	// Delete the associated records in the "post_tags" table
	if err := db.Exec("DELETE FROM post_tags WHERE tag_id = ?", tag.ID).Error; err != nil {
		return ShowToastError(c, "Error deleting associated records")
	}

	if err := db.Delete(&tag).Error; err != nil {
		return ShowToastError(c, "Error deleting tag")
	}

	return ShowToastError(c, "Tag with ID "+id+" deleted successfully")
}
