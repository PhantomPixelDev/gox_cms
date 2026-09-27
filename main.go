package main

import (
	"goxcms/database"
	handlers "goxcms/handler"
	"goxcms/plugin_system"
	"goxcms/routes"
	"goxcms/utils"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/csrf"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

// jwtSessionHours mirrors the JWT lifetime (app.session_hours) so the CSRF
// cookie cannot outlive the session it protects.
func jwtSessionHours() time.Duration {
	hours := viper.GetInt("app.session_hours")
	if hours <= 0 {
		hours = 12
	}
	return time.Duration(hours) * time.Hour
}

func main() {

	utils.InitConfig()

	db := database.InitDB()

	defer func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}()

	app := setupFiberApp(db)

	host := viper.GetString("server.host")
	port := viper.GetString("server.port")

	log.Fatal(app.Listen(host + ":" + port))
}

func setupFiberApp(db *gorm.DB) *fiber.App {

	engine := utils.SetupEngine()

	buildMode := viper.GetString("build.mode")

	engine.Debug(buildMode != "production")
	// In development, templates are re-parsed on every render so edits show up
	// without a restart. In production they are frozen, and
	// handlers.ReloadTemplates clears Engine.Loaded so a newly created theme
	// is picked up when an admin activates it.
	engine.Reload(buildMode != "production")
	handlers.SetTemplateEngine(engine)
	handlers.InitThemeSystem()

	trustedProxies := viper.GetStringSlice("server.trusted_proxies")

	app := fiber.New(fiber.Config{
		Views:                   engine,
		Prefork:                 viper.GetBool("server.prefork"),
		CompressedFileSuffix:    ".fiber.gz",
		BodyLimit:               viper.GetInt("server.body_limit") * 1024 * 1024,
		ProxyHeader:             fiber.HeaderXForwardedFor,
		EnableTrustedProxyCheck: true,
		TrustedProxies:          trustedProxies,
		DisableStartupMessage:   false,
	})

	// Middleware order matters: Fiber runs app.Use handlers in registration
	// order, and anything registered after the 404 catch-all never runs.

	// Recover from handler panics so one bad request cannot take the server down.
	app.Use(recover.New())

	// Static files are served before the auth and settings middleware so they
	// do not cost a database round trip per asset.
	app.Static("/static", "./static", fiber.Static{
		Compress:      true,
		ByteRange:     true,
		Browse:        false,
		CacheDuration: 24 * time.Hour,
		MaxAge:        3600,
	})

	app.Use(cors.New(cors.Config{
		AllowCredentials: viper.GetBool("cors.allow_credentials"),
		AllowOrigins:     strings.Join(corsOrigins(), ","),
	}))

	store := utils.SetupStore(app)

	utils.SetupRateLimiter(app, store)

	// CSRF: the token lives in the csrf_ cookie and must be echoed back in the
	// X-Csrf-Token header on unsafe requests. views/main.html adds the header
	// to every HTMX request.
	//
	// The token is bound to the session (Session/SessionKey) rather than kept
	// in a global storage namespace keyed by the token value. With a flat
	// store, any valid token was accepted on any request, including by a
	// different user, and it never rotated. Because Login calls
	// sess.Regenerate(), binding it here also rotates the token on login: the
	// new session has no token, so the next safe request issues a fresh one
	// and the pre-login token stops working.
	app.Use(csrf.New(csrf.Config{
		KeyLookup:      "header:" + csrf.HeaderName,
		Session:        store,
		SessionKey:     "gox.csrf.token",
		CookieName:     "csrf_",
		CookiePath:     "/",
		CookieSameSite: "Lax",
		CookieSecure:   utils.SecureCookies(),
		// Match the JWT lifetime rather than outliving it.
		Expiration: jwtSessionHours(),
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			handlers.ShowToastError(c, "Your session expired, please reload the page and try again")
			return c.Status(fiber.StatusForbidden).SendString("Forbidden: invalid CSRF token")
		},
	}))

	routes.SetupRoutes(app, db, store, engine)

	pluginsToRegister := plugin_system.PluginList()
	for _, plugin := range pluginsToRegister {
		plugin_system.RegisterPlugin(plugin, db)
	}

	plugin_system.InitializePlugins(app, db, engine)
	plugin_system.AddPluginManagerRoutes(app, db)

	// Custom pages match any remaining path, so they go after all other routes.
	routes.SetupCustomPageRoutes(app, db)

	// 404 catch-all: must stay last.
	app.Use(func(c *fiber.Ctx) error {
		return handlers.RenderNotFound(c)
	})

	utils.CreateBasicWebsiteInfo(db)
	handlers.SeedDemoContent(db)

	return app
}

// corsOrigins returns the configured allowed origins, defaulting to the site's
// own URL.
func corsOrigins() []string {
	origins := viper.GetStringSlice("cors.allowed_origins")
	if len(origins) == 0 {
		if url := viper.GetString("app.url"); url != "" {
			origins = []string{url}
		}
	}
	return origins
}
