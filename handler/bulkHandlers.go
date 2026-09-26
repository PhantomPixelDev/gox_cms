package handlers

import (
	"goxcms/model"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// BulkPosts publishes, unpublishes or deletes several posts at once.
// Form: action=publish|unpublish|delete, ids=1,2,3
func BulkPosts(c *fiber.Ctx, db *gorm.DB) error {
	action := c.FormValue("action")
	ids := parseBulkIDs(c.FormValue("ids"))
	if len(ids) == 0 {
		ShowToastError(c, "Select at least one post")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Select at least one post"})
	}

	var affected int64
	switch action {
	case "publish", "unpublish":
		res := db.Model(&model.Post{}).Where("id IN ?", ids).Update("published", action == "publish")
		if res.Error != nil {
			ShowToastError(c, "Could not update posts")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not update posts"})
		}
		affected = res.RowsAffected
	case "delete":
		err := db.Transaction(func(tx *gorm.DB) error {
			for _, id := range ids {
				if err := deletePostByID(tx, int(id)); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			ShowToastError(c, "Could not delete posts")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not delete posts"})
		}
		affected = int64(len(ids))
	default:
		ShowToastError(c, "Unknown bulk action")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Unknown bulk action"})
	}

	ShowToast(c, "Bulk action done")
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"affected": affected})
}

// BulkComments approves or deletes several comments at once.
// Form: action=approve|delete, ids=1,2,3
func BulkComments(c *fiber.Ctx, db *gorm.DB) error {
	action := c.FormValue("action")
	ids := parseBulkIDs(c.FormValue("ids"))
	if len(ids) == 0 {
		ShowToastError(c, "Select at least one comment")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Select at least one comment"})
	}

	var res *gorm.DB
	switch action {
	case "approve":
		res = db.Model(&model.Comment{}).Where("id IN ?", ids).Update("status", "approved")
	case "delete":
		res = db.Where("id IN ?", ids).Delete(&model.Comment{})
	default:
		ShowToastError(c, "Unknown bulk action")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Unknown bulk action"})
	}
	if res.Error != nil {
		ShowToastError(c, "Could not update comments")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not update comments"})
	}

	ShowToast(c, "Bulk action done")
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"affected": res.RowsAffected})
}
