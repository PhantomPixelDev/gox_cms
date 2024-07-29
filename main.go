package main

import (
	"goxcms/database"
	"goxcms/plugin_system"
	"goxcms/routes"
	"goxcms/utils"
	"log"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/csrf"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

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
	engine.Reload(buildMode != "production")

	app := fiber.New(fiber.Config{
		Views:                   engine,
		Prefork:                 viper.GetBool("server.prefork"),
		CompressedFileSuffix:    ".fiber.gz",
		BodyLimit:               viper.GetInt("server.body_limit") * 1024 * 1024,
		ProxyHeader:             "X-Forwarded-For",
		EnableTrustedProxyCheck: true,
		DisableStartupMessage:   false,
	})

	fsConfig := filesystem.Config{
		Root:   http.Dir("./static"),
		Browse: false,
		Index:  "index.html",
		MaxAge: 3600,
	}

	store := utils.SetupStore(app)

	utils.SetupRateLimiter(app, store)

	routes.SetupRoutes(app, db, store, engine)

	pluginsToRegister := plugin_system.PluginList()
	for _, plugin := range pluginsToRegister {
		plugin_system.RegisterPlugin(plugin, db)
	}

	plugin_system.InitializePlugins(app, db, engine)
	plugin_system.AddPluginManagerRoutes(app, db)

	app.Use("/static", filesystem.New(fsConfig))

	app.Static("/static", "./static", fiber.Static{
		Compress:      true,
		ByteRange:     true,
		CacheDuration: 24 * time.Hour,
	})

	csrfMiddleware := csrf.New(csrf.Config{
		KeyLookup:  "form:csrf",
		CookieName: "csrf",
		ContextKey: "csrf",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusForbidden).SendString(err.Error())
		},
	})

	app.Use(cors.New(cors.Config{
		AllowCredentials: viper.GetBool("cors.allow_credentials"),
		AllowOrigins:     viper.GetString("cors.allow_origins"),
	}))

	app.Use(func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).Render("404", fiber.Map{
			"Title":    "404 - Page Not Found",
			"Settings": c.Locals("Settings"),
		}, "main")
	})

	app.Use(csrfMiddleware)

	app.Use(func(c *fiber.Ctx) error {
		c.Locals("captcha_enabled", viper.GetBool("captcha.enabled"))
		return c.Next()
	})

	utils.GenerateSiteMap(db)
	utils.CreateBasicWebsiteInfo(db)

	return app
}
