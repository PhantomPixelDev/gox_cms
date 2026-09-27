package handlers

import (
	"goxcms/model"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

func UpdateSettings(c *fiber.Ctx, db *gorm.DB) error {
	// Initialize an empty settings structure
	var settings model.BasicWebsiteInfo

	// Retrieve current website settings from the database
	if err := db.First(&settings).Error; err != nil {
		ShowToastError(c, "Error retrieving current settings")
		return c.Status(fiber.StatusInternalServerError).SendString("Error retrieving current settings")
	}

	// A theme is only accepted if every required template is present and
	// parses. Rejecting here, with the reason, is what keeps a half-written
	// theme from taking the public site down: the previous behaviour silently
	// normalised an unknown name to "default", which hid the mistake.
	requested := c.FormValue("site_template")
	if msg := ValidateTheme(requested); msg != "" {
		ShowToastError(c, msg)
		return c.Status(fiber.StatusBadRequest).SendString(msg)
	}

	// Update settings with form data
	updatedSettings := updateSettingsFromForm(&settings, c)

	// Save updated settings to the database
	if err := db.Save(&updatedSettings).Error; err != nil {
		ShowToastError(c, "Failed to update settings")
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update settings")
	}

	// Refresh the cached settings and this request's copy.
	ReloadSiteSettings(db)
	c.Locals("Settings", SiteSettings(db))
	// The theme may have just been created on disk; re-scan and re-register
	// templates so it takes effect without a restart.
	ReloadThemes()
	ReloadTemplates()

	// Show success message
	ShowToast(c, "Settings updated successfully")
	return c.Status(fiber.StatusOK).SendString("Settings updated successfully")
}

func updateSettingsFromForm(settings *model.BasicWebsiteInfo, c *fiber.Ctx) model.BasicWebsiteInfo {
	settings.Name = c.FormValue("name")
	settings.Tagline = c.FormValue("tagline")
	settings.Email = c.FormValue("email")
	settings.Phone = c.FormValue("phone")
	settings.Address = c.FormValue("address")
	settings.About = c.FormValue("about")
	settings.LogoURL = c.FormValue("logo_url")
	// For theme, the form uses "theme", so it's correctly mapped
	settings.Theme = c.FormValue("theme")
	settings.ContainerClass = c.FormValue("container_class")
	// Already validated by UpdateSettings before this runs.
	settings.SiteTemplate = c.FormValue("site_template")
	// Update social media URLs based on your form's input names
	settings.FacebookURL = c.FormValue("facebookUrl") // Changed from "facebookURL" to match form name attribute
	settings.TwitterURL = c.FormValue("twitterUrl")   // Changed from "twitter_url" to match form name attribute
	settings.LinkedInURL = c.FormValue("linkedinUrl") // Changed from "linkedin_url" to match form name attribute
	// Update SEO settings based on form input names
	settings.SEOKeywords = c.FormValue("seoKeywords")         // Changed to match form name attribute
	settings.SEODescription = c.FormValue("seoDescription")   // Changed to match form name attribute
	settings.LogoURL = c.FormValue("logo_url")                // Add or update based on your actual form and needs
	settings.FaviconURL = c.FormValue("favicon_url")          // Add or update based on your actual form and needs
	settings.AnalyticsID = c.FormValue("analytics_id")        // Add or update based on your actual form and needs
	settings.FooterText = c.FormValue("footer_text")          // Add or update based on your actual form and needs
	settings.ContactEmail = c.FormValue("contact_email")      // Add or update based on your actual form and needs
	settings.PrivacyPolicy = c.FormValue("privacy_policy")    // Add or update based on your actual form and needs
	settings.TermsOfService = c.FormValue("terms_of_service") // Add or update based on your actual form and needs
	settings.Language = c.FormValue("language")               // Add or update based on your actual form and needs
	settings.Locale = c.FormValue("locale")                   // Add or update based on your actual form and needs
	settings.TimeZone = c.FormValue("timezone")               // Add or update based on your actual form and needs
	settings.RegistrationEnabled = c.FormValue("registration_enabled") == "on"
	return *settings
}

func MapSettingsToMap(settings model.BasicWebsiteInfo) map[string]string {
	return map[string]string{
		"Name":                settings.Name,
		"Tagline":             settings.Tagline,
		"Email":               settings.Email,
		"Phone":               settings.Phone,
		"Address":             settings.Address,
		"About":               settings.About,
		"LogoURL":             settings.LogoURL,
		"FaviconURL":          settings.FaviconURL,
		"FacebookURL":         settings.FacebookURL,
		"TwitterURL":          settings.TwitterURL,
		"LinkedInURL":         settings.LinkedInURL,
		"SEOKeywords":         settings.SEOKeywords,
		"SEODescription":      settings.SEODescription,
		"AnalyticsID":         settings.AnalyticsID,
		"FooterText":          settings.FooterText,
		"Theme":               settings.Theme,
		"ContactEmail":        settings.ContactEmail,
		"PrivacyPolicy":       settings.PrivacyPolicy,
		"TermsOfService":      settings.TermsOfService,
		"Language":            settings.Language,
		"Locale":              settings.Locale,
		"TimeZone":            settings.TimeZone,
		"ContainerClass":      settings.ContainerClass,
		"NavbarClass":         navbarClassForTheme(settings.Theme),
		"InitialTheme":        initialTheme(settings.Theme),
		"SiteTemplate":        SiteTemplateSet(settings.SiteTemplate),
		"RegistrationEnabled": strconv.FormatBool(settings.RegistrationEnabled),
		"CaptchaEnabled":      strconv.FormatBool(viper.GetBool("captcha.enabled")),
		"CaptchaSiteKey":      viper.GetString("captcha.public_key"),
	}
}

// darkBootswatchThemes are dark by design, so the page starts in dark mode
// with them.
var darkBootswatchThemes = map[string]bool{
	"cyborg": true, "darkly": true, "slate": true,
	"solar": true, "superhero": true, "vapor": true,
}

// navbarClassForTheme returns extra navbar classes for the Bootswatch theme.
//
// It deliberately returns "" for light themes. It used to return
// "navbar-light bg-light" (or "navbar-dark bg-dark"), which pinned the navbar
// to a fixed palette: with dark mode on, the page went dark while the navbar
// stayed white, and the light/dark link colors fought the active theme. A bare
// .navbar inherits Bootstrap's --bs-navbar-* variables, which follow
// data-bs-theme automatically.
func navbarClassForTheme(theme string) string {
	return ""
}

// initialTheme is the data-bs-theme rendered into <html>. Dark Bootswatch
// themes start dark; everything else follows the visitor's saved choice, which
// static/js/site.js applies after load.
func initialTheme(theme string) string {
	if darkBootswatchThemes[theme] {
		return "dark"
	}
	return "light"
}

// settingsTTL bounds how stale cached settings can get in another process
// (prefork or multiple instances) after an update.
const settingsTTL = 30 * time.Second

var siteSettings struct {
	sync.RWMutex
	values   map[string]string
	loadedAt time.Time
}

// SiteSettings returns the website settings used by every page, loading them
// from the database at most once per settingsTTL. The returned map is shared
// and must not be modified.
func SiteSettings(db *gorm.DB) map[string]string {
	siteSettings.RLock()
	values, fresh := siteSettings.values, time.Since(siteSettings.loadedAt) < settingsTTL
	siteSettings.RUnlock()

	if values != nil && fresh {
		return values
	}
	return ReloadSiteSettings(db)
}

// ReloadSiteSettings reads the settings from the database into the cache.
// The oldest row wins: it is the install-time singleton.
func ReloadSiteSettings(db *gorm.DB) map[string]string {
	var settings model.BasicWebsiteInfo
	db.Order("id ASC").First(&settings)
	values := MapSettingsToMap(settings)

	siteSettings.Lock()
	siteSettings.values = values
	siteSettings.loadedAt = time.Now()
	siteSettings.Unlock()

	return values
}
