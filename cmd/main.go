package main

import (
	"fmt"
	handlers "goxcms/handler"
	"goxcms/model"
	"goxcms/plugin_system"
	"goxcms/plugins/latest_posts_plugin"
	html2 "html"
	"html/template"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/gofiber/template/html/v2"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/dbresolver"

	/// add redis for session store
	"github.com/gofiber/storage/redis/v3"
)

func main() {

	// setup config
	initConfig()

	// setup database
	db := initDB()

	// setup fiber app
	app := setupFiberApp()

	// setup store and cache
	store := setupStore(app)

	//store.Reset() // Clear all sessions

	// setup rate limiter
	setupRateLimiter(app, store)

	// setup routes
	setupRoutes(app, db, store)

	// generate sitemap
	generateSiteMap(app, db)

	createBasicWebsiteInfo(db)

	// setup cors

	app.Use(cors.New(cors.Config{
		AllowCredentials: true,
		AllowOrigins:     viper.GetString("cors.allow_origins"),
	}))

	// Register plugins here
	plugin_system.RegisterPlugin(&latest_posts_plugin.LatestPostsPlugin{})

	//plugin_system.RegisterPlugin(&logger_plugin.LoggerPlugin{})

	plugin_system.InitializePlugins(app, db)

	host := viper.GetString("server.host")
	port := viper.GetString("server.port")

	log.Fatal(app.Listen(host + ":" + port))
}

func setupStore(app *fiber.App) *session.Store {
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
			CookieSecure:   runtime.GOOS != "windows",
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
			CookieSecure:   runtime.GOOS != "windows",
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
		log.Fatal("Store is nil")
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

func setupRateLimiter(app *fiber.App, store *session.Store) {
	if viper.GetBool("ratelimiter.enabled") {
		println("Rate limiter enabled")
		app.Use(limiter.New(limiter.Config{
			Max:        viper.GetInt("ratelimiter.max_requests"),
			Expiration: 30 * time.Second,
			KeyGenerator: func(c *fiber.Ctx) string {
				return c.IP()
			},
			LimitReached: func(c *fiber.Ctx) error {
				return c.SendString("Limit reached")
			},
			Storage: store.Storage,
		}))
	} else {
		println("Rate limiter not enabled")
	}
}

func initConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Fatal error config file: %s \n", err)
	}

	viper.AutomaticEnv()

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

	if viper.GetBool("redis.enabled") {
		println("REDIS ENABLED")
	}
}

func initDB() *gorm.DB {
	var db *gorm.DB
	var err error
	databaseDriver := viper.GetString("database.driver")

	databasePath := ""

	//// setup new connection pool for gorm
	config_gorrm := &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger.Default.LogMode(logger.Silent),
	}

	switch databaseDriver {
	case "mysql":
		databasePath = viper.GetString("database.mysql.dsn")
		db, err = gorm.Open(mysql.Open(databasePath), config_gorrm)
	case "postgres":
		databasePath = viper.GetString("database.postgres.dsn")
		db, err = gorm.Open(postgres.Open(databasePath), config_gorrm)
	case "sqlite":
		databasePath = viper.GetString("database.sqlite.dsn")
		db, err = gorm.Open(sqlite.Open(databasePath), config_gorrm)
	default:
		panic("database driver not supported")
	}

	if err != nil {
		panic("failed to connect database")
	}

	db.Use(dbresolver.Register(dbresolver.Config{
		Replicas: []gorm.Dialector{db.Dialector},
		Policy:   dbresolver.RandomPolicy{},
	}))

	//// migrate database models
	db.AutoMigrate(
		&model.User{},
		&model.Post{},
		&model.Category{},
		&model.Tag{},
		&model.Menu{},
		&model.MenuItem{},
		&model.BasicWebsiteInfo{},
		&model.CustomPage{},
		&model.File{},
		&model.Comment{},
	)

	//// add foreign key constraints
	db.Migrator().CreateConstraint(&model.Post{}, "CategoryID")
	db.Migrator().CreateConstraint(&model.Post{}, "TagID")
	db.Migrator().CreateConstraint(&model.Post{}, "UserID")
	db.Migrator().CreateConstraint(&model.Post{}, "MenuID")
	db.Migrator().CreateConstraint(&model.Post{}, "MenuItemID")
	db.Migrator().CreateConstraint(&model.Post{}, "CustomPageID")

	//// add foreign key constraints
	db.Migrator().CreateConstraint(&model.Category{}, "PostID")
	db.Migrator().CreateConstraint(&model.Category{}, "TagID")

	/// comments foreign key constraints
	db.Migrator().CreateConstraint(&model.Comment{}, "UserID")
	db.Migrator().CreateConstraint(&model.Comment{}, "PostID")

	return db
}

