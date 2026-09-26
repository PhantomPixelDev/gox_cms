package handlers

import (
	"goxcms/model"
	"html/template"
	"regexp"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"gorm.io/gorm"
)

func AddComment(c *fiber.Ctx, db *gorm.DB) error {

	if !captchaPassed(c) {
		return nil
	}
	if authBlocked(c.IP()) {
		ShowToastError(c, "Too many attempts, try again later")
		return c.Status(fiber.StatusTooManyRequests).SendString("Too many attempts")
	}
	// The author is always the logged-in user: a client-supplied user_id is
	// never trusted.
	uid := currentUserID(c)
	if uid == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	postID, err := strconv.Atoi(c.FormValue("post_id"))
	if err != nil || postID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid data",
		})
	}
	var post model.Post
	if err := db.First(&post, postID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Invalid data",
		})
	}

	content := SanitizeText(c.FormValue("comment"))
	if content == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid data",
		})
	}

	comment := model.Comment{
		Content: content,
		PostID:  uint(postID),
		UserID:  uid,
		User:    model.User{ID: uid},
		Status:  "pending",
	}

	// Validate the data
	if err := db.Create(&comment).Error; err != nil {
		ShowToastError(c, "Could not save comment")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid data",
		})
	}

	ShowToast(c, "Comment created successfully")

	htmlMessage := template.HTML("<div class='alert alert-success'>Comment created successfully</div>")
	htmlMessage += template.HTML("<div class='alert alert-info'>Your comment is pending approval</div>")

	return c.Status(fiber.StatusCreated).SendString(string(htmlMessage))
}

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// SanitizeText strips HTML tags and escapes what is left, so the result is
// safe to render as HTML.
func SanitizeText(input string) string {
	return template.HTMLEscapeString(htmlTagPattern.ReplaceAllString(input, ""))
}

func SearchCommentsView(c *fiber.Ctx, db *gorm.DB) error {
	var comments []model.Comment
	searchQuery := c.Query("query", c.FormValue("query"))
	page := queryPage(c)
	limit := 10
	offset := (page - 1) * limit

	db.Where("content LIKE ? ESCAPE '\\'", likePattern(searchQuery)).Limit(limit).Offset(offset).Find(&comments)

	currentPage := page

	/// count total comments for pagination ///
	var totalComments int64
	db.Model(&model.Comment{}).Where("content LIKE ? ESCAPE '\\'", likePattern(searchQuery)).Count(&totalComments)
	TotalPages := pageCount(totalComments, limit)
	if TotalPages == 0 {
		TotalPages = 1
	}

	return c.Render("admin/table/comments-table", fiber.Map{
		"Comments":    comments,
		"CurrentPage": currentPage,
		"TotalPages":  TotalPages,
		"SearchQuery": searchQuery,
	})
}

func ToggleCommentStatus(c *fiber.Ctx, db *gorm.DB) error {
	commentID, err := c.ParamsInt("id")
	if err != nil || commentID <= 0 {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid ID")
	}
	var comment model.Comment
	if err := db.First(&comment, commentID).Error; err != nil {
		ShowToastError(c, "Comment not found")
		return c.Status(fiber.StatusNotFound).SendString("Comment not found")
	}

	if comment.Status == "approved" {
		comment.Status = "pending"
	} else {
		comment.Status = "approved"
	}

	if err := db.Save(&comment).Error; err != nil {
		ShowToastError(c, "Could not update comment")
		return c.Status(fiber.StatusInternalServerError).SendString("Could not update comment")
	}

	ShowToast(c, "Comment status changed successfully")

	return c.Render("partials/comment-status-button", comment)
}

func DeleteComment(c *fiber.Ctx, db *gorm.DB) error {
	commentID, err := c.ParamsInt("id")
	if err != nil || commentID <= 0 {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid ID")
	}
	result := db.Delete(&model.Comment{}, commentID)
	if result.Error != nil {
		ShowToastError(c, "Could not delete comment")
		return c.Status(fiber.StatusInternalServerError).SendString("Could not delete comment")
	}
	if result.RowsAffected == 0 {
		ShowToastError(c, "Comment not found")
		return c.Status(fiber.StatusNotFound).SendString("Comment not found")
	}

	ShowToast(c, "Comment deleted successfully")

	return c.SendStatus(fiber.StatusNoContent)

}
