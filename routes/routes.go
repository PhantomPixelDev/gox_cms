// routes/routes.go
package routes

import (
	"html/template"
	"strconv"
	"strings"

	handlers "goxcms/handler"
	"goxcms/model"
	"goxcms/plugin_system"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/gofiber/template/html/v2"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

func SetupRoutes(app *fiber.App, db *gorm.DB, store *session.Store, engine *html.Engine) {

	app.Use(handlers.AuthStatusMiddleware(db))

	app.Use(func(c *fiber.Ctx) error {

		settings_cms := model.BasicWebsiteInfo{}

		db.First(&settings_cms)

		captcha_enabled := viper.GetBool("captcha.enabled")

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
			"CaptchaEnabled": strconv.FormatBool(captcha_enabled),
			"CaptchaSiteKey": viper.GetString("captcha.public_key"),
			"ContainerClass": settings_cms.ContainerClass,
		}

		c.Locals("Settings", settings_cms_db)
		return c.Next()
	})

	hotload_custom_pages := viper.GetBool("app.hotload_custom_pages")

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
			return c.Render("page/"+customPage.Template, fiber.Map{
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
					return c.Render("page/"+cp.Template, fiber.Map{
						"Title":    cp.Title,
						"Content":  template.HTML(cp.Content),
						"Settings": c.Locals("Settings"),
					}, "main")
				}
			}(customPage))
		}
	}

	app.Get("/", func(c *fiber.Ctx) error {

		return c.Render("index", fiber.Map{
			"Title":      "GoX CMS - HomePage",
			"IsLoggedIn": c.Locals("isLoggedin"),
			"IsAdmin":    c.Locals("isAdmin"),
			"Settings":   c.Locals("Settings"),
		}, "main")
	})

	app.Post("/clear-cache", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {

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

	app.Get("/admin-settings", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {

		settings_cms := model.BasicWebsiteInfo{}

		if err := db.First(&settings_cms).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}

		themes_list := []string{"cerulean", "cosmo", "cyborg", "darkly", "flatly", "journal", "litera", "lumen", "lux", "materia", "minty", "pulse", "sandstone", "simplex", "sketchy", "slate", "solar", "spacelab", "superhero", "united", "yeti", "morph", "quartz", "vapor", "zephyr"}
		containers_list := []string{"container", "container-fluid"}
		return c.Render("website_settings", fiber.Map{
			"Title":         "Admin Settings",
			"Settings":      c.Locals("Settings"),
			"SettingsAdmin": handlers.MapSettingsToMap(settings_cms),
			"Themes":        themes_list,
			"Containers":    containers_list,
		})

	})

	app.Post("/update-settings", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {

		return handlers.UpdateSettings(c, db)
	})

	app.Post("/toggle-post-status", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {

		return handlers.TogglePostStatus(c, db)
	})

	app.Get("/search-posts", func(c *fiber.Ctx) error {
		return handlers.AdminSearchPosts(c, db)
	})

	app.Delete("/delete-post/:id", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AdminDeletePost(c, db)
	})

	app.Get("/search-tags", func(c *fiber.Ctx) error {
		return handlers.SearchTag(c, db)
	})

	app.Delete("/delete-tag", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteTag(c, db)
	})

	/// add tag
	app.Post("/add-tag", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AddTag(c, db)
	})

	/// add menu
	app.Post("/add-menu", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AddMenu(c, db)

	})

	// add menu item to menu
	app.Post("/add-menu-item", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AddMenuItem(c, db)
	})

	// delete menu item
	app.Delete("/delete-menu-item/:id", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteMenuItem(c, db)
	})

	// delete menu
	app.Delete("/delete-menu/:id", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteMenu(c, db)
	})

	// edit menu
	app.Post("/edit-menu/:id", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.EditMenu(c, db)
	})

	/// remove submenu from menu
	app.Delete("/remove-submenu/:id", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.RemoveSubmenuFromMenu(c, db)
	})

	// edit menu item
	app.Post("/edit-menu-item/:id", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.EditMenuItem(c, db)
	})

	/// create get view for edit menu and menu item return modal htmx view
	app.Get("/edit-menu/:id", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.EditMenuView(c, db)
	})

	/// create get view for edit menu and menu item return modal htmx view
	app.Get("/edit-menu-item/:id", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.EditMenuItemView(c, db)
	})

	app.Get("/search-menu", func(c *fiber.Ctx) error {
		return handlers.SearchMenuAdminTable(c, db)
	})

	app.Get("/get-primary-menu", func(c *fiber.Ctx) error {
		return handlers.GetPrimaryMenuRender(c, db)
	})

	app.Get("/search-users", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.SearchUsers(c, db)
	})

	app.Get("/search-comments", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.SearchCommentsView(c, db)
	})

	/// toggle comment status#
	app.Post("/toggle-comment-status/:id", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.ToggleCommentStatus(c, db)
	})

	/// delete comment
	app.Delete("/delete-comment/:id", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteComment(c, db)
	})

	app.Delete("/delete-user/:id", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteUser(c, db)
	})

	app.Get("/search-categories", func(c *fiber.Ctx) error {
		return handlers.SearchCategories(c, db)
	})

	app.Post("/add-category", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AddCategory(c, db)
	})

	app.Delete("/delete-category", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteCategory(c, db)
	})

	app.Get("/search-custompages", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.SearchCustomPages(c, db)
	})

	app.Post("/add-custompage", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AddCustomPage(c, db, app, engine)
	})

	app.Get("/add-custompage", func(c *fiber.Ctx) error {
		if c.Locals("isAdmin") == false {
			return c.Redirect("/")
		}
		return c.Render("page/page_add", fiber.Map{
			"TitleView": "Add Custom Page",
			"Settings":  c.Locals("Settings"),
		}, "main")
	})

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
			"Title":    customPage.Title,
			"Content":  customPage.Content,
			"ID":       customPage.ID,
			"Slug":     customPage.Slug,
			"Template": customPage.Template,
			"Settings": c.Locals("Settings"),
		}, "main")
	})

	app.Post("/edit-custompage", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.EditCustomPage(c, db)
	})

	app.Delete("/delete-custompage/:id", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteCustomPage(c, db)
	})

	app.Get("/search-files", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {

		return handlers.SearchFiles(c, db)
	})

	app.Post("/add-comment", handlers.IsLoggedIn, func(c *fiber.Ctx) error {
		return handlers.AddComment(c, db)
	})

	app.Post("/upload-file", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.UploadFile(c, db)
	})

	app.Delete("/delete-file", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteFile(c, db)
	})

	app.Get("/blog/:page?", func(c *fiber.Ctx) error {
		return handlers.BlogPage(c, db)
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

	app.Get("/admin/post/edit/:post_id", func(c *fiber.Ctx) error {
		return handlers.AdminEditBlogPost(c, db)
	})

	app.Post("/admin/post/edit", func(c *fiber.Ctx) error {
		return handlers.AdminUpdateBlogPost(c, db)
	})

	app.Get("/admin/post/add", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {

		var categories []model.Category
		var tags []model.Tag

		db.Find(&categories)
		db.Find(&tags)

		c.Set("HX-Trigger", "Action: addPost")

		html_basic_test := "<p>Write your post here</p>"

		return c.Render("admin/post/post_add", fiber.Map{
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

	app.Get("/admin", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {

		plugins := plugin_system.GetPlugins()
		pluginData := make([]map[string]interface{}, 0, len(plugins))
		for _, plugin := range plugins {
			pluginData = append(pluginData, map[string]interface{}{
				"Name":    plugin.Name(),
				"Enabled": plugin.Enabled(db),
				"Author":  plugin.Author(),
				"Version": plugin.Version(),
			})
		}

		var enabled_plugins int64
		db.Model(&model.Plugin{}).Where("enabled = ?", true).Count(&enabled_plugins)

		return c.Render("admin/admin", fiber.Map{
			"Title":          "Admin Panel",
			"IsAdmin":        c.Locals("isAdmin"),
			"IsLoggedIn":     c.Locals("isLoggedin"),
			"Settings":       c.Locals("Settings"),
			"Plugins":        pluginData,
			"EnabledPlugins": enabled_plugins,
		}, "main")
	})

	app.Get("/sitemap.xml", func(c *fiber.Ctx) error {
		return c.SendFile("./static/sitemap.xml")
	})

}
