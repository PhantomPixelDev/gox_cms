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
	var comment model.Comment

	// Extract comment data from the form
	comment.Content = sanitizeHTML(c.FormValue("comment"))
	postID, _ := strconv.Atoi(c.FormValue("post_id"))
	comment.PostID = uint(postID)
	userID, _ := strconv.Atoi(c.FormValue("user_id"))
	comment.UserID = uint(userID)

	comment.User = model.User{ID: comment.UserID}

	comment.Status = "pending"

	// Check if the user is authenticated and is the same user as in the form data
	if uint(userID) != c.Locals("user").(model.User).ID {
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

// SanitizeHTML sanitizes the HTML input to prevent XSS attacks
func sanitizeHTML(input string) string {
	// Remove any HTML tags and attributes
	sanitized := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(input, "")

	// Replace special characters with their HTML entities
	sanitized = template.HTMLEscapeString(sanitized)

	return sanitized
}

func SearchCommentsView(c *fiber.Ctx, db *gorm.DB) error {
	var comments []model.Comment
	searchQuery := c.FormValue("query")
	page, err := strconv.Atoi(c.FormValue("page", "1"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid page number",
		})
	}
	limit := 10
	offset := (page - 1) * limit

	db.Where("content LIKE ?", "%"+searchQuery+"%").Limit(limit).Offset(offset).Find(&comments)

	currentPage := page

	/// count total comments for pagination ///
	var totalComments int64
	db.Model(&model.Comment{}).Where("content LIKE ?", "%"+searchQuery+"%").Count(&totalComments)
	TotalPages := int(totalComments / int64(limit))

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
	button := GetCommentStatusButton(comment)
	button = string(template.HTML(button)) // convert to string

	ShowToast(c, "Comment status changed successfully")

	return c.SendString(button)
	/// return the button with the new status
}

func GetCommentStatusButton(comment model.Comment) string {
	var button string

	commentID := strconv.Itoa(int(comment.ID))

	if comment.Status == "approved" {
		button = `<button id="comment-status-button-` + commentID + `"
					class="btn btn-secondary"
					hx-post="/toggle-comment-status/` + commentID + `" 
					hx-target="#comment-status-button-` + commentID + `" hx-headers='{"X-No-Cache": "true"}'
					hx-confirm="Are you sure you want to change the status of this comment?"
					hx-swap="outerHTML">
					Unapprove
				</button>`
	} else {
		button = `<button id="comment-status-button-` + commentID + `"
					class="btn btn-success"
					hx-post="/toggle-comment-status/` + commentID + `" 
					hx-target="#comment-status-button-` + commentID + `" hx-headers='{"X-No-Cache": "true"}'
					hx-confirm="Are you sure you want to change the status of this comment?"
					hx-swap="outerHTML">
					Approve
				</button>`
	}

	return button
}

func DeleteComment(c *fiber.Ctx, db *gorm.DB) error {
	commentID, _ := strconv.Atoi(c.Params("id"))
	var comment model.Comment
	db.First(&comment, commentID)

	db.Delete(&comment)

	ShowToast(c, "Comment deleted successfully")

	return c.SendStatus(fiber.StatusNoContent)

}