func setupEngine() *html.Engine {
	var engine *html.Engine = nil

	engine = html.New("./views", ".html")

	engine.AddFunc("timestamp", func() string {
		return fmt.Sprintf("?v=%d", time.Now().Unix())
	})

	engine.AddFunc("unescape", func(s string) template.HTML {
		return template.HTML(s)
	})

	engine.AddFunc("truncate", func(s string, length int) string {
		if len(s) > length {
			return s[:length] + "..."
		}
		return s
	})

	engine.AddFunc(
		// add unescape function
		"unescape", func(s string) template.HTML {
			return template.HTML(s)
		},
	)

	engine.AddFunc("add", func(a, b int) int {
		return a + b
	})

	/// add sub functions

	engine.AddFunc("sub", func(a, b int) int {
		return a - b
	})

	engine.AddFunc("sequence", func(start, end int) []int {
		var seq []int
		for i := start; i <= end; i++ {
			seq = append(seq, i)
		}
		return seq
	})

	engine.AddFunc("default", func(value, defaultValue string) string {
		if value == "" {
			return defaultValue
		}
		return value
	})

	engine.AddFunc("count_post", func(value []model.Post) int {
		/// get the count of posts in the tag array
		for _, post := range value {
			return len(post.Tags)
		}
		return 0

	})

	engine.AddFunc("removeHTML", func(s string) string {
		return html2.EscapeString(s)
	})

	return engine

}

func setupFiberApp() *fiber.App {

	engine := setupEngine()

	build_mode := viper.GetString("build.mode")

	if build_mode == "production" {
		engine.Debug(false)
		engine.Reload(false)
	} else {

		engine.Debug(true)
		engine.Reload(true)
	}

	app := fiber.New(fiber.Config{
		Views:                engine,
		Prefork:              viper.GetBool("server.prefork"),
		CompressedFileSuffix: ".fiber.gz",
		/// max upload size 10MB
		BodyLimit: viper.GetInt("server.body_limit") * 1024 * 1024,
		//// app is behind reverse proxy (nginx) so we need to set this to true to get the real ip
		ProxyHeader:             "X-Forwarded-For",
		EnableTrustedProxyCheck: true, // Enable trusted proxy check to get the real IP when behind a reverse proxy (e.g., nginx)
		DisableStartupMessage:   false,
	})

	// Filesystem configuration for serving files from ./static/uploads
	fsConfig := filesystem.Config{
		Root:   http.Dir("./static"),
		Browse: false,        // Disable directory listing
		Index:  "index.html", // Optional: default file to serve
		MaxAge: 3600,         // Set cache control max age for browser caching
	}

	app.Use("/static", filesystem.New(fsConfig))

	app.Static("/static", "./static",
		fiber.Static{
			Compress:      true,
			ByteRange:     true,
			CacheDuration: 24 * time.Hour,
		},
	)

	return app
}

