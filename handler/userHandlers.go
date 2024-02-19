package handlers

import (
	"goxcms/model"
	"math"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func SearchUsers(c *fiber.Ctx, db *gorm.DB) error {
	var users []model.User
	searchQuery := c.Query("query")
	page := c.Query("page", "1")
	pageSize := 10

	pageInt, err := strconv.Atoi(page)

	if err != nil || pageInt < 1 {
		pageInt = 1
	}

	db.Where("username LIKE ?", "%"+searchQuery+"%").
		Limit(pageSize).
		Offset((pageInt - 1) * pageSize).
		Find(&users)

	var count int64
	db.Model(&model.User{}).
		Where("username LIKE ?", "%"+searchQuery+"%").
		Count(&count)
	totalPages := int(math.Ceil(float64(count) / float64(pageSize)))

	return c.Render("admin/admin-user-table", fiber.Map{
		"Users":       users,
		"TotalPages":  totalPages,
		"CurrentPage": pageInt,
		"SearchQuery": searchQuery,
	})
}

func DeleteUser(c *fiber.Ctx, db *gorm.DB) error {
	id := c.Params("id")
	var user model.User

	current_user := c.Locals("user").(model.User)

	idUint, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return err
	}

	if current_user.ID == uint(idUint) {
		return c.Status(fiber.StatusBadRequest).SendString("You cannot delete yourself")
	}

	if err := db.First(&user, id).Error; err != nil {
		return err
	}

	if err := db.Delete(&user).Error; err != nil {
		return err
	}

	c.Status(fiber.StatusOK)

	ShowToast(c, "User deleted successfully")

	return nil
}
