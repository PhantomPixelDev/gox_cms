package shop_plugin

import (
	"encoding/json"
	"fmt"
	handlers "goxcms/handler"
	"goxcms/model"
	"html/template"
	"regexp"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
	"gorm.io/gorm"
)

type ShopPlugin struct{}

const (
	PluginName = "ShopPlugin"
	Author     = "Ashba22"
	Version    = "1.0"
	Enabled    = false
)

var defaultSettings = map[string]string{
	"Shop Name":        "Shop Name",
	"Shop Description": "Shop Description",
	"Shop Address":     "Shop Address",
	"Shop Phone":       "Shop Phone",
	"Shop Email":       "Shop Email",
}

// / add models here
type Product struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	Name        string `json:"name"`
	Price       uint   `json:"price"`
	Description string `json:"description" gorm:"default:''"`
	Status      string `json:"status" gorm:"default:'pending'"`
}

func (p *ShopPlugin) AddProduct(c *fiber.Ctx, db *gorm.DB) error {
	var product Product

	// Extract product data from the form
	product.Name = sanitizeHTML(c.FormValue("name"))
	price, _ := strconv.Atoi(c.FormValue("price"))
	product.Price = uint(price)

	// Validate the data
	err := db.Create(&product).Error

	if err != nil {
		price := strconv.Itoa(price)
		println("Error creating product", err.Error(), product.Name, price)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid data",
		})
	}

	return c.Status(fiber.StatusCreated).SendString("Product created successfully")
}

// SanitizeHTML sanitizes the HTML input to prevent XSS attacks
func sanitizeHTML(input string) string {
	// Remove any HTML tags and attributes
	sanitized := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(input, "")

	// Replace special characters with their HTML entities
	sanitized = template.HTMLEscapeString(sanitized)

	return sanitized
}

func (p *ShopPlugin) Setup(app *fiber.App, db *gorm.DB, engine *html.Engine) error {
	fmt.Println("ShopPlugin setup")
	// Add routes here
	/// migrate models here

	///

	db.AutoMigrate(&Product{})

	/// add example product if none exists
	products := []Product{}
	productQuery := db.Find(&products)
	if productQuery.Error != nil {
		return productQuery.Error
	}

	if len(products) == 0 {
		db.Create(&Product{Name: "Product 2", Price: 100, Description: "Product 1 description", Status: "pending"})
	}

	/// add routes here

	app.Post("/ShopPlugin/add_product", func(c *fiber.Ctx) error {
		/// if plugin is not enabled return 404
		if !p.Enabled(db) {
			return c.Status(404).SendString("Plugin not enabled")
		}
		return p.AddProduct(c, db)
	})

	/// shop admin routes
	app.Get("/ShopPlugin/admin", handlers.IsLoggedIn, handlers.IsAdmin, func(c *fiber.Ctx) error {

		/// if plugin is not enabled return 404
		if !p.Enabled(db) {
			return c.Status(fiber.StatusNotFound).SendString("Plugin not enabled")
		}

		handlers.ShowToast(c, "Shop Admin")

		products := []Product{}
		if err := db.Find(&products).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Error fetching products")
		}

		// no cache for this page
		c.Set("Cache-Control", "no-store, no-cache, must-revalidate, post-check=0, pre-check=0")

		return c.Render("plugins/shop_plugin/admin", fiber.Map{
			"Title":    "Shop Admin",
			"Products": products,
			"Settings": c.Locals("Settings"),
		}, "main")
	})

	// shop main page route ///
	app.Get("/shop", func(c *fiber.Ctx) error {
		/// if plugin is not enabled return 404
		if !p.Enabled(db) {
			return c.Status(fiber.StatusNotFound).SendString("Plugin not enabled")
		}

		products := []Product{}
		if err := db.Find(&products).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Error fetching products")
		}

		return c.Render("plugins/shop_plugin/shop", fiber.Map{
			"Title":    "Shop",
			"Products": products,
			"Settings": c.Locals("Settings"),
		}, "main")
	})

	/// product page
	app.Get("/product/:id", func(c *fiber.Ctx) error {
		/// if plugin is not enabled return 404
		if !p.Enabled(db) {
			return c.Status(fiber.StatusNotFound).SendString("Plugin not enabled")
		}

		productID, _ := strconv.Atoi(c.Params("id"))
		product := Product{}
		if err := db.First(&product, productID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Product not found")
		}

		htmlMessage := template.HTML("<div class='alert alert-success'>Product</div>")
		htmlMessage += template.HTML("<div class='alert alert-info'>Your product</div>")
		return c.Render("plugins/shop_plugin/product", fiber.Map{
			"Title":    "Product",
			"Product":  product,
			"Settings": c.Locals("Settings"),
		}, "main")
	})

	/// settings page for the plugin
	app.Get("/ShopPlugin/admin/settings", handlers.IsAdmin, handlers.IsLoggedIn, func(c *fiber.Ctx) error {
		/// if plugin is not enabled return 404
		if !p.Enabled(db) {
			return c.Status(fiber.StatusNotFound).SendString("Plugin not enabled")
		}

		plugin_settings := p.Settings(db)

		/// convert string to map for settings
		// settings := make(map[string]string)

		return c.Render("plugins/shop_plugin/settings", fiber.Map{
			"Title":          "Shop Plugin Settings",
			"PluginName":     p.Name(),
			"Settings":       c.Locals("Settings"),
			"PluginSettings": plugin_settings,
		}, "main")

	})

	println("ShopPlugin setup done")
	return nil
}

func (p *ShopPlugin) Teardown() error {
	fmt.Println("ShopPlugin teardown")
	return nil
}

func (p *ShopPlugin) Name() string {
	return PluginName
}

func (p *ShopPlugin) Author() string {
	return Author
}

func (p *ShopPlugin) Version() string {
	return Version
}

func (p *ShopPlugin) DefaultSettings() map[string]string {
	return defaultSettings

}
func (p *ShopPlugin) Settings(db *gorm.DB) map[string]string {
	/// return settings from database!
	//// get settings from database and return them
	plugin := &model.Plugin{}
	db.Where("name = ?", PluginName).First(plugin)
	fmt.Println(PluginName, "settings:", plugin.Settings)
	settings := plugin.Settings
	if len(settings) == 0 {
		return p.DefaultSettings()
	}

	/// convert to json string
	mappedSettings := make(map[string]string)
	json.Unmarshal([]byte(settings), &mappedSettings)

	return mappedSettings
}

func (p *ShopPlugin) Enabled(db *gorm.DB) bool {
	/// get status from database
	plugin := &model.Plugin{}
	db.Where("name = ?", PluginName).First(plugin)
	fmt.Println(PluginName, "enabled status:", plugin.Enabled)
	return plugin.Enabled
}
