package handlers

import (
	"strings"

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
	" form-action 'self';" +
	// No inline event handlers exist in any template or in the first-party JS
	// (admin behaviour uses data-* hooks), so forbidding them turns a future
	// regression into a hard CSP failure instead of a silent hole.
	" script-src-attr 'none'"

// adminPathPrefix marks the routes whose responses must never be stored by a
// browser or a shared proxy: they contain usernames, e-mail addresses, site
// settings and the signed-in user's own profile.
const adminPathPrefix = "/admin"

// SecurityHeaders sets baseline hardening headers on every response. HSTS is
// only sent when the site is served over HTTPS.
//
// Authenticated responses also get Cache-Control: no-store, which the standard
// security headers do not cover and which nothing else in the app set.
func SecurityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "SAMEORIGIN")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Content-Security-Policy", contentSecurityPolicy)
		c.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// Isolates the admin panel from cross-origin window handle sharing.
		// Cheap, and no first-party asset needs a cross-origin document.
		c.Set("Cross-Origin-Opener-Policy", "same-origin")
		if utils.SecureCookies() {
			c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		noStore(c)
		return c.Next()
	}
}

// noStore marks per-user responses as uncacheable.
//
// Every admin screen was otherwise heuristically cacheable, so a shared proxy
// in front of the app or the browser bfcache could retain and re-serve HTML
// containing usernames, e-mail addresses and settings. Vary: Cookie is set too
// so a cache cannot serve a personalised body to the next visitor.
func noStore(c *fiber.Ctx) {
	path := c.Path()
	isAdmin := path == adminPathPrefix ||
		strings.HasPrefix(path, adminPathPrefix+"/") ||
		path == "/account" ||
		path == "/login" ||
		path == "/register"
	if !isAdmin {
		return
	}
	if IsTrue(c, "isLoggedin") {
		c.Set("Cache-Control", "no-store, private")
		c.Set("Vary", "Cookie")
		return
	}
	c.Set("Cache-Control", "no-store")
}
