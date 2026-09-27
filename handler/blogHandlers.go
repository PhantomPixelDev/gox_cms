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
	}, AdminLayout)
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

// deletePostByID removes a post with its category/tag links and comments.
// Callers must run it inside a transaction.
func deletePostByID(tx *gorm.DB, id int) error {
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
}

// parseBulkIDs reads a comma-separated "ids" form value into valid IDs.
func parseBulkIDs(raw string) []uint {
	var ids []uint
	for _, part := range strings.Split(raw, ",") {
		n, err := strconv.ParseUint(strings.TrimSpace(part), 10, 64)
		if err != nil || n == 0 {
			continue
		}
		ids = append(ids, uint(n))
	}
	return ids
}

func AdminDeletePost(c *fiber.Ctx, db *gorm.DB) error {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid post ID")
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		return deletePostByID(tx, id)
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
	// Clamped: an unbounded /blog/99999999 turns into a full-table scan.
	pageNumber := clampPage(c.Params("page"))
	return renderPostList(c, db, "blog/blog", "Blog", pageNumber, nil)
}

// renderPostList renders a page of published posts, and is shared with the
// /frag/blog endpoint so the two can never disagree about the data contract.
//
// scope narrows the query to a category or tag; nil means the whole blog.
func renderPostList(c *fiber.Ctx, db *gorm.DB, view, title string, pageNumber int, scope *postScope) error {
	data, err := postListData(db, view, title, pageNumber, scope)
	if err != nil {
		return c.Status(500).SendString("Could not load posts")
	}
	// A page past the end redirects to the last page rather than rendering an
	// empty list, so a stale bookmark still shows something. Fragments must not
	// redirect: an hx-get that 302s swaps the response of the redirect target,
	// which is a full page, into the panel.
	if data["__overRange"] == true {
		return c.Redirect(scope.lastPageURL(data["TotalPagesInt"].(int)))
	}
	return RenderSite(c, view, data)
}

// postListData loads one page of posts and the pagination window.
//
// The over-range case is reported in the map as __overRange rather than
// redirecting, so a fragment can render an empty panel instead of triggering a
// navigation.
func postListData(db *gorm.DB, view, title string, pageNumber int, scope *postScope) (fiber.Map, error) {
	offset := (pageNumber - 1) * postsPerPage

	query := db.Preload("Categories").Preload("Tags").Model(&model.Post{})
	countQuery := db.Model(&model.Post{})
	if scope != nil {
		query = scope.apply(query)
		countQuery = scope.apply(countQuery)
	}
	query = query.Where("published = ?", true).Order("created_at DESC").
		Offset(offset).Limit(postsPerPage)
	countQuery = countQuery.Where("published = ?", true)

	var posts []model.Post
	if err := query.Find(&posts).Error; err != nil {
		return nil, err
	}

	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, err
	}
	totalPages := pageCount(total, postsPerPage)

	extra := fiber.Map{"Posts": posts}
	if scope != nil {
		extra["Slug"] = scope.slug
		extra["CategoryName"] = scope.name
		extra["TagName"] = scope.name
	}
	data := listDataNoCtx(title, pageNumber, totalPages, extra)
	if pageNumber > totalPages {
		data["__overRange"] = true
	}
	return data, nil
}

// postScope narrows a post list to one taxonomy term. It carries the redirect
// base too, so the out-of-range redirect matches the list it came from.
type postScope struct {
	join   string
	clause string
	args   []any
	slug   string
	name   string
	base   string
}

func (s *postScope) apply(q *gorm.DB) *gorm.DB {
	return q.Joins(s.join).Where(s.clause, s.args...)
}

// lastPageURL is where an out-of-range page number should send the visitor.
func (s *postScope) lastPageURL(page int) string {
	if s == nil {
		return "/blog/" + strconv.Itoa(page)
	}
	return s.base + "/" + strconv.Itoa(page)
}

// errNotFound distinguishes "no such post" from a database failure, so a
// broken link is a 404 rather than a 500 in the logs.
var errNotFound = errors.New("not found")

// postPageData assembles the data for a single post and its comments. Shared
// by the page handler and the /frag/post and /frag/comments fragments, so all
// three agree on the shape.
func postPageData(c *fiber.Ctx, db *gorm.DB) (fiber.Map, error) {
	slug := c.Params("slug")
	if slug == "" {
		return nil, errNotFound
	}

	var post model.Post
	// published is checked in Go below rather than in SQL so an admin can
	// preview a draft; the check is not skipped for an admin here, it is
	// applied after the load.
	if err := db.Preload("Categories").Preload("Tags").Where("slug = ?", slug).First(&post).Error; err != nil || post.ID == 0 {
		return nil, errNotFound
	}

	// An unpublished post is a 404 for everyone but an admin. This has to
	// happen before the comments are loaded, so a draft's comments are not
	// fetched for a visitor who is about to be refused.
	if !post.Published && !IsTrue(c, "isAdmin") {
		return nil, errNotFound
	}

	comments := []model.Comment{}
	if err := db.Preload("User").
		Where("post_id = ? AND status != ?", post.ID, "pending").
		Find(&comments).Error; err != nil {
		return nil, err
	}

	return fiber.Map{
		"UserID":     currentUserID(c),
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
	}, nil
}

func BlogPostPage(c *fiber.Ctx, db *gorm.DB) error {
	slug := c.Params("slug")
	if slug == "" {
		return c.Redirect("/blog")
	}

	data, err := postPageData(c, db)
	if err != nil {
		if err == errNotFound {
			return RenderNotFound(c)
		}
		return c.Status(500).SendString("Could not load post")
	}
	return RenderSite(c, "blog/blog_post", data)
}

func TogglePostStatus(c *fiber.Ctx, db *gorm.DB) error {

	id, ok := parseIDParam(c.FormValue("id"))
	if !ok {
		ShowToastError(c, "Invalid post ID")
		return c.Status(fiber.StatusBadRequest).SendString("Invalid post ID")
	}

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
