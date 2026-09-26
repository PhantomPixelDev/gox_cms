package handlers

import (
	"goxcms/utils"

	"github.com/gofiber/fiber/v2"
)

// contentSecurityPolicy locks the page to same-origin plus the CDNs the
// templates use. Inline scripts/styles stay allowed ('unsafe-inline') because
// main.html and the admin views rely on them (CSRF header wiring, toasts,
// theme switch, Quill); object/embed, foreign frames and base-uri hijacking
// are still blocked. Saved HTML is sanitized on top of this (SanitizeRichHTML).
const contentSecurityPolicy = "default-src 'self';" +
	" script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com https://ajax.googleapis.com https://js.hcaptcha.com;" +
	" style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net;" +
	" img-src 'self' data: https:;" +
	" font-src 'self' data: https://cdn.jsdelivr.net;" +
	" frame-src https://hcaptcha.com https://*.hcaptcha.com;" +
	" object-src 'none';" +
	" base-uri 'self';" +
	" frame-ancestors 'self';" +
	" form-action 'self'"

// SecurityHeaders sets baseline hardening headers on every response. HSTS is
// only sent when the site is served over HTTPS.
func SecurityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "SAMEORIGIN")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Content-Security-Policy", contentSecurityPolicy)
		c.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if utils.SecureCookies() {
			c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		return c.Next()
	}
}