func setupRoutes(app *fiber.App, db *gorm.DB, store *session.Store) {

	app.Use(handlers.AuthStatusMiddleware(db))

	app.Use(func(c *fiber.Ctx) error {

		settings_cms := model.BasicWebsiteInfo{}

		db.First(&settings_cms)

		// Map the website settings to a map for easy access
		settings_cms_db := map[string]string{
			"Name":           settings_cms.Name,
			"Tagline":        settings_cms.Tagline,
			"Email":          settings_cms.Email,
			"Phone":          settings_cms.Phone,
			"Address":        settings_cms.Address,
			"About":          settings_cms.About,
			"LogoURL":        settings_cms.LogoURL,
			"FaviconURL":     settings_cms.FaviconURL,
			"FacebookURL":    settings_cms.FacebookURL,
			"TwitterURL":     settings_cms.TwitterURL,
			"LinkedInURL":    settings_cms.LinkedInURL,
			"SEOKeywords":    settings_cms.SEOKeywords,
			"SEODescription": settings_cms.SEODescription,
			"AnalyticsID":    settings_cms.AnalyticsID,
			"FooterText":     settings_cms.FooterText,
			"Theme":          settings_cms.Theme,
			"ContactEmail":   settings_cms.ContactEmail,
			"PrivacyPolicy":  settings_cms.PrivacyPolicy,
			"TermsOfService": settings_cms.TermsOfService,
			"Language":       settings_cms.Language,
			"Locale":         settings_cms.Locale,
			"TimeZone":       settings_cms.TimeZone,
		}

		c.Locals("Settings", settings_cms_db)
		return c.Next()
	})

	hotload_custom_pages := viper.GetBool("app.hotload_custom_pages")

	println("HOTLOAD_CUSTOM_PAGES", hotload_custom_pages)

	if hotload_custom_pages {

		app.Use(func(c *fiber.Ctx) error {
			// Get the path from the request
			path := c.Path()

			path = strings.TrimPrefix(path, "/")

			var customPage model.CustomPage
			if err := db.Where("slug = ?", path).First(&customPage).Error; err != nil {
				return c.Next()
			}
			// Render the custom page with the custom page data
			return c.Render("page/page", fiber.Map{
				"Title":    customPage.Title,
				"Content":  customPage.Content,
				"Settings": c.Locals("Settings"),
			}, "main")
		})

	} else {

		var customPages []model.CustomPage
		db.Find(&customPages)

		println("Generating Custom Page Routes:", len(customPages))

		for _, customPage := range customPages {
			println("Custom Page Route Created:", customPage.Slug)
			app.Get("/"+customPage.Slug, func(cp model.CustomPage) func(*fiber.Ctx) error {
				return func(c *fiber.Ctx) error {
					return c.Render("page/page", fiber.Map{
						"Title":    cp.Title,
						"Content":  cp.Content,
						"Settings": c.Locals("Settings"),
					}, "main")
				}
			}(customPage))
		}
	}

	/// add loaderio-422dc4c70ddacc89acd6a63f82d42134 for domain verification
	app.Get("/loaderio-422dc4c70ddacc89acd6a63f82d42134", func(c *fiber.Ctx) error {
		return c.SendString("loaderio-422dc4c70ddacc89acd6a63f82d42134")
	})

	app.Get("/", func(c *fiber.Ctx) error {

		return c.Render("index", fiber.Map{
			"Title":      "GoX CMS - HomePage",
			"IsLoggedIn": c.Locals("isLoggedin"),
			"IsAdmin":    c.Locals("isAdmin"),
			"Settings":   c.Locals("Settings"),
		}, "main")
	})

	/// clear cache route
	app.Post("/clear-cache", handlers.IsAdmin, func(c *fiber.Ctx) error {

		store.Reset()

		handlers.ShowToast(c, "Cache cleared successfully")

		return nil
	})

	app.Get("/register", func(c *fiber.Ctx) error {

		if c.Locals("isLoggedin") == true {
			c.Redirect("/")
			return nil
		}

		return c.Render("register", fiber.Map{
			"Title":    "Register",
			"Settings": c.Locals("Settings"),
		}, "main")
	})

	app.Post("/register", handlers.Register(db))

	app.Get("/login", func(c *fiber.Ctx) error {

		if c.Locals("isLoggedin") == true {
			c.Redirect("/")
			return nil
		}

		return c.Render("login", fiber.Map{
			"Title":    "Login",
			"Settings": c.Locals("Settings"),
		}, "main")
	})

	app.Post("/login", handlers.Login(db, store))

	app.Post("/logout", func(c *fiber.Ctx) error {

		return handlers.Logout(c)
	})

	app.Get("/admin-settings", handlers.IsAdmin, func(c *fiber.Ctx) error {

		settings_cms := model.BasicWebsiteInfo{}

		if err := db.First(&settings_cms).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}

		themes_list := []string{"cerulean", "cosmo", "cyborg", "darkly", "flatly", "journal", "litera", "lumen", "lux", "materia", "minty", "pulse", "sandstone", "simplex", "sketchy", "slate", "solar", "spacelab", "superhero", "united", "yeti", "morph", "quartz", "vapor", "zephyr"}

		return c.Render("website_settings", fiber.Map{
			"Title":         "Admin Settings",
			"Settings":      c.Locals("Settings"),
			"SettingsAdmin": handlers.MapSettingsToMap(settings_cms),
			"Themes":        themes_list,
		})

	})

	app.Post("/update-settings", handlers.IsAdmin, func(c *fiber.Ctx) error {

		return handlers.UpdateSettings(c, db)
	})

	app.Post("/toggle-post-status", handlers.IsAdmin, func(c *fiber.Ctx) error {

		return handlers.TogglePostStatus(c, db)
	})

	app.Get("/search-posts", func(c *fiber.Ctx) error {
		return handlers.AdminSearchPosts(c, db)
	})

	app.Delete("/delete-post/:id", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AdminDeletePost(c, db)
	})

	app.Get("/search-tags", func(c *fiber.Ctx) error {
		return handlers.SearchTag(c, db)
	})

	app.Delete("/delete-tag", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteTag(c, db)
	})

	/// add tag
	app.Post("/add-tag", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AddTag(c, db)
	})

	/// add menu
	app.Post("/add-menu", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AddMenu(c, db)

	})

	// add menu item to menu
	app.Post("/add-menu-item", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AddMenuItem(c, db)
	})

	// delete menu item
	app.Delete("/delete-menu-item/:id", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteMenuItem(c, db)
	})

	// delete menu
	app.Delete("/delete-menu/:id", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteMenu(c, db)
	})

	// edit menu
	app.Post("/edit-menu/:id", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.EditMenu(c, db)
	})

	/// remove submenu from menu
	app.Delete("/remove-submenu/:id", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.RemoveSubmenuFromMenu(c, db)
	})

	// edit menu item
	app.Post("/edit-menu-item/:id", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.EditMenuItem(c, db)
	})

	/// create get view for edit menu and menu item return modal htmx view
	app.Get("/edit-menu/:id", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.EditMenuView(c, db)
	})

	/// create get view for edit menu and menu item return modal htmx view
	app.Get("/edit-menu-item/:id", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.EditMenuItemView(c, db)
	})

	app.Get("/search-menu", func(c *fiber.Ctx) error {
		return handlers.SearchMenuAdminTable(c, db)
	})

	app.Get("/get-primary-menu", func(c *fiber.Ctx) error {
		return handlers.GetPrimaryMenuRender(c, db)
	})

	app.Get("/search-users", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.SearchUsers(c, db)
	})

	/// add comments table for admin view
	app.Get("/search-comments", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.SearchCommentsView(c, db)
	})

	/// toggle comment status#
	app.Post("/toggle-comment-status/:id", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.ToggleCommentStatus(c, db)
	})

	/// delete comment
	app.Delete("/delete-comment/:id", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteComment(c, db)
	})

	app.Delete("/delete-user/:id", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteUser(c, db)
	})

	app.Get("/search-categories", func(c *fiber.Ctx) error {
		return handlers.SearchCategories(c, db)
	})

	app.Post("/add-category", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AddCategory(c, db)
	})

	app.Delete("/delete-category", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteCategory(c, db)
	})

	app.Get("/search-custompages", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.SearchCustomPages(c, db)
	})

	/// add custom page
	app.Post("/add-custompage", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AddCustomPage(c, db, app)
	})

	/// get view for add custom page
	app.Get("/add-custompage", func(c *fiber.Ctx) error {
		if c.Locals("isAdmin") == false {
			return c.Redirect("/")
		}
		return c.Render("page/page_add", fiber.Map{
			"TitleView": "Add Custom Page",
			"Settings":  c.Locals("Settings"),
		}, "main")
	})

	/// edit custom page
	app.Get("/edit-custompage/:id", func(c *fiber.Ctx) error {
		if c.Locals("isAdmin") == false {
			return c.Redirect("/")
		}

		id, err := c.ParamsInt("id")
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString(err.Error())
		}

		var customPage model.CustomPage
		if err := db.First(&customPage, id).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}

		return c.Render("page/page_edit", fiber.Map{
			"Title":   customPage.Title,
			"Content": customPage.Content,
			"ID":      customPage.ID,
			"Slug":    customPage.Slug,
		}, "main")
	})

	app.Post("/edit-custompage", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.EditCustomPage(c, db)
	})

	/// delete custom page
	app.Delete("/delete-custompage/:id", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteCustomPage(c, db)
	})

	app.Get("/search-files", func(c *fiber.Ctx) error {
		return handlers.SearchFiles(c, db)
	})

	/// add blog comment to post
	app.Post("/add-comment", handlers.IsLoggedIn, func(c *fiber.Ctx) error {
		return handlers.AddComment(c, db)
	})

	app.Post("/upload-file", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.UploadFile(c, db)
	})

	app.Delete("/delete-file", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteFile(c, db)
	})

	app.Get("/blog/:page?", func(c *fiber.Ctx) error {
		return handlers.BlogPage(c, db)
	})

	app.Get("/admin/post/add", handlers.IsAdmin, func(c *fiber.Ctx) error {

		var categories []model.Category
		var tags []model.Tag

		db.Find(&categories)
		db.Find(&tags)

		c.Set("HX-Trigger", "Action: addPost")

		html_basic_test := "<p>Write your post here</p>"
		// Render the view with the categories and tags

		return c.Render("admin/admin_post_add", fiber.Map{
			"Title":      "Add Post",
			"Categories": categories,
			"Tags":       tags,
			"Content":    template.HTML(html_basic_test),
			"IsAdmin":    c.Locals("isAdmin"),
			"IsLoggedIn": c.Locals("isLoggedin"),
			"Settings":   c.Locals("Settings"),
		}, "main")
	})

	app.Post("/admin/post/add", func(c *fiber.Ctx) error {

		return handlers.AdminAddBlogPost(c, db)
	})

	app.Get("/admin/post/edit/:post_id", func(c *fiber.Ctx) error {
		return handlers.AdminEditBlogPost(c, db)
	})

	app.Post("/admin/post/edit", func(c *fiber.Ctx) error {
		return handlers.AdminUpdateBlogPost(c, db)
	})

	app.Get("/blog/post/:slug", func(c *fiber.Ctx) error {
		return handlers.BlogPostPage(c, db)
	})

	app.Get("/blog/category/:slug/:page?", func(c *fiber.Ctx) error {
		return handlers.BlogCategoryPage(c, db)
	})

	app.Get("/blog/tag/:slug/:page?", func(c *fiber.Ctx) error {
		return handlers.BlogTagPage(c, db)
	})

	app.Get("/admin", handlers.IsAdmin, func(c *fiber.Ctx) error {

		return c.Render("admin/admin", fiber.Map{
			"Title":      "Admin Panel",
			"IsAdmin":    c.Locals("isAdmin"),
			"IsLoggedIn": c.Locals("isLoggedin"),
			"Settings":   c.Locals("Settings"),
		}, "main")
	})

	app.Get("/sitemap.xml", func(c *fiber.Ctx) error {
		return c.SendFile("./static/sitemap.xml")
	})

}

