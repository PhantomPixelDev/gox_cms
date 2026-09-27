package handlers

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"goxcms/model"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// accountProfileError writes a validation failure to the response. It returns
// the value to hand back from the route handler.
//
// Note: c.SendString returns nil on success, so a helper that returns
// SendString's error cannot be used to signal "validation failed" — that
// mistake silently turns every 400 into a 200.
func accountProfileError(c *fiber.Ctx, msg string) error {
	ShowToastError(c, msg)
	return c.Status(fiber.StatusBadRequest).SendString(msg)
}

// validateAccountProfile checks the editable profile fields and returns an
// error message, or "" when they are all acceptable.
func validateAccountProfile(c *fiber.Ctx, db *gorm.DB, user model.User) string {
	firstName := strings.TrimSpace(c.FormValue("first_name"))
	lastName := strings.TrimSpace(c.FormValue("last_name"))
	username := strings.TrimSpace(c.FormValue("username"))
	email := strings.TrimSpace(c.FormValue("email"))

	if len(firstName) < 2 || len(firstName) > 30 {
		return "First name must be 2-30 characters"
	}
	if len(lastName) < 2 || len(lastName) > 30 {
		return "Last name must be 2-30 characters"
	}
	if len(username) < 2 || len(username) > 30 {
		return "Username must be 2-30 characters"
	}
	if !alphanumeric(username) {
		return "Username can only contain letters and numbers"
	}
	if username != user.Username {
		var taken int64
		db.Model(&model.User{}).Where("username = ? AND id <> ?", username, user.ID).Count(&taken)
		if taken > 0 {
			return "That username is already taken"
		}
	}
	if email != "" && !validEmail(email) {
		return "Enter a valid email address"
	}
	return ""
}

func validEmail(s string) bool {
	at := strings.Index(s, "@")
	if at <= 0 || at == len(s)-1 {
		return false
	}
	host := s[at+1:]
	return strings.Contains(host, ".") && !strings.HasSuffix(host, ".") &&
		!strings.ContainsAny(s, " \t")
}

func alphanumeric(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// UpdateAccountProfile saves the profile form from /account.
func UpdateAccountProfile(c *fiber.Ctx, db *gorm.DB) error {
	user, ok := CurrentUser(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).SendString("Not logged in")
	}
	if msg := validateAccountProfile(c, db, user); msg != "" {
		return accountProfileError(c, msg)
	}

	firstName := strings.TrimSpace(c.FormValue("first_name"))
	lastName := strings.TrimSpace(c.FormValue("last_name"))
	username := strings.TrimSpace(c.FormValue("username"))
	email := strings.TrimSpace(c.FormValue("email"))

	// Email is nullable, so an emptied field clears it rather than storing "".
	updates := map[string]interface{}{
		"first_name": firstName,
		"last_name":  lastName,
		"username":   username,
		"email":      nil,
	}
	if email != "" {
		updates["email"] = email
	}

	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		if isDupKeyError(err) {
			return accountProfileError(c, "That username is already taken")
		}
		return accountProfileError(c, "Could not save your profile")
	}

	ShowToast(c, "Profile updated")
	return accountProfilePanel(c, db, user.ID)
}

// accountProfilePanel re-reads the user and renders just the profile card so
// HTMX can swap it in place.
func accountProfilePanel(c *fiber.Ctx, db *gorm.DB, userID uint) error {
	var fresh model.User
	if err := db.First(&fresh, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).SendString("User not found")
	}
	return c.Render("partials/account-profile", fiber.Map{
		"User":     fresh,
		"Settings": c.Locals("Settings"),
	})
}

// AccountPage renders the whole /account view, so HTMX targets and a plain
// GET always show identical markup.
func AccountPage(c *fiber.Ctx, db *gorm.DB) error {
	user, ok := CurrentUser(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).SendString("Not logged in")
	}

	var posts, comments int64
	db.Model(&model.Post{}).Where("user_id = ?", user.ID).Count(&posts)
	db.Model(&model.Comment{}).Where("user_id = ?", user.ID).Count(&comments)

	sessionHours := int(jwtLifetime().Hours())

	return c.Render("account", fiber.Map{
		"Title":        "Account",
		"Settings":     c.Locals("Settings"),
		"User":         user,
		"PostCount":    posts,
		"CommentCount": comments,
		"SessionHours": sessionHours,
	}, "main")
}

// UpdateAccountAvatar stores a new profile image in the uploads directory and
// points the user record at it. The previous avatar is removed so uploads
// don't accumulate.
func UpdateAccountAvatar(c *fiber.Ctx, db *gorm.DB) error {
	user, ok := CurrentUser(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).SendString("Not logged in")
	}

	file, err := c.FormFile("avatar")
	if err != nil {
		return accountProfileError(c, "Choose an image to upload")
	}
	if file.Size > maxUploadSize() {
		return accountProfileError(c, "Image is too large")
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !AllowedFileTypes[ext] {
		return accountProfileError(c, "Avatars must be a JPG, PNG or GIF")
	}
	contentType, err := sniffContentType(file)
	if err != nil || !AllowedContentTypes[contentType] {
		return accountProfileError(c, "That file is not a valid image")
	}

	// Cap avatars at a few hundred KB; they are downloaded on every page that
	// shows an author name.
	const maxAvatarBytes = 2 << 20
	if file.Size > maxAvatarBytes {
		return accountProfileError(c, "Avatars must be under 2 MB")
	}

	if err := os.MkdirAll(UploadDir, 0o755); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Cannot create upload directory")
	}
	filename := fmt.Sprintf("avatar_%d_%s%s", user.ID, randomFilenameString(RandomFilenameSize), ext)
	dest := filepath.Join(UploadDir, filename)
	if err := c.SaveFile(file, dest); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Cannot save avatar")
	}

	path := "/static/uploads/" + filename
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Update("avatar_url", path).Error; err != nil {
		os.Remove(dest)
		return c.Status(fiber.StatusInternalServerError).SendString("Cannot save avatar")
	}

	// Best-effort cleanup of the replaced file; never fail the request on it.
	if user.AvatarURL != nil && strings.HasPrefix(*user.AvatarURL, "/static/uploads/") {
		old := filepath.Join(UploadDir, filepath.Base(*user.AvatarURL))
		if _, err := os.Stat(old); err == nil {
			if err := os.Remove(old); err != nil {
				log.Printf("account avatar: removing old file: %v", err)
			}
		}
	}

	ShowToast(c, "Avatar updated")
	return accountProfilePanel(c, db, user.ID)
}
