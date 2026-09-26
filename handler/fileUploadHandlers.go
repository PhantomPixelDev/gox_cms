package handlers

import (
	"crypto/rand"
	"goxcms/model"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

const (
	UploadDir          = "./static/uploads"
	RandomFilenameSize = 10
)

var (
	AllowedFileTypes = map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
	}

	// AllowedContentTypes are checked against the sniffed file content, not
	// the client-supplied Content-Type header.
	AllowedContentTypes = map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/gif":  true,
	}
)

// maxUploadSize returns the upload limit in bytes from upload.max_size_mb.
func maxUploadSize() int64 {
	return viper.GetInt64("upload.max_size_mb") * 1024 * 1024
}

func UploadFile(c *fiber.Ctx, db *gorm.DB) error {
	file, err := c.FormFile("file")
	if err != nil {
		log.Printf("upload: reading form file: %v", err)
		return c.Status(fiber.StatusBadRequest).SendString("Cannot read file")
	}

	if file.Size > maxUploadSize() {
		return c.Status(fiber.StatusBadRequest).SendString("File size exceeds the limit")
	}

	fileType := strings.ToLower(filepath.Ext(file.Filename))
	if !AllowedFileTypes[fileType] {
		return c.Status(fiber.StatusBadRequest).SendString("File type not allowed")
	}

	contentType, err := sniffContentType(file)
	if err != nil {
		log.Printf("upload: sniffing content type: %v", err)
		return c.Status(fiber.StatusBadRequest).SendString("Cannot read file")
	}
	if !AllowedContentTypes[contentType] {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid content type")
	}

	// Add a random suffix so uploads never overwrite each other.
	baseName := strings.TrimSuffix(filepath.Base(file.Filename), filepath.Ext(file.Filename))
	filename := baseName + "_" + randomFilenameString(RandomFilenameSize) + fileType

	if err := os.MkdirAll(UploadDir, 0o755); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Cannot create upload directory")
	}

	if err := c.SaveFile(file, filepath.Join(UploadDir, filename)); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Cannot save file to disk")
	}

	fileModel := model.File{
		Name:      filename,
		Extension: fileType,
		Path:      "/static/uploads/" + filename,
	}

	if err := db.Create(&fileModel).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Cannot save file to database")
	}

	return ShowToast(c, "File uploaded successfully")
}

// sniffContentType detects the MIME type from the first 512 bytes of the file.
func sniffContentType(fh *multipart.FileHeader) (string, error) {
	f, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer f.Close()

	head := make([]byte, 512)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", err
	}
	return http.DetectContentType(head[:n]), nil
}

// randomFilenameString returns a random alphanumeric string.
func randomFilenameString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}

func DeleteFile(c *fiber.Ctx, db *gorm.DB) error {
	filename := c.FormValue("name")
	safeFilename := filepath.Base(filename)
	filePath := filepath.Join(UploadDir, safeFilename)

	// Delete the database row first: it is the source of truth, and a
	// missing row is a clean 404 instead of a half-deleted state.
	result := db.Delete(&model.File{}, "name = ?", safeFilename)
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete file from database"})
	}
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "File not found"})
	}

	// Best effort on disk: the row is already gone, so a missing file is
	// only worth a log line, not a failure.
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		log.Printf("delete file: removing %s from disk: %v", filePath, err)
	}

	ShowToast(c, "File deleted successfully")
	c.SendStatus(fiber.StatusOK)
	return nil
}

// SearchFiles searches for files based on a query and returns the results.
func SearchFiles(c *fiber.Ctx, db *gorm.DB) error {
	searchQuery := c.Query("query", "")
	// validate page number
	pageInt := queryPage(c)
	pageSize := 20

	var files []model.File
	var totalMatchingCount int64

	if searchQuery != "" {
		db.Model(&model.File{}).
			Where("name LIKE ? ESCAPE '\\'", likePattern(searchQuery)).
			Count(&totalMatchingCount)

		db.Where("name LIKE ? ESCAPE '\\'", likePattern(searchQuery)).
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

	totalPages := pageCount(totalMatchingCount, pageSize)

	return c.Render("partials/file-manager", fiber.Map{
		"Files":       files,
		"TotalPages":  totalPages,
		"CurrentPage": pageInt,
		"SearchQuery": searchQuery,
	})
}
