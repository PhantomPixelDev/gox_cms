package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"goxcms/model"
	"html/template"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// postForm holds the fields shared by the add and edit post forms.
type postForm struct {
	title, content, slug, image string
	categoryIDs, tagIDs         []uint
}

// parsePostForm reads and validates the post form. On failure it writes the
// response and returns false.
func parsePostForm(c *fiber.Ctx) (postForm, bool) {
	f := postForm{
		title:       strings.TrimSpace(c.FormValue("title")),
		content:     SanitizeRichHTML(c.FormValue("content")),
		slug:        strings.TrimSpace(c.FormValue("post_slug")),
		image:       strings.TrimSpace(c.FormValue("image")),
		categoryIDs: extractIDs(c.FormValue("categories_input")),
		tagIDs:      extractIDs(c.FormValue("tags_input")),
	}

	if f.title == "" || f.content == "" || f.slug == "" {
		ShowToastError(c, "Missing required fields: title, content, slug")
		c.SendString("Missing required fields: title, content, slug")
		return f, false
	}
	if f.image == "" {
		ShowToastError(c, "Missing required fields: image")
		c.SendString("Missing required fields: image")
		return f, false
	}
	return f, true
}

var errSlugTaken = errors.New("slug is already used by another post")

// loadPostRelations fetches the selected categories and tags. Unknown IDs
// are rejected instead of silently dropping them.
func loadPostRelations(tx *gorm.DB, f postForm) ([]model.Category, []model.Tag, error) {
	var categories []model.Category
	if len(f.categoryIDs) > 0 {
		if err := tx.Find(&categories, f.categoryIDs).Error; err != nil {
			return nil, nil, fmt.Errorf("fetching categories: %w", err)
		}
		if len(categories) != len(f.categoryIDs) {
			return nil, nil, errors.New("unknown category selected")
		}
	}

	var tags []model.Tag
	if len(f.tagIDs) > 0 {
		if err := tx.Find(&tags, f.tagIDs).Error; err != nil {
			return nil, nil, fmt.Errorf("fetching tags: %w", err)
		}
		if len(tags) != len(f.tagIDs) {
			return nil, nil, errors.New("unknown tag selected")
		}
	}
	return categories, tags, nil
}

// postSlugTaken reports whether another post already uses slug.
func postSlugTaken(tx *gorm.DB, slug string, excludeID uint) (bool, error) {
	var count int64
	err := tx.Model(&model.Post{}).Where("slug = ? AND id <> ?", slug, excludeID).Count(&count).Error
	return count > 0, err
}

func AdminAddBlogPost(c *fiber.Ctx, db *gorm.DB) error {
	f, ok := parsePostForm(c)
	if !ok {
		return nil
	}

	post := model.Post{
		Title:    f.title,
		Content:  f.content,
		Slug:     f.slug,
		ImageURL: f.image,
		UserID:   currentUserID(c),
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if taken, err := postSlugTaken(tx, f.slug, 0); err != nil {
			return err
		} else if taken {
			return errSlugTaken
		}

		categories, tags, err := loadPostRelations(tx, f)
		if err != nil {
			return err
		}
		post.Categories, post.Tags = categories, tags

		if err := tx.Create(&post).Error; err != nil {
			if isDupKeyError(err) {
				return errSlugTaken
			}
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errSlugTaken) {
			ShowToastError(c, "Post creation failed: "+err.Error())
			return c.SendString("Post creation failed: " + err.Error())
		}
		log.Printf("creating post: %v", err)
		ShowToastError(c, "Post creation failed")
		return c.SendString("Post creation failed")
	}

	message := map[string]string{"showToast": "Post created successfully", "clearForm": "true"}
	messageBytes, _ := json.Marshal(message)
	c.Set("HX-Trigger", string(messageBytes))

	return c.Render("partials/post-created", post)
}

func AdminEditBlogPost(c *fiber.Ctx, db *gorm.DB) error {
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

	selectedIDs := func(items []model.Category) string {
		ids := make([]string, 0, len(items))
		for _, item := range items {
			ids = append(ids, strconv.Itoa(int(item.ID)))
		}
		return strings.Join(ids, ",")
	}
	selectedTagIDs := func(items []model.Tag) string {
		ids := make([]string, 0, len(items))
		for _, item := range items {
			ids = append(ids, strconv.Itoa(int(item.ID)))
		}
		return strings.Join(ids, ",")
	}

	/// convert post ID so can print in Template as string
	postIDStr := strconv.Itoa(int(post.ID))

	return c.Render("admin/post/post_edit", fiber.Map{
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
		"PostCatIDs":  selectedIDs(post.Categories),
		"PostTagIDs":  selectedTagIDs(post.Tags),
		"IsAdmin":     c.Locals("isAdmin"),
		"IsLoggedIn":  c.Locals("isLoggedin"),
		"Settings":    c.Locals("Settings"),
	}, "main")
}

