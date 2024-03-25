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
        return c.Status(fiber.StatusBadRequest).SendString("Cannot read file: " + err.Error())
    }

    // Validate file size
    if file.Size > MaxFileSize {
        return c.Status(fiber.StatusBadRequest).SendString("File size exceeds the limit")
    }

    // Validate file type based on extension
    fileType := filepath.Ext(file.Filename)
    if !isValidFileType(fileType) {
        return c.Status(fiber.StatusBadRequest).SendString("File type not allowed")
    }

    // Check file content type from header
    if !isValidContentType(file.Header.Get("Content-Type")) {
        return c.Status(fiber.StatusBadRequest).SendString("Invalid content type")
    }

    // Generate a random string for the filename to ensure uniqueness
    randomString := generateRandomFilenameString(10)
    filename := randomString + fileType

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
	c.SendStatus(fiber.StatusOK)
	ShowToast(c, "File uploaded successfully") 
    // Respond with success message
    return nil
}

// Check if file type is allowed
func isValidFileType(fileType string) bool {
    allowedTypes := map[string]bool{
        ".jpg":  true,
        ".jpeg": true,
        ".png":  true,
        ".gif":  true,
    }
    return allowedTypes[fileType]
}

// Check if content type is allowed
func isValidContentType(contentType string) bool {
    validContentTypes := []string{"image/jpeg", "image/png", "image/gif", "image/jpg"}
    for _, validType := range validContentTypes {
        if validType == contentType {
            return true
        }
    }
    return false
}

// Generate random filename string
func generateRandomFilenameString(length int) string {
    const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    randomString := make([]byte, length)
    for i := range randomString {
        randomString[i] = charset[rand.Intn(len(charset))]
    }
    return string(randomString)
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
	c.SendStatus(fiber.StatusOK)
	return nil

}

// SearchFiles searches for files based on a query and returns the results.
func SearchFiles(c *fiber.Ctx, db *gorm.DB) error {
	searchQuery := c.Query("query", "")
	page := c.Query("page", "1")
	pageInt, err := strconv.Atoi(page)
	if err != nil || pageInt < 1 {
		pageInt = 1
	}
	pageSize := 10

	var files []model.File
	var totalMatchingCount int64

	if searchQuery != "" {
		db.Model(&model.File{}).
			Where("name LIKE ?", "%"+searchQuery+"%").
			Count(&totalMatchingCount)

		db.Where("name LIKE ?", "%"+searchQuery+"%").
			Order("created_at DESC").
			Offset((pageInt - 1) * pageSize).
			Limit(pageSize).
			Find(&files)
	} else {
		db.Model(&model.File{}).
			Count(&totalMatchingCount)

		db.Order("created_at DESC").
			Offset((pageInt - 1) * pageSize).
			Limit(pageSize).
			Find(&files)
	}

	totalPages := int(math.Ceil(float64(totalMatchingCount) / float64(pageSize)))

	return c.Render("partials/file-manager", fiber.Map{
		"Files":       files,
		"TotalPages":  totalPages,
		"CurrentPage": pageInt,
		"SearchQuery": searchQuery,
	})
}
