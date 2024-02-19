package handlers

import (
	"encoding/json"

	"goxcms/model"
	"html/template"
	"strconv"
	"strings"

	"math"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Register handles the registration process
func AdminAddBlogPost(c *fiber.Ctx, db *gorm.DB) error {
	// Parse form values with validation
	title, content, slug := c.FormValue("title"), c.FormValue("content"), c.FormValue("post_slug")

	if title == "" || content == "" || slug == "" {
		return c.SendString("Missing required fields: title, content, slug" + title + content + slug) // Show toast error
	}

	image := c.FormValue("image")
	if image == "" {
		ShowToastError(c, "Missing required fields: image")
		return c.SendString("Missing required fields: image") // Show toast error
	}

	categoryIDs, tagIDs := extractIDs(c.FormValue("categories_input")), extractIDs(c.FormValue("tags_input"))

	// Start a transaction
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil || tx.Error != nil {
			tx.Rollback()
		}
	}()

	// Fetch categories and tags from the database
	var categories []model.Category
	if err := tx.Find(&categories, categoryIDs).Error; err != nil {
		return c.SendString("Error fetching categories: " + err.Error())
	}

	var tags []model.Tag
	if err := tx.Find(&tags, tagIDs).Error; err != nil {
		return c.SendString("Error fetching tags: " + err.Error())
	}

	// Create a new Post instance
	post := model.Post{
		Title: title, Content: content, Slug: slug,
		ImageURL:   image,
		UserID:     c.Locals("user").(model.User).ID,
		Categories: categories, Tags: tags,
	}

	if err := tx.Create(&post).Error; err != nil {
		return c.SendString("Post creation failed: " + err.Error())
	}

	tx.Commit()
	if tx.Error != nil {
		return c.SendString("Transaction commit failed: " + tx.Error.Error())
	}

	postID := strconv.Itoa(int(post.ID))

	message := map[string]string{"showToast": "Settings updated successfully", "clearForm": "true"}
	messageBytes, _ := json.Marshal(message)
	c.Set("HX-Trigger", string(messageBytes))

	button_show_post_and_edit_post_html := `
	
		<div class="alert alert-success alert-dismissible fade show" role="alert">
			<span class="alert-icon"><i class="ni ni-like-2"></i></span>
			
			<h4 class="alert-heading">Post Created!</h4>
			
			<p class="mb-0">Post ID: ` + postID + `</p>
			<p class="mb-0">Post Slug: ` + slug + `</p>
			<p class="mb-0">Post Title: ` + title + `</p>
			<hr> 
			<a href="/blog/post/` + slug + `" class="btn btn-sm btn-success">Show Post</a>
			<a href="/admin/post/edit/` + postID + `" class="btn btn-sm btn-primary">Edit Post</a>
			<button type="button" class="btn-close" data-bs-dismiss="alert" aria-label="Close"></button>
		</div>
		`

	return c.SendString(button_show_post_and_edit_post_html)

}

func AdminEditBlogPost(c *fiber.Ctx, db *gorm.DB) error {
	if c.Locals("isAdmin") == false || c.Locals("isLoggedin") == false {
		c.Redirect("/")
		return nil
	}

	postID, _ := c.ParamsInt("post_id")

	var post model.Post
	db.Preload("Categories").Preload("Tags").First(&post, postID)

	var categories []model.Category
	var tags []model.Tag

	db.Find(&categories)
	db.Find(&tags)

	//// connvert categories and tags to json string
	postCategories, _ := json.Marshal(post.Categories)
	postTags, _ := json.Marshal(post.Tags)

	/// convert post ID so can print in Template as string
	postIDStr := strconv.Itoa(int(post.ID))

	return c.Render("admin/admin_post_edit", fiber.Map{
		"Title":       "Edit Post",
		"PostTitle":   post.Title,
		"PostSlug":    post.Slug,
		"PostImage":   post.ImageURL,
		"PostID":      postIDStr,
		"Published":   post.Published,
		"PostContent": template.HTML(post.Content),
		"Categories":  categories,
		"Tags":        tags,
		"PostTags":    template.JS(postTags),
		"PostCats":    template.JS(postCategories),
		"IsAdmin":     c.Locals("isAdmin"),
		"IsLoggedIn":  c.Locals("isLoggedin"),
		"Settings":    c.Locals("Settings"),
	}, "main")
}

