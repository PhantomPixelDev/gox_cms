package handlers

import (
	"goxcms/model"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func SearchUsers(c *fiber.Ctx, db *gorm.DB) error {
	var users []model.User
	searchQuery := c.Query("query")
	pageSize := 10

	pageInt := queryPage(c)

	db.Where("username LIKE ? ESCAPE '\\'", likePattern(searchQuery)).
		Limit(pageSize).
		Offset((pageInt - 1) * pageSize).
		Find(&users)

	var count int64
	db.Model(&model.User{}).
		Where("username LIKE ? ESCAPE '\\'", likePattern(searchQuery)).
		Count(&count)
	totalPages := pageCount(count, pageSize)

	return c.Render("admin/table/user-table", fiber.Map{
		"Users":       users,
		"TotalPages":  totalPages,
		"CurrentPage": pageInt,
		"SearchQuery": searchQuery,
	})
}

func DeleteUser(c *fiber.Ctx, db *gorm.DB) error {
	id := c.Params("id")
	var user model.User

	current_user, _ := CurrentUser(c)

	idUint, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		ShowToastError(c, "Invalid user ID")
		return c.Status(fiber.StatusBadRequest).SendString("Invalid user ID")
	}

	if current_user.ID == uint(idUint) {
		ShowToastError(c, "You cannot delete yourself")
		return c.Status(fiber.StatusBadRequest).SendString("You cannot delete yourself")
	}

	if err := db.First(&user, id).Error; err != nil {
		ShowToastError(c, "User not found")
		return c.Status(fiber.StatusNotFound).SendString("User not found")
	}

	// Never remove the last administrator: there would be no way back in.
	if user.RoleID == model.RoleAdmin {
		var admins int64
		db.Model(&model.User{}).Where("role_id = ?", model.RoleAdmin).Count(&admins)
		if admins <= 1 {
			ShowToastError(c, "You cannot delete the last administrator")
			return c.Status(fiber.StatusBadRequest).SendString("You cannot delete the last administrator")
		}
	}

	if err := db.Delete(&user).Error; err != nil {
		ShowToastError(c, "Could not delete user")
		return c.Status(fiber.StatusInternalServerError).SendString("Could not delete user")
	}

	c.Status(fiber.StatusOK)

	ShowToast(c, "User deleted successfully")

	return nil
}
