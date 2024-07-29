package model

import "time"

type BasicWebsiteInfo struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	Name           string    `json:"name"`
	Tagline        string    `json:"tagline"`
	Email          string    `json:"email"`
	Phone          string    `json:"phone"`
	Address        string    `json:"address"`
	About          string    `json:"about"`
	LogoURL        string    `json:"logo_url"`
	FaviconURL     string    `json:"favicon_url"`
	FacebookURL    string    `json:"facebook_url"`
	TwitterURL     string    `json:"twitter_url"`
	LinkedInURL    string    `json:"linkedin_url"`
	SEOKeywords    string    `json:"seo_keywords"`
	SEODescription string    `json:"seo_description"`
	AnalyticsID    string    `json:"analytics_id"`
	FooterText     string    `json:"footer_text"`
	Maintenance    bool      `json:"maintenance"`
	Theme          string    `json:"theme"`
	ContactEmail   string    `json:"contact_email"`
	PrivacyPolicy  string    `json:"privacy_policy"`
	TermsOfService string    `json:"terms_of_service"`
	Language       string    `json:"language"`
	Locale         string    `json:"locale"`
	TimeZone       string    `json:"time_zone"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	SelectedTheme  string    `json:"selected_theme" default:"cerulean"`
	ContainerClass string    `json:"container_class" default:"container"`
}