func AdminUpdateBlogPost(c *fiber.Ctx, db *gorm.DB) error {
	postID := c.FormValue("id")

	// Parse form values with validation
	if postID == "" || postID == "0" {
		return c.SendString("Missing required fields: post_id") // Show toast error
	}

	title, content, slug := c.FormValue("title"), c.FormValue("content"), c.FormValue("post_slug")

	if title == "" || content == "" || slug == "" {
		return c.SendString("Missing required fields: title, content, slug") // Show toast error
	}

	image := c.FormValue("image")

	if image == "" {
		ShowToast(c, "Missing required fields: image")
		return c.SendString("Missing required fields: image") // Show toast error
	}

	categoryIDs, tagIDs := extractIDs(c.FormValue("categories_input")), extractIDs(c.FormValue("tags_input")) // c.FormValue("categories"), c.FormValue("tags")

	// Start a transaction
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil || tx.Error != nil {
			tx.Rollback()
		}
	}()

	// Fetch the existing post from the database
	var post model.Post
	if err := tx.Preload("Categories").Preload("Tags").First(&post, postID).Error; err != nil {
		return c.SendString("Error fetching post: " + err.Error())
	}

	// Update the post with the new values
	post.Title = title
	post.Content = content
	post.Slug = slug
	post.ImageURL = image

	// Fetch categories and tags from the database
	var categories []model.Category
	if err := tx.Find(&categories, categoryIDs).Error; err != nil {
		return c.SendString("Error fetching categories: " + err.Error())
	}

	var tags []model.Tag
	if err := tx.Find(&tags, tagIDs).Error; err != nil {
		return c.SendString("Error fetching tags: " + err.Error())
	}

	// Assign the fetched categories and tags to the post
	post.Categories = categories
	post.Tags = tags

	// Check if slug is unique
	var postWithSlug model.Post
	if err := tx.Where("slug = ? AND id != ?", slug, postID).First(&postWithSlug).Error; err != nil {
		if err.Error() != "record not found" {
			return c.SendString("Error checking if slug is unique: " + err.Error())
		}
		if postWithSlug.ID != 0 {
			return c.SendString("Slug is not unique")
		}
	}

	// Update the post in the database
	if err := tx.Save(&post).Error; err != nil {
		return c.SendString("Post update failed: " + err.Error())
	}

	// Update the post's categories and tags in the database
	if err := tx.Model(&post).Association("Categories").Replace(&categories); err != nil {
		return c.SendString("Error updating post's categories: " + err.Error())
	}

	if err := tx.Model(&post).Association("Tags").Replace(&tags); err != nil {
		return c.SendString("Error updating post's tags: " + err.Error())
	}

	tx.Commit()
	if tx.Error != nil {
		return c.SendString("Transaction commit failed: " + tx.Error.Error())
	}

	message := map[string]string{"showToast": "Post updated successfully", "clearForm": "true"}
	messageBytes, _ := json.Marshal(message)
	c.Set("HX-Trigger", string(messageBytes))

	return c.SendString("Post updated successfully")
}

func AdminSearchPosts(c *fiber.Ctx, db *gorm.DB) error {
	var posts []model.Post
	searchQuery := c.Query("query")
	page := c.Query("page", "1")
	pageSize := 10 // Or whatever your default page size is

	// Convert page string to int
	pageInt, err := strconv.Atoi(page)
	if err != nil || pageInt < 1 {
		pageInt = 1
	}

	// Implement search logic with pagination
	db.Preload("Categories").Preload("Tags").
		Where("title LIKE ?", "%"+searchQuery+"%").
		Order("created_at desc").
		Limit(pageSize).
		Offset((pageInt - 1) * pageSize).
		Find(&posts)

	// Calculate total pages
	var count int64
	db.Model(&model.Post{}).
		Where("title LIKE ?", "%"+searchQuery+"%").
		Count(&count)
	totalPages := int(math.Ceil(float64(count) / float64(pageSize)))

	return c.Render("admin/admin-post-table", fiber.Map{
		"Posts":       posts,
		"TotalPages":  totalPages,
		"CurrentPage": pageInt,
		// Indicate that this is a search result
		"SearchQuery": searchQuery, // Pass the current search query
	})
}

func AdminDeletePost(c *fiber.Ctx, db *gorm.DB) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}

	var post model.Post

	db.Preload("Categories").Preload("Tags").First(&post, id)

	for _, category := range post.Categories {
		db.Model(&category).Association("Posts").Delete(&post)
	}

	for _, tag := range post.Tags {
		db.Model(&tag).Association("Posts").Delete(&post)
	}

	db.Delete(&post)

	c.Status(fiber.StatusOK)

	message := map[string]string{"showToast": "Post with ID " + strconv.Itoa(id) + " deleted successfully"}
	messageBytes, _ := json.Marshal(message)
	c.Set("HX-Trigger", string(messageBytes))

	return nil
}