func generateSiteMap(app *fiber.App, db *gorm.DB) {
	baseURL := viper.GetString("base_url")

	pages := []string{
		"/",
		"/blog",
		"/login",
		"/register",
	}

	var posts []model.Post
	db.Where("published = ?", true).Find(&posts)

	var users []model.User
	db.Find(&users)

	var categories []model.Category
	db.Find(&categories)

	var tags []model.Tag
	db.Find(&tags)

	var custompages []model.CustomPage
	db.Where("published = ?", true).Find(&custompages)

	var urls []string

	for _, post := range posts {
		urls = append(urls, "/blog/post/"+post.Slug)
	}

	for _, user := range users {
		urls = append(urls, "/user/"+strconv.Itoa(int(user.ID)))
	}

	for _, category := range categories {
		urls = append(urls, "/blog/category/"+category.Slug)
	}

	for _, tag := range tags {
		urls = append(urls, "/blog/tag/"+tag.Slug)
	}

	for _, custompage := range custompages {
		urls = append(urls, "/"+custompage.Slug)
	}

	urls = append(pages, urls...)

	filePath := "./static/sitemap.xml"
	file, err := os.Create(filePath)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	_, err = file.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	if err != nil {
		log.Fatal(err)
	}
	_, err = file.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")
	if err != nil {
		log.Fatal(err)
	}

	for _, url := range urls {
		_, err = file.WriteString("<url>\n<loc>" + baseURL + url + "</loc>\n</url>\n")
		if err != nil {
			log.Fatal(err)
		}
	}

	_, err = file.WriteString("</urlset>")
	if err != nil {
		log.Fatal(err)
	}

	println("Sitemap generated successfully")
}

