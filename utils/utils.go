package utils

import (
	"bytes"
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"goxcms/model"
	htmlstd "html"
	"html/template"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/gofiber/storage/redis/v3"
	"github.com/gofiber/template/html/v2"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func InitConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Fatal error config file: %s", err)
	}

	viper.AutomaticEnv()

	// Set default values
	viper.SetDefault("server.host", "localhost")
	viper.SetDefault("server.port", "3000")
	viper.SetDefault("server.prefork", false)
	viper.SetDefault("build.mode", "production")
	viper.SetDefault("database.driver", "sqlite")
	viper.SetDefault("database.sqlite.dsn", "./data/database.sqlite")
	viper.SetDefault("upload.max_size_mb", 50)
	viper.SetDefault("ratelimiter.enabled", true)
	viper.SetDefault("ratelimiter.max_requests", 100)
	viper.SetDefault("redis.enabled", false)
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.username", "")
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.database", 0)
	viper.SetDefault("redis.pool_size", 10)
	viper.SetDefault("server.body_limit", 10)
	viper.SetDefault("captcha.public_key", "")
	viper.SetDefault("captcha.secret_key", "")
	viper.SetDefault("captcha.enabled", false)
	// Login sessions last app.session_hours (JWT expiry); logout revokes
	// them immediately via the user's session version.
	viper.SetDefault("app.session_hours", 12)
	// Brute-force guard: auth.login_max_attempts failures per
	// auth.login_window_minutes from one IP -> HTTP 429 on /login.
	viper.SetDefault("auth.login_max_attempts", 10)
	viper.SetDefault("auth.login_window_minutes", 5)

	if viper.GetBool("redis.enabled") {
		log.Println("Redis enabled")
	}

	validateSecret()
}

const exampleSecret = "change_this_secret"

// validateSecret makes sure app.secret, which signs the login JWTs, is set to
// something that is not publicly known. Production refuses to start without
// one; development falls back to a random per-process secret.
func validateSecret() {
	secret := viper.GetString("app.secret")
	weak := secret == "" || secret == exampleSecret

	if viper.GetString("build.mode") == "production" {
		if weak {
			log.Fatal("app.secret must be set to a long random value in production (e.g. `openssl rand -hex 32`)")
		}
		if len(secret) < 32 {
			log.Println("WARNING: app.secret is shorter than 32 characters; use a longer random value")
		}
		return
	}

	if weak {
		viper.Set("app.secret", randomString(32))
		log.Println("WARNING: app.secret is not set; using a random secret for this process. Logins will not survive a restart.")
	}
}

// randomString returns a URL-safe random string of the given length.
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			log.Fatalf("Failed to generate random value: %v", err)
		}
		out[i] = charset[n.Int64()]
	}
	return string(out)
}

// ViewsDir is where the template engine looks for templates, and the root that
// site theme discovery scans (see handlers.SetViewsRoot).
const ViewsDir = "./views"

// SetupEngine builds the HTML engine.
//
// The engine is pointed at a filesystem wrapper that hides templates which do
// not parse. Without it, a single malformed template anywhere under views/ — for
// instance a theme mid-edit — fails the whole load and takes every page on the
// site down, including the admin page that is supposed to report the problem.
// See parseGuard.go.
func SetupEngine() *html.Engine {
	funcs := TemplateFuncMap()
	engine := html.NewFileSystem(ParseGuardFS(http.FS(os.DirFS(viewsRoot())), funcs), ".html")
	engine.AddFuncMap(funcs)
	return engine
}

// viewsRoot resolves the template directory for the engine. A directory that
// does not exist falls back to the default so the error surfaces as a missing
// template rather than as a confusing "views: no such file or directory" from
// the filesystem itself.
func viewsRoot() string {
	dir := ViewsDir
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return dir
	}
	return "views"
}

