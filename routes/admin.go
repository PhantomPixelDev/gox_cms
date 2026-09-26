package routes

import (
	"html/template"

	handlers "goxcms/handler"
	"goxcms/model"
	"goxcms/plugin_system"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
	"gorm.io/gorm"
)

// setupAdminRoutes registers the admin panel and its HTMX endpoints. Every
// route here must be guarded by handlers.IsAdmin.
func setupAdminRoutes(app *fiber.App, db *gorm.DB, engine *html.Engine) {
	app.Post("/clear-cache", handlers.IsAdmin, func(c *fiber.Ctx) error {

		// Reload cached settings and templates. This deliberately does not
		// touch the session store, which also holds every user's CSRF token.
		handlers.ReloadSiteSettings(db)
		if err := engine.Load(); err != nil {
			return handlers.ShowToastError(c, "Failed to reload templates: "+err.Error())
		}

		handlers.ShowToast(c, "Cache cleared successfully")

		return nil
	})

	app.Get("/admin-settings", handlers.IsAdmin, func(c *fiber.Ctx) error {

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

	app.Post("/update-settings", handlers.IsAdmin, func(c *fiber.Ctx) error {

		return handlers.UpdateSettings(c, db)
	})

	app.Post("/toggle-post-status", handlers.IsAdmin, func(c *fiber.Ctx) error {

		return handlers.TogglePostStatus(c, db)
	})

	app.Get("/search-posts", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AdminSearchPosts(c, db)
	})

	app.Delete("/delete-post/:id", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AdminDeletePost(c, db)
	})

	app.Get("/search-tags", handlers.IsAdmin, func(c *fiber.Ctx) error {
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

	// reorder a menu item within its menu
	app.Post("/move-menu-item/:id/:direction", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.MoveMenuItem(c, db)
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

	app.Get("/search-menu", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.SearchMenuAdminTable(c, db)
	})

	app.Get("/search-users", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.SearchUsers(c, db)
	})

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

	app.Get("/search-categories", handlers.IsAdmin, func(c *fiber.Ctx) error {
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

	app.Post("/add-custompage", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AddCustomPage(c, db)
	})

	app.Get("/add-custompage", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return c.Render("page/page_add", fiber.Map{
			"TitleView": "Add Custom Page",
			"Settings":  c.Locals("Settings"),
		}, "main")
	})

	app.Get("/edit-custompage/:id", handlers.IsAdmin, func(c *fiber.Ctx) error {

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

	app.Post("/edit-custompage", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.EditCustomPage(c, db)
	})

	app.Delete("/delete-custompage/:id", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteCustomPage(c, db)
	})

	app.Get("/search-files", handlers.IsAdmin, func(c *fiber.Ctx) error {

		return handlers.SearchFiles(c, db)
	})

	app.Post("/upload-file", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.UploadFile(c, db)
	})

	app.Delete("/delete-file", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.DeleteFile(c, db)
	})

	app.Get("/admin/post/edit/:post_id", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AdminEditBlogPost(c, db)
	})

	app.Post("/admin/post/edit", handlers.IsAdmin, func(c *fiber.Ctx) error {
		return handlers.AdminUpdateBlogPost(c, db)
	})

	app.Get("/admin/post/add", handlers.IsAdmin, func(c *fiber.Ctx) error {

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

	app.Post("/admin/post/add", handlers.IsAdmin, func(c *fiber.Ctx) error {

		return handlers.AdminAddBlogPost(c, db)
	})

	app.Get("/admin", handlers.IsAdmin, func(c *fiber.Ctx) error {

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

		var enabled_plugins, post_count, user_count, pending_comments, page_count, file_count int64
		db.Model(&model.Plugin{}).Where("enabled = ?", true).Count(&enabled_plugins)
		db.Model(&model.Post{}).Count(&post_count)
		db.Model(&model.User{}).Count(&user_count)
		db.Model(&model.Comment{}).Where("status = ?", "pending").Count(&pending_comments)
		db.Model(&model.CustomPage{}).Count(&page_count)
		db.Model(&model.File{}).Count(&file_count)

		return c.Render("admin/admin", fiber.Map{
			"Title":           "Admin Panel",
			"IsAdmin":         c.Locals("isAdmin"),
			"IsLoggedIn":      c.Locals("isLoggedin"),
			"Settings":        c.Locals("Settings"),
			"Plugins":         pluginData,
			"EnabledPlugins":  enabled_plugins,
			"PostCount":       post_count,
			"UserCount":       user_count,
			"PendingComments": pending_comments,
			"PageCount":       page_count,
			"FileCount":       file_count,
		}, "main")
	})
}