func BlogPage(c *fiber.Ctx, db *gorm.DB) error {

	/// set header for cache X-No-Cache to prevent caching
	c.Set("X-No-Cache", "true")

	page := c.Params("page")
	if page == "" {
		page = "1"
	}

	pageNumber, err := strconv.Atoi(page)

	if err != nil || pageNumber < 1 {
		pageNumber = 1
	}

	postsPerPage := 10

	offset := (pageNumber - 1) * postsPerPage

	var posts []model.Post
	result := db.Preload("Categories").Preload("Tags").Where("published = ?", true).Offset(offset).Limit(postsPerPage).Find(&posts)
	if result.Error != nil {
		return c.Status(500).SendString(result.Error.Error())
	}

	var totalPosts int64
	result = db.Model(&model.Post{}).Where("published = ?", true).Count(&totalPosts)
	if result.Error != nil {
		return c.Status(500).SendString(result.Error.Error())
	}

	totalPages := 1
	if totalPosts > 0 {
		totalPages = int(math.Ceil(float64(totalPosts) / float64(postsPerPage)))
	}

	var totalPagesArray []int
	for i := 1; i <= totalPages; i++ {
		totalPagesArray = append(totalPagesArray, i)
	}

	if pageNumber > totalPages {
		return c.Redirect("/blog/1")
	}

	return c.Render("blog/blog", fiber.Map{
		"Title":         "Blog",
		"Posts":         posts,
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

func BlogPostPage(c *fiber.Ctx, db *gorm.DB) error {
	slug := c.Params("slug")

	/// get current loged in userID if any
	userID := uint(0)
	if c.Locals("isLoggedin") == true {

		userID = c.Locals("user").(model.User).ID

	}

	if slug == "" {
		return c.Redirect("/blog")
	}

	var post model.Post
	result := db.Preload("Categories").Preload("Tags").Where("Slug = ?", slug).First(&post)
	if result.Error != nil || post.ID == 0 {
		return c.Status(404).Render("404", fiber.Map{
			"Title":    "404",
			"Settings": c.Locals("Settings"),
		}, "main")
	}

	comments := []model.Comment{}
	db.Preload("User").Where("post_id = ? AND status != ?", post.ID, "pending").Find(&comments)

	// Handle unpublished posts
	if !post.Published && !c.Locals("isAdmin").(bool) {
		return c.Status(404).Render("404", fiber.Map{
			"Title":    "404",
			"Settings": c.Locals("Settings"),
		}, "main")
	}

	return c.Render("blog/blog_post", fiber.Map{
		"UserID":     userID,
		"Title":      post.Title,
		"Post":       post,
		"Comments":   comments,
		"Content":    template.HTML(post.Content),
		"Tags":       post.Tags,
		"Categories": post.Categories,
		"CreatedAt":  post.CreatedAt,
		"IsAdmin":    c.Locals("isAdmin"),
		"IsLoggedIn": c.Locals("isLoggedin"),
		"Settings":   c.Locals("Settings"),
	}, "main")
}

func TogglePostStatus(c *fiber.Ctx, db *gorm.DB) error {

	id := c.FormValue("id")

	var post model.Post

	if err := db.First(&post, id).Error; err != nil {
		return ShowToastError(c, "Post not found")
	}

	newStatus := !post.Published
	if err := db.Model(&post).Update("published", newStatus).Error; err != nil {
		return ShowToastError(c, "Error updating post status")
	}

	if newStatus {
		ShowToastError(c, "Post published successfully")
		button_unpublish_html := `
		<button id="post-status-button-` + id + `"
		class="btn btn-secondary"
		hx-post="/toggle-post-status"
		hx-vals='{"id": "` + id + `"}'
		hx-target="#post-status-button-` + id + `"
		hx-swap="outerHTML"
		hx-confirm="Are you sure you want to change the status of this post?">
		Unpublish
		</button>
		`
		return c.SendString(button_unpublish_html)

	}
	ShowToastError(c, "Post unpublished successfully")
	button_unpublish_html := `
	<button id="post-status-button-` + id + `"
	class="btn btn-success"
	hx-post="/toggle-post-status"
	hx-vals='{"id": "` + id + `"}'
	hx-target="#post-status-button-` + id + `"
	hx-swap="outerHTML"
	hx-confirm="Are you sure you want to change the status of this post?">
	Publish
	</button>
	`
	return c.SendString(button_unpublish_html)

}

func extractIDs(ids string) []uint {
	var idList []uint

	if ids == "" {
		return idList
	}

	idStrings := strings.Split(ids, ",")

	for _, id := range idStrings {
		uintID, err := strconv.ParseUint(id, 10, 32)
		if err != nil {
			continue
		}

		idList = append(idList, uint(uintID))
	}

	return idList
}