func AdminUpdateBlogPost(c *fiber.Ctx, db *gorm.DB) error {
	postID, err := strconv.ParseUint(c.FormValue("id"), 10, 64)
	if err != nil || postID == 0 {
		ShowToastError(c, "Missing required fields: post_id")
		return c.SendString("Missing required fields: post_id")
	}

	f, ok := parsePostForm(c)
	if !ok {
		return nil
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		var post model.Post
		if err := tx.First(&post, postID).Error; err != nil {
			return fmt.Errorf("fetching post: %w", err)
		}

		if taken, err := postSlugTaken(tx, f.slug, post.ID); err != nil {
			return err
		} else if taken {
			return errSlugTaken
		}

		categories, tags, err := loadPostRelations(tx, f)
		if err != nil {
			return err
		}

		post.Title = f.title
		post.Content = f.content
		post.Slug = f.slug
		post.ImageURL = f.image

		if err := tx.Omit(clause.Associations).Save(&post).Error; err != nil {
			if isDupKeyError(err) {
				return errSlugTaken
			}
			return err
		}
		if err := tx.Model(&post).Association("Categories").Replace(categories); err != nil {
			return fmt.Errorf("updating categories: %w", err)
		}
		if err := tx.Model(&post).Association("Tags").Replace(tags); err != nil {
			return fmt.Errorf("updating tags: %w", err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errSlugTaken) {
			ShowToastError(c, "Post update failed: "+err.Error())
			return c.SendString("Post update failed: " + err.Error())
		}
		log.Printf("updating post: %v", err)
		ShowToastError(c, "Post update failed")
		return c.SendString("Post update failed")
	}

	message := map[string]string{"showToast": "Post updated successfully", "clearForm": "true"}
	messageBytes, _ := json.Marshal(message)
	c.Set("HX-Trigger", string(messageBytes))

	return c.SendString("Post updated successfully")
}

func AdminSearchPosts(c *fiber.Ctx, db *gorm.DB) error {
	var posts []model.Post
	searchQuery := c.Query("query")
	pageSize := 10 // Or whatever your default page size is

	// Convert page string to int
	pageInt := queryPage(c)

	// Implement search logic with pagination
	db.Preload("Categories").Preload("Tags").
		Where("title LIKE ? ESCAPE '\\'", likePattern(searchQuery)).
		Order("created_at desc").
		Limit(pageSize).
		Offset((pageInt - 1) * pageSize).
		Find(&posts)

	// Calculate total pages
	var count int64
	db.Model(&model.Post{}).
		Where("title LIKE ? ESCAPE '\\'", likePattern(searchQuery)).
		Count(&count)
	totalPages := pageCount(count, pageSize)

	return c.Render("admin/table/post-table", fiber.Map{
		"Posts":       posts,
		"TotalPages":  totalPages,
		"CurrentPage": pageInt,
		// Indicate that this is a search result
		"SearchQuery": searchQuery, // Pass the current search query
	})
}

func AdminDeletePost(c *fiber.Ctx, db *gorm.DB) error {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid post ID")
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		var post model.Post
		if err := tx.First(&post, id).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM post_categories WHERE post_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM post_tags WHERE post_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Where("post_id = ?", id).Delete(&model.Comment{}).Error; err != nil {
			return err
		}
		return tx.Delete(&post).Error
	})
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			ShowToastError(c, "Post not found")
			return c.Status(fiber.StatusNotFound).SendString("Post not found")
		}
		ShowToastError(c, "Could not delete post")
		return c.Status(fiber.StatusInternalServerError).SendString("Could not delete post")
	}

	c.Status(fiber.StatusOK)

	message := map[string]string{"showToast": "Post with ID " + strconv.Itoa(id) + " deleted successfully"}
	messageBytes, _ := json.Marshal(message)
	c.Set("HX-Trigger", string(messageBytes))

	return nil
}

func BlogPage(c *fiber.Ctx, db *gorm.DB) error {

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
		log.Printf("blog list: %v", result.Error)
		return c.Status(500).SendString("Could not load posts")
	}

	var totalPosts int64
	result = db.Model(&model.Post{}).Where("published = ?", true).Count(&totalPosts)
	if result.Error != nil {
		log.Printf("blog count: %v", result.Error)
		return c.Status(500).SendString("Could not load posts")
	}

	totalPages := 1
	if totalPosts > 0 {
		totalPages = pageCount(totalPosts, postsPerPage)
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

	userID := currentUserID(c)

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
	if !post.Published && !IsTrue(c, "isAdmin") {
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
		ShowToastError(c, "Post not found")
		return c.Status(fiber.StatusNotFound).SendString("Post not found")
	}

	newStatus := !post.Published
	if err := db.Model(&post).Update("published", newStatus).Error; err != nil {
		ShowToastError(c, "Error updating post status")
		return c.Status(fiber.StatusInternalServerError).SendString("Error updating post status")
	}

	post.Published = newStatus
	if newStatus {
		ShowToast(c, "Post published successfully")
	} else {
		ShowToast(c, "Post unpublished successfully")
	}

	return c.Render("partials/post-status-button", post)
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

// currentUserID returns the ID of the logged-in user, or 0 for anonymous
// requests.
func currentUserID(c *fiber.Ctx) uint {
	if user, ok := CurrentUser(c); ok {
		return user.ID
	}
	return 0
}
