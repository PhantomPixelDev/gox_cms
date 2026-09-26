package handlers

import (
	"regexp"
	"strings"

	"goxcms/model"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

var slugCleanup = regexp.MustCompile(`[^a-z0-9]+`)

// slugify turns a display name into a URL-safe slug.
func slugify(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = slugCleanup.ReplaceAllString(slug, "-")
	return strings.Trim(slug, "-")
}

// AddTaxonomyInline creates a tag or category from the post editor's
// selectize "create" flow and returns its ID as JSON. Duplicates return the
// existing row instead of an error, so double submits stay harmless.
func AddTaxonomyInline(c *fiber.Ctx, db *gorm.DB) error {
	kind := c.FormValue("kind")
	name := strings.TrimSpace(c.FormValue("name"))
	if name == "" || len(name) > 60 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Name is required (max 60 characters)"})
	}
	slug := slugify(name)
	if slug == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Name must contain letters or numbers"})
	}

	switch kind {
	case "tag":
		var tag model.Tag
		if err := db.Where("slug = ?", slug).First(&tag).Error; err == nil {
			return c.Status(fiber.StatusOK).JSON(fiber.Map{"id": tag.ID})
		}
		tag = model.Tag{Name: name, Slug: slug}
		if err := db.Create(&tag).Error; err != nil {
			if isDupKeyError(err) {
				db.Where("slug = ?", slug).First(&tag)
				return c.Status(fiber.StatusOK).JSON(fiber.Map{"id": tag.ID})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not create tag"})
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": tag.ID})
	case "category":
		var category model.Category
		if err := db.Where("slug = ?", slug).First(&category).Error; err == nil {
			return c.Status(fiber.StatusOK).JSON(fiber.Map{"id": category.ID})
		}
		category = model.Category{Name: name, Slug: slug}
		if err := db.Create(&category).Error; err != nil {
			if isDupKeyError(err) {
				db.Where("slug = ?", slug).First(&category)
				return c.Status(fiber.StatusOK).JSON(fiber.Map{"id": category.ID})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not create category"})
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": category.ID})
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Unknown kind"})
	}
}