func createBasicWebsiteInfo(db *gorm.DB) {
	var basicWebsiteInfo model.BasicWebsiteInfo
	db.First(&basicWebsiteInfo)
	if basicWebsiteInfo.ID == 0 {
		db.Create(&model.BasicWebsiteInfo{
			Name:           "GoX CMS",
			Tagline:        "GoX CMS - A CMS built with Go and Fiber",
			Email:          "basic email",
			Phone:          "basic phone",
			Address:        "basic address",
			About:          "basic about",
			LogoURL:        "/static/images/logo.png",
			FaviconURL:     "/static/images/favicon.png",
			FacebookURL:    "https://facebook.com",
			TwitterURL:     "https://twitter.com",
			LinkedInURL:    "https://linkedin.com",
			SEOKeywords:    "basic seo keywords",
			SEODescription: "basic seo description",
			AnalyticsID:    "basic analytics id",
			FooterText:     "basic footer text",
			Maintenance:    false,
			Theme:          "vapor",
			ContactEmail:   "basic contact email",
			PrivacyPolicy:  "basic privacy policy",
			TermsOfService: "basic terms of service",
			Language:       "en",
			Locale:         "en-US",
			TimeZone:       "UTC",
			SelectedTheme:  "vapor",
		})
	}

	println("Basic website info created successfully")
	/// create a default user admin if not exists
	var user model.User
	hashed_password, err := handlers.HashPassword("admin1234")
	if err != nil {
		log.Fatal(err)
	}

	db.First(&user, "username = ?", "admin1234")
	if user.ID == 0 {
		db.Create(&model.User{
			Username:  "admin1234",
			Password:  hashed_password,
			RoleID:    2,
			FirstName: "Admin",
			LastName:  "User",
			Email:     nil,
		})
	}

	println("Default admin user created successfully")
}