// TemplateFuncMap is every helper available inside a template, exported so the
// theme validator can parse a candidate theme with exactly the same function
// set the engine will use. A theme referencing a helper that does not exist
// would otherwise fail only at render time.
func TemplateFuncMap() template.FuncMap {
	return template.FuncMap{
		"timestamp": func() string {
			return fmt.Sprintf("?v=%d", time.Now().Unix())
		},
		"year": func() int {
			return time.Now().Year()
		},
		"truncate": func(s string, length int) string {
			runes := []rune(s)
			if len(runes) > length {
				return string(runes[:length]) + "..."
			}
			return s
		},
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"sequence": func(start, end int) []int {
			seq := make([]int, end-start+1)
			for i := range seq {
				seq[i] = start + i
			}
			return seq
		},
		"default": func(value, defaultValue string) string {
			if value == "" {
				return defaultValue
			}
			return value
		},
		"count_post": func(posts []model.Post) int {
			var count int
			for _, post := range posts {
				count += len(post.Tags)
			}
			return count
		},
		// escape converts HTML to plain text; the template escapes the result.
		"escape": htmlToPlainText,
		"unescape": func(s string) template.HTML {
			return template.HTML(s)
		},
		// uintval dereferences a *uint column (MenuID, ParentID) so templates
		// can compare it against a plain ID; nil becomes 0.
		"uintval": func(p *uint) uint {
			if p == nil {
				return 0
			}
			return *p
		},

		"max": max,
		"min": min,
		"ge":  ge,
		"gt":  gt,
		"le":  le,
		"lt":  lt,
		// window returns the page numbers to show in a pager: a first page, a
		// few either side of the current one, and a last page, with 0 marking
		// where an ellipsis belongs.
		//
		// It exists because the old pager rendered one <li> per page: 200
		// posts produced 200 page links, which is a wall of buttons that
		// dwarfs the table it paginates.
		"window": pageWindow,
		// ellipsis is the marker value window uses for a gap.
		"ellipsis": func() int { return 0 },
		// dict builds a map for passing named params to sub-templates:
		// {{template "partials/pagination" dict "Base" "/search-posts" ...}}
		"dict": func(values ...interface{}) map[string]interface{} {
			m := make(map[string]interface{}, len(values)/2)
			for i := 0; i+1 < len(values); i += 2 {
				if k, ok := values[i].(string); ok {
					m[k] = values[i+1]
				}
			}
			return m
		},
	}
}

// pageWindow builds a bounded page list for pagination controls.
// current <= 0 or total <= 0 is treated as "no pager".
func pageWindow(current, total, span int) []int {
	if total < 1 {
		return nil
	}
	if current < 1 {
		current = 1
	}
	if current > total {
		current = total
	}
	if span < 1 {
		span = 2
	}

	var out []int
	add := func(n int) {
		if n < 1 || n > total {
			return
		}
		if len(out) > 0 && out[len(out)-1] == n {
			return
		}
		out = append(out, n)
	}
	gap := func() {
		if len(out) > 0 && out[len(out)-1] != 0 {
			out = append(out, 0)
		}
	}

	first := current - span
	last := current + span

	add(1)
	if first > 2 {
		gap()
	}
	for n := max(first, 2); n <= min(last, total-1); n++ {
		add(n)
	}
	if last < total-1 {
		gap()
	}
	add(total)
	return out
}

func SetupStore(app *fiber.App) *session.Store {
	var store *session.Store

	if viper.GetBool("redis.enabled") {
		redisStorage := redis.New(redis.Config{
			Host:     viper.GetString("redis.host"),
			Port:     viper.GetInt("redis.port"),
			Username: viper.GetString("redis.username"),
			Password: viper.GetString("redis.password"),
			Database: viper.GetInt("redis.database"),
			PoolSize: viper.GetInt("redis.pool_size"),
		})

		store = session.New(session.Config{
			Expiration:     24 * time.Hour,
			CookieHTTPOnly: true,
			CookieSecure:   SecureCookies(),
			Storage:        redisStorage,
		})
	} else {
		store = session.New(session.Config{
			Expiration:     24 * time.Hour,
			CookieHTTPOnly: true,
			CookieSecure:   SecureCookies(),
		})
	}

	if store == nil {
		log.Fatal("Store initialization failed")
	}

	app.Use(func(c *fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			log.Printf("session load: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Session error")
		}
		c.Locals("session", sess)
		return c.Next()
	})

	return store
}

// rateLimitWindow is the fixed-window length for the global limiter.
const rateLimitWindow = 30 * time.Second

