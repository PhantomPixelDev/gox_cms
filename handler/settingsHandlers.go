package handlers

import (
	"goxcms/model"

	"github.com/gofiber/fiber/v2"
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

	// Update settings with form data
	updatedSettings := updateSettingsFromForm(&settings, c)

	// Save updated settings to the database
	if err := db.Save(&updatedSettings).Error; err != nil {
		ShowToastError(c, "Failed to update settings")
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update settings")
	}

	// Update settings in locals
	c.Locals("Settings", MapSettingsToMap(updatedSettings))

	// Show success message
	ShowToastError(c, "Settings updated successfully, please clear your cache to see the changes")
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
	return *settings
}

func MapSettingsToMap(settings model.BasicWebsiteInfo) map[string]string {
	return map[string]string{
		"Name":           settings.Name,
		"Tagline":        settings.Tagline,
		"Email":          settings.Email,
		"Phone":          settings.Phone,
		"Address":        settings.Address,
		"About":          settings.About,
		"LogoURL":        settings.LogoURL,
		"FaviconURL":     settings.FaviconURL,
		"FacebookURL":    settings.FacebookURL,
		"TwitterURL":     settings.TwitterURL,
		"LinkedInURL":    settings.LinkedInURL,
		"SEOKeywords":    settings.SEOKeywords,
		"SEODescription": settings.SEODescription,
		"AnalyticsID":    settings.AnalyticsID,
		"FooterText":     settings.FooterText,
		"Theme":          settings.Theme,
		"ContactEmail":   settings.ContactEmail,
		"PrivacyPolicy":  settings.PrivacyPolicy,
		"TermsOfService": settings.TermsOfService,
		"Language":       settings.Language,
		"Locale":         settings.Locale,
		"TimeZone":       settings.TimeZone,
		"SelectedTheme":  settings.SelectedTheme,
		"ContainerClass": settings.ContainerClass,
	}
}
