package handlers

import (
	"goxcms/model"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

const (
	MaxFileSize = 10 * 1024 * 1024 // 10 MB
	UploadDir   = "./static/uploads"
)

func UploadFile(c *fiber.Ctx, db *gorm.DB) error {

	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Cannot read file" + err.Error())
	}

	// Validate file size
	if file.Size > MaxFileSize {
		return c.Status(fiber.StatusBadRequest).SendString("File size exceeds the limit")
	}

	// Validate file type
	fileType := filepath.Ext(file.Filename)
	allowedTypes := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
	}

	if !allowedTypes[fileType] {
		return c.Status(fiber.StatusBadRequest).SendString("File type not allowed")
	}

	validContentTypes := []string{"image/jpeg", "image/png", "image/gif", "image/jpg"}
	contentType := file.Header.Get("Content-Type")
	isValidContentType := false
	for _, validType := range validContentTypes {
		if validType == contentType {
			isValidContentType = true
			break
		}
	}

	if !isValidContentType {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid content type")
	}

	// Generate a random string for the filename
	randomString := generateRandomFilenameString(10)

	filename := randomString + file.Filename // Generate a random filename

	// Save the file to the disk
	if err := c.SaveFile(file, filepath.Join(UploadDir, filename)); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Cannot save file to disk")
	}

	// Represent the file in the database
	fileModel := model.File{
		Name: filename,
		Path: "/static/uploads/" + filename, // Save the path to the file
	}

	// Save file reference to the database
	if err := db.Create(&fileModel).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Cannot save file to database")
	}

	ShowToast(c, "File uploaded successfully")

	c.Status(fiber.StatusOK)

	return nil

}

func generateRandomFilenameString(length int) string {
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func DeleteFile(c *fiber.Ctx, db *gorm.DB) error {
	filename := c.FormValue("name")
	safeFilename := filepath.Base(filename)
	uploadDir := "./static/uploads"
	filePath := filepath.Join(uploadDir, safeFilename)
	// Check if the file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "File not found"})
	}

	// Delete the file from disk
	if err := os.Remove(filePath); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete file from disk"})
	}

	// Delete the file from the database
	if err := db.Delete(&model.File{}, "name = ?", safeFilename).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete file from database"})
	}

	ShowToast(c, "File deleted successfully")

	return c.SendStatus(fiber.StatusOK)

}

func SearchFiles(c *fiber.Ctx, db *gorm.DB) error {

	var files []model.File
	searchQuery := c.Query("query")
	page := c.Query("page", "1")
	//convert page to int
	pageInt, err := strconv.Atoi(page)
	if err != nil || pageInt < 1 {
		pageInt = 1
	}
	pageSize := 10 // Default page size

	db.Where("name LIKE ?", "%"+searchQuery+"%").
		Order("created_at DESC"). // Sort by created_at column in descending order
		Limit(pageSize).
		Offset((pageInt - 1) * pageSize).
		Find(&files)

	var totalMatchingCount int64
	db.Model(&model.File{}).
		Where("name LIKE ?", "%"+searchQuery+"%").
		Count(&totalMatchingCount)
	totalPages := int(math.Ceil(float64(totalMatchingCount) / float64(pageSize)))

	//// render data as HTML and send it to the client using HTMX
	return c.Render("partials/file-manager", fiber.Map{
		"Files":       files,
		"TotalPages":  totalPages,
		"CurrentPage": pageInt,
		"SearchQuery": searchQuery,
	})
}