func SetupRateLimiter(app *fiber.App, store *session.Store) {
	if viper.GetBool("ratelimiter.enabled") {
		log.Println("Rate limiter enabled")
		app.Use(limiter.New(limiter.Config{
			Max:        viper.GetInt("ratelimiter.max_requests"),
			Expiration: rateLimitWindow,
			// Keyed on the socket peer, not c.IP(): the latter is taken from
			// X-Forwarded-For for anything inside server.trusted_proxies, so
			// rotating the header would reset the budget. Static assets are
			// skipped so a page pulling its CSS/JS cannot exhaust the window.
			KeyGenerator: func(c *fiber.Ctx) string {
				addr := c.Context().RemoteAddr().String()
				if host, _, err := net.SplitHostPort(addr); err == nil {
					return host
				}
				return addr
			},
			// Never limit the health check: an orchestrator probing at a fixed
			// interval must not be able to lock itself out.
			Next: func(c *fiber.Ctx) bool {
				return c.Path() == "/healthz" || strings.HasPrefix(c.Path(), "/static/")
			},
			LimitReached: func(c *fiber.Ctx) error {
				return c.Status(fiber.StatusTooManyRequests).SendString("Rate limit exceeded")
			},
			Storage: store.Storage,
		}))
	} else {
		log.Println("Rate limiter not enabled")
	}
}

// sitemapCache holds the rendered sitemap for sitemapTTL: crawlers hit this
// endpoint in bursts, and rebuilding means four full-table scans per hit.
var sitemapCache struct {
	sync.Mutex
	renderedAt time.Time
	payload    []byte
}

const sitemapTTL = 10 * time.Minute

// BuildSitemap renders sitemap.xml for all published content, cached briefly.
// Only published pages with non-empty slugs are listed.
func BuildSitemap(db *gorm.DB) []byte {
	sitemapCache.Lock()
	defer sitemapCache.Unlock()

	if sitemapCache.payload != nil && time.Since(sitemapCache.renderedAt) < sitemapTTL {
		return sitemapCache.payload
	}

	baseURL := strings.TrimSuffix(viper.GetString("app.url"), "/")
	paths := []string{"/", "/blog", "/login", "/register"}

	var postSlugs, categorySlugs, tagSlugs, pageSlugs []string
	db.Model(&model.Post{}).Where("published = ? AND slug <> ?", true, "").Limit(5000).Pluck("slug", &postSlugs)
	db.Model(&model.Category{}).Where("slug <> ?", "").Limit(5000).Pluck("slug", &categorySlugs)
	db.Model(&model.Tag{}).Where("slug <> ?", "").Limit(5000).Pluck("slug", &tagSlugs)
	db.Model(&model.CustomPage{}).Where("published = ? AND slug <> ?", true, "").Limit(5000).Pluck("slug", &pageSlugs)

	for _, slug := range postSlugs {
		paths = append(paths, "/blog/post/"+slug)
	}
	for _, slug := range categorySlugs {
		paths = append(paths, "/blog/category/"+slug)
	}
	for _, slug := range tagSlugs {
		paths = append(paths, "/blog/tag/"+slug)
	}
	for _, slug := range pageSlugs {
		paths = append(paths, "/"+slug)
	}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	buf.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, path := range paths {
		buf.WriteString("  <url>\n    <loc>")
		xml.EscapeText(&buf, []byte(baseURL+path))
		buf.WriteString("</loc>\n  </url>\n")
	}
	buf.WriteString("</urlset>\n")

	sitemapCache.payload = buf.Bytes()
	sitemapCache.renderedAt = time.Now()
	return sitemapCache.payload
}

