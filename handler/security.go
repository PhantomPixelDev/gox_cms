package handlers

import (
	"goxcms/utils"

	"github.com/gofiber/fiber/v2"
)

// contentSecurityPolicy locks the page to same-origin plus the CDNs the
// templates use. All first-party JavaScript lives in /static/js files and
// admin behavior uses data-* hooks evaluated by static listeners, so no
// 'unsafe-inline' or 'unsafe-eval' is needed for scripts. Inline styles stay
// allowed because the Quill editor applies them at runtime; object/embed,
// foreign frames and base-uri hijacking are still blocked. Saved HTML is
// sanitized on top of this (SanitizeRichHTML).
const contentSecurityPolicy = "default-src 'self';" +
	" script-src 'self' https://cdn.jsdelivr.net https://unpkg.com https://ajax.googleapis.com https://cdnjs.cloudflare.com https://js.hcaptcha.com;" +
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
