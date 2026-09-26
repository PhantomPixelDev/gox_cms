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
	var comment model.Comment

	// Extract comment data from the form
	comment.Content = SanitizeText(c.FormValue("comment"))
	postID, _ := strconv.Atoi(c.FormValue("post_id"))
	comment.PostID = uint(postID)
	userID, _ := strconv.Atoi(c.FormValue("user_id"))
	comment.UserID = uint(userID)

	comment.User = model.User{ID: comment.UserID}

	comment.Status = "pending"

	// Check if the user is authenticated and is the same user as in the form data
	if uid := currentUserID(c); uid == 0 || uint(userID) != uid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	// Validate the data
	err := db.Create(&comment).Error
	if err != nil {
		ShowToast(c, "Error creating comment"+err.Error())
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
	searchQuery := c.FormValue("query")
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
	commentID, _ := strconv.Atoi(c.Params("id"))
	var comment model.Comment
	db.First(&comment, commentID)

	if comment.Status == "approved" {
		comment.Status = "pending"
	} else {
		comment.Status = "approved"
	}

	db.Save(&comment)

	ShowToast(c, "Comment status changed successfully")

	return c.Render("partials/comment-status-button", comment)
}

func DeleteComment(c *fiber.Ctx, db *gorm.DB) error {
	commentID, _ := strconv.Atoi(c.Params("id"))
	var comment model.Comment
	db.First(&comment, commentID)

	db.Delete(&comment)

	ShowToast(c, "Comment deleted successfully")

	return c.SendStatus(fiber.StatusNoContent)

}
