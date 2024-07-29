package utils

import (
	"bufio"
	"fmt"
	"goxcms/model"
	"html/template"
	"log"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
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
	viper.SetDefault("build.mode", "development")
	viper.SetDefault("database.driver", "sqlite")
	viper.SetDefault("database.sqlite.dsn", "./goxcms.db")
	viper.SetDefault("upload.max_size_mb", 50)
	viper.SetDefault("ratelimiter.enabled", false)
	viper.SetDefault("ratelimiter.max_requests", 10)
	viper.SetDefault("cors.allow_origins", "*")
	viper.SetDefault("redis.enabled", false)
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.username", "")
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.database", 0)
	viper.SetDefault("redis.pool_size", 10)
	viper.SetDefault("server.body_limit", 10)
	viper.SetDefault("app.hotload_custom_pages", false)
	viper.SetDefault("captcha.public_key", "")
	viper.SetDefault("captcha.secret_key", "")
	viper.SetDefault("captcha.enabled", false)

	if viper.GetBool("redis.enabled") {
		log.Println("Redis enabled")
	}
}

func SetupEngine() *html.Engine {
	engine := html.New("./views", ".html")

	funcMap := template.FuncMap{
		"timestamp": func() string {
			return fmt.Sprintf("?v=%d", time.Now().Unix())
		},
		"truncate": func(s string, length int) string {
			if len(s) > length {
				return s[:length] + "..."
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
		"escape": func(s string) template.HTML {
			return template.HTML(htmlToPlainText(s))
		},
		"unescape": func(s string) template.HTML {
			return template.HTML(s)
		},

		"max": max,
		"min": min,
		"ge":  ge,
		"gt":  gt,
		"le":  le,
		"lt":  lt,
	}

	engine.AddFuncMap(funcMap)

	return engine
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
			CookieSecure:   !isWindows(),
			Storage:        redisStorage,
		})

		app.Use(cache.New(cache.Config{
			Next: func(c *fiber.Ctx) bool {
				return c.Get("X-No-Cache") == "true"
			},
			Expiration: 30 * time.Minute,
			Storage:    redisStorage,
		}))
	} else {
		store = session.New(session.Config{
			Expiration:     24 * time.Hour,
			CookieHTTPOnly: true,
			CookieSecure:   !isWindows(),
		})

		app.Use(cache.New(cache.Config{
			Next: func(c *fiber.Ctx) bool {
				return c.Get("X-No-Cache") == "true"
			},
			Expiration: 30 * time.Minute,
			Storage:    store.Storage,
		}))
	}

	if store == nil {
		log.Fatal("Store initialization failed")
	}

	app.Use(func(c *fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}
		c.Locals("session", sess)
		return c.Next()
	})

	return store
}

func SetupRateLimiter(app *fiber.App, store *session.Store) {
	if viper.GetBool("ratelimiter.enabled") {
		log.Println("Rate limiter enabled")
		app.Use(limiter.New(limiter.Config{
			Max:        viper.GetInt("ratelimiter.max_requests"),
			Expiration: 30 * time.Second,
			KeyGenerator: func(c *fiber.Ctx) string {
				return c.IP()
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

func GenerateSiteMap(db *gorm.DB) {
	baseURL := viper.GetString("app.url")
	urls := []string{"/", "/blog", "/login", "/register"}

	// Use a single query to fetch all required data
	var posts []model.Post
	var users []model.User
	var categories []model.Category
	var tags []model.Tag
	var customPages []model.CustomPage

	db.Where("published = ?", true).Find(&posts)
	db.Select("id").Find(&users)
	db.Select("slug").Find(&categories)
	db.Select("slug").Find(&tags)
	db.Where("published = ?", true).Select("slug").Find(&customPages)

	// Pre-allocate the urls slice
	totalURLs := len(urls) + len(posts) + len(users) + len(categories) + len(tags) + len(customPages)
	urls = make([]string, 0, totalURLs)

	for _, post := range posts {
		urls = append(urls, "/blog/post/"+post.Slug)
	}

	for _, user := range users {
		urls = append(urls, "/user/"+strconv.FormatUint(uint64(user.ID), 10))
	}

	for _, category := range categories {
		urls = append(urls, "/blog/category/"+category.Slug)
	}

	for _, tag := range tags {
		urls = append(urls, "/blog/tag/"+tag.Slug)
	}

	for _, customPage := range customPages {
		urls = append(urls, "/"+customPage.Slug)
	}

	writeSitemapToFile(baseURL, urls)
}

func writeSitemapToFile(baseURL string, urls []string) {
	filePath := "./static/sitemap.xml"
	file, err := os.Create(filePath)
	if err != nil {
		log.Fatalf("Failed to create sitemap file: %v", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	writer.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
`)

	for _, url := range urls {
		writer.WriteString(fmt.Sprintf("  <url>\n    <loc>%s%s</loc>\n  </url>\n", baseURL, url))
	}

	writer.WriteString("</urlset>")

	log.Println("Sitemap generated successfully")
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
			Theme:          "vapor",
			ContactEmail:   "support@goxcms.com",
			PrivacyPolicy:  "Our privacy policy goes here...",
			TermsOfService: "Our terms of service go here...",
			Language:       "en",
			Locale:         "en-US",
			TimeZone:       "UTC",
			SelectedTheme:  "vapor",
		}

		result := db.Create(&newInfo)
		if result.Error != nil {
			log.Fatalf("Failed to create BasicWebsiteInfo: %v", result.Error)
		}
		log.Println("Basic website info created successfully")
	}

	createDefaultAdminUser(db)
}

func createDefaultAdminUser(db *gorm.DB) {
	var count int64
	db.Model(&model.User{}).Where("username = ?", "admin").Count(&count)

	if count == 0 {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte("admin1234"), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("Failed to hash password: %v", err)
		}

		email := "admin@goxcms.com"
		newUser := model.User{
			Username:  "admin",
			Password:  string(hashedPassword),
			RoleID:    2, // Assuming 2 is the admin role ID
			FirstName: "Admin",
			LastName:  "User",
			Email:     &email,
		}

		result := db.Create(&newUser)
		if result.Error != nil {
			log.Fatalf("Failed to create admin user: %v", result.Error)
		}
		log.Println("Default admin user created successfully")
	}
}

func htmlToPlainText(html string) string {
	// Remove HTML tags using a regular expression
	re := regexp.MustCompile(`\<[^>]*\>`)
	plainText := re.ReplaceAllString(html, "")
	// Replace HTML entities with their plain text equivalents
	plainText = strings.ReplaceAll(plainText, "&amp;", "&")
	plainText = strings.ReplaceAll(plainText, "&lt;", "<")
	plainText = strings.ReplaceAll(plainText, "&gt;", ">")
	plainText = strings.ReplaceAll(plainText, "&quot;", "\"")
	plainText = strings.ReplaceAll(plainText, "&#39;", "'")
	return plainText
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

func isWindows() bool {
	return runtime.GOOS == "windows"
}