func CreateBasicWebsiteInfo(db *gorm.DB) {
	var count int64
	db.Model(&model.BasicWebsiteInfo{}).Count(&count)

	if count == 0 {
		newInfo := model.BasicWebsiteInfo{
			Name:           "GoX CMS",
			Tagline:        "GoX CMS - A CMS built with Go and Fiber",
			Email:          "contact@goxcms.xyz",
			Phone:          "+1 (123) 456-7890",
			Address:        "123 GoX Street, Fiber City, GO 12345",
			About:          "GoX CMS is a powerful and flexible content management system built with Go and Fiber.",
			LogoURL:        "/static/images/logo.png",
			FaviconURL:     "/static/images/favicon.png",
			FacebookURL:    "https://facebook.com/goxcms",
			TwitterURL:     "https://twitter.com/goxcms",
			LinkedInURL:    "https://linkedin.com/company/goxcms",
			SEOKeywords:    "CMS, Go, Fiber, Web Development",
			SEODescription: "GoX CMS - A fast and flexible content management system for modern websites",
			AnalyticsID:    "UA-XXXXXXXX-X",
			FooterText:     "© 2024 GoX CMS. All rights reserved.",
			Maintenance:    false,
			Theme:          "flatly",
			ContactEmail:   "support@goxcms.com",
			PrivacyPolicy:  "Our privacy policy goes here...",
			TermsOfService: "Our terms of service go here...",
			Language:       "en",
			Locale:         "en-US",
			TimeZone:       "UTC",
			ContainerClass: "container",
		}

		result := db.Create(&newInfo)
		if result.Error != nil {
			log.Fatalf("Failed to create BasicWebsiteInfo: %v", result.Error)
		}
		log.Println("Basic website info created successfully")
	} else {
		// Self-heal rows created before a default existed: an empty container
		// class stretches the whole site full-width, an empty theme breaks the
		// stylesheet link.
		var existing model.BasicWebsiteInfo
		if err := db.First(&existing).Error; err == nil {
			updates := map[string]interface{}{}
			if strings.TrimSpace(existing.ContainerClass) == "" {
				updates["container_class"] = "container"
			}
			if strings.TrimSpace(existing.Theme) == "" {
				updates["theme"] = "flatly"
			}
			if len(updates) > 0 {
				db.Model(&model.BasicWebsiteInfo{}).Where("id = ?", existing.ID).Updates(updates)
				log.Println("Basic website info normalized (empty theme/container filled in)")
			}
		}
	}

	createDefaultAdminUser(db)
}

// createDefaultAdminUser creates the first administrator on a fresh install.
// The password comes from the ADMIN_PASSWORD environment variable or
// app.admin_password; if neither is set a random one is generated and printed
// once to the log.
func createDefaultAdminUser(db *gorm.DB) {
	var count int64
	db.Model(&model.User{}).Where("role_id = ?", model.RoleAdmin).Count(&count)
	if count > 0 {
		return
	}

	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		password = viper.GetString("app.admin_password")
	}
	generated := password == ""
	if generated {
		password = randomString(20)
	} else if len(password) < 12 || len(password) > 72 {
		log.Fatal("The configured admin password must be 12-72 characters")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	email := "admin@goxcms.com"
	newUser := model.User{
		Username:  "admin",
		Password:  string(hashedPassword),
		RoleID:    model.RoleAdmin,
		FirstName: "Admin",
		LastName:  "User",
		Email:     &email,
	}

	if err := db.Create(&newUser).Error; err != nil {
		log.Fatalf("Failed to create admin user: %v", err)
	}

	if generated {
		// Printed exactly once: container/file logs retain it, so treat it
		// as a bootstrap secret and replace it immediately. Prefer setting
		// ADMIN_PASSWORD (or app.admin_password) so no secret is ever logged.
		log.Printf("Default admin user created. Username: admin  Password: %s  (shown once — change it after logging in)", password)
	} else {
		log.Println("Default admin user created with the configured password")
	}
}

var htmlTagPattern = regexp.MustCompile(`<[^>]*>?`)

// htmlToPlainText strips tags and decodes entities. The result is plain text
// and must be escaped before it is written into HTML.
func htmlToPlainText(s string) string {
	return htmlstd.UnescapeString(htmlTagPattern.ReplaceAllString(s, ""))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func ge(a, b int) bool { return a >= b }
func gt(a, b int) bool { return a > b }
func le(a, b int) bool { return a <= b }
func lt(a, b int) bool { return a < b }

// SecureCookies reports whether cookies should carry the Secure flag, which is
// the case whenever the site is served over HTTPS.
func SecureCookies() bool {
	return strings.HasPrefix(viper.GetString("app.url"), "https://")
}
