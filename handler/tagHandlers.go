package handlers

import (
	"encoding/json"
	"goxcms/model"
	"strconv"

	"github.com/gofiber/fiber/v2"

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

	totalPages := pageCount(totalPosts, postsPerPage)

	if totalPosts > 0 && pageNumber > totalPages {
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
	pageSize := 10 // Default page size

	// Convert page string to int
	pageInt := queryPage(c)

	// Search for tags with pagination
	db.Where("name LIKE ? ESCAPE '\\'", likePattern(searchQuery)).
		Limit(pageSize).
		Offset((pageInt - 1) * pageSize).
		Find(&tags)

	// Count posts per tag in one query instead of one per row.
	if len(tags) > 0 {
		ids := make([]uint, 0, len(tags))
		for i := range tags {
			ids = append(ids, tags[i].ID)
		}
		type tagCount struct {
			TagID uint
			Total int64
		}
		var counts []tagCount
		db.Model(&model.Post{}).
			Select("post_tags.tag_id as tag_id, COUNT(*) as total").
			Joins("join post_tags on post_tags.post_id = posts.id").
			Where("post_tags.tag_id IN ?", ids).
			Group("post_tags.tag_id").
			Scan(&counts)
		byID := make(map[uint]int, len(counts))
		for _, tc := range counts {
			byID[tc.TagID] = int(tc.Total)
		}
		for i := range tags {
			tags[i].PostsCount = byID[tags[i].ID]
		}
	}

	// Count total tags that match the search query for pagination
	var totalMatchingCount int64
	db.Model(&model.Tag{}).
		Where("name LIKE ? ESCAPE '\\'", likePattern(searchQuery)).
		Count(&totalMatchingCount)
	totalPages := pageCount(totalMatchingCount, pageSize)

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

	if err := db.Create(&model.Tag{
		Name: name,
		Slug: slug,
	}).Error; err != nil {
		if isDupKeyError(err) {
			return ShowToastError(c, "Tag name or slug already exists")
		}
		return ShowToastError(c, "Could not create tag")
	}

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
		ShowToastError(c, "Tag not found")
		return c.Status(fiber.StatusNotFound).SendString("Tag not found")
	}

	// Delete the tag and its associations atomically.
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM post_tags WHERE tag_id = ?", tag.ID).Error; err != nil {
			return err
		}
		return tx.Delete(&tag).Error
	}); err != nil {
		ShowToastError(c, "Error deleting tag")
		return c.Status(fiber.StatusInternalServerError).SendString("Error deleting tag")
	}

	return ShowToast(c, "Tag with ID "+id+" deleted successfully")
}
