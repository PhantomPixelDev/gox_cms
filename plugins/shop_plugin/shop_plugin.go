package shop_plugin

import (
	"encoding/json"
	handlers "goxcms/handler"
	"goxcms/model"
	"log"
	"math/rand"
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
	"shop_name":        "Shop Name",
	"shop_description": "Shop Description",
	"shop_address":     "Shop Address",
	"shop_phone":       "Shop Phone",
	"shop_email":       "Shop Email",
}

type Product struct {
	ID                uint            `json:"id" gorm:"primaryKey"`
	Name              string          `json:"name"`
	Slug              string          `json:"slug" gorm:"default:''"`
	Price             uint            `json:"price"`
	Description       string          `json:"description" gorm:"default:''"`
	Picture           string          `json:"picture" gorm:"default:''"`
	MorePictures      string          `json:"more_pictures" gorm:"default:''"`
	Status            string          `json:"status" gorm:"default:'pending'"`
	ProductCategory   ProductCategory `json:"product_category"`
	ProductCategoryID uint            `json:"product_category_id"`
}

type ProductCategory struct {
	ID            uint   `json:"id" gorm:"primaryKey"`
	Name          string `json:"name"`
	SubCategories string `json:"sub_categories" gorm:"default:''"`
}

func (p *ShopPlugin) AddProduct(c *fiber.Ctx, db *gorm.DB) error {
	var product Product

	// Extract product data from the form
	product.Name = handlers.SanitizeText(c.FormValue("name"))
	price, _ := strconv.Atoi(c.FormValue("price"))
	product.Description = handlers.SanitizeText(c.FormValue("description"))
	product.Picture = c.FormValue("picture")
	product.MorePictures = c.FormValue("more_pictures")
	product.Price = uint(price)

	// Validate the data
	err := db.Create(&product).Error

	if err != nil {
		log.Printf("shop: creating product %q: %v", product.Name, err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid data",
		})
	}

	return c.Status(fiber.StatusCreated).SendString("Product created successfully")
}

var nonSlugChars = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func generateSlugFromProductName(productName string) string {
	return nonSlugChars.ReplaceAllString(productName, "-")
}

// demoProductCount is how many placeholder products are seeded the first
// time the shop plugin starts with an empty catalogue.
const demoProductCount = 30

func generateRandomProducts(db *gorm.DB) error {
	products := make([]Product, 0, demoProductCount)
	for i := 1; i <= demoProductCount; i++ {
		name := "Product " + strconv.Itoa(i)
		products = append(products, Product{
			Name:              name,
			Price:             uint(rand.Intn(100)),
			Description:       name + " description",
			Status:            "pending",
			Slug:              generateSlugFromProductName(name),
			ProductCategoryID: 1,
			Picture:           "https://placehold.co/600x400/EEE/31343C",
			MorePictures:      "https://placehold.co/600x400/EEE/31343C",
		})
	}

	return db.CreateInBatches(products, 100).Error
}

func (p *ShopPlugin) Setup(app *fiber.App, db *gorm.DB, engine *html.Engine) error {

	db.AutoMigrate(&Product{})
	db.AutoMigrate(&ProductCategory{})

	/// check settings if they are empty and add default settings
	plugin := &model.Plugin{}
	db.Where("name = ?", PluginName).First(plugin)
	settings := plugin.Settings

	if settings == "" {
		defaultSettingsJSON, err := json.Marshal(p.DefaultSettings())
		if err != nil {
			log.Printf("shop: marshaling default settings: %v", err)
			return err
		}

		plugin.Settings = string(defaultSettingsJSON)
		db.Save(&plugin)
	}

	// Check if product categories exist, if not, add an example category
	var productCategories []ProductCategory
	if err := db.Find(&productCategories).Error; err != nil {
		return err
	}

	if len(productCategories) == 0 {
		db.Create(&ProductCategory{Name: "Category 1"})
	}

	// Check if products exist, if not, generate random products
	var count int64
	if err := db.Model(&Product{}).Count(&count).Error; err != nil {
		return err
	}

	if count == 0 {
		if err := generateRandomProducts(db); err != nil {
			return err
		}
	}

	app.Post("/ShopPlugin/add_product", handlers.IsAdmin, func(c *fiber.Ctx) error {
		if !p.Enabled(db) {
			return c.Status(404).SendString("Plugin not enabled")
		}
		return p.AddProduct(c, db)
	})

	app.Get("/ShopPlugin/admin/:page?", handlers.IsAdmin, func(c *fiber.Ctx) error {
		if !p.Enabled(db) {
			return c.Status(fiber.StatusNotFound).SendString("Plugin not enabled")
		}

		pluginSettings := p.Settings(db)
		// Pagination parameters
		limit := 10
		page := c.Params("page")
		pageInt, err := strconv.Atoi(page)
		if err != nil {
			pageInt = 1
		}
		offset := (pageInt - 1) * limit

		// Search query
		searchQuery := c.Query("search_query")

		// Fetch products based on search query and pagination
		var products []Product
		var totalProducts int64
		query := db.Model(&Product{})
		if searchQuery != "" {
			query = query.Where("name LIKE ?", "%"+searchQuery+"%")
		}
		query.Count(&totalProducts)
		totalPages := int(totalProducts / int64(limit))
		if totalPages == 0 {
			totalPages = 1
		}
		query.Limit(limit).Offset(offset).Find(&products)

		c.Set("Cache-Control", "no-store, no-cache, must-revalidate, post-check=0, pre-check=0")

		return c.Render("plugins/shop_plugin/admin", fiber.Map{
			"Title":          "Shop Admin",
			"Products":       products,
			"Settings":       c.Locals("Settings"),
			"TotalPages":     totalPages,
			"CurrentPage":    pageInt,
			"SearchQuery":    searchQuery,
			"PluginSettings": pluginSettings,
		}, "main")
	})

	/// /ShopPlugin/update-settings endpoint
	app.Post("/ShopPlugin/update-settings", handlers.IsAdmin, func(c *fiber.Ctx) error {
		if !p.Enabled(db) {
			return c.Status(fiber.StatusNotFound).SendString("Plugin not enabled")
		}
		/// print the form values to debug

		shopName := c.FormValue("shop_name")
		shopDescription := c.FormValue("shop_description")
		shopAddress := c.FormValue("shop_address")
		shopPhone := c.FormValue("shop_phone")
		shopEmail := c.FormValue("shop_email")

		settings := map[string]string{
			"shop_name":        shopName,
			"shop_description": shopDescription,
			"shop_address":     shopAddress,
			"shop_phone":       shopPhone,
			"shop_email":       shopEmail,
		}

		settingsJSON, err := json.Marshal(settings)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Error updating settings")
		}

		plugin := &model.Plugin{}
		db.Where("name = ?", PluginName).First(plugin)
		plugin.Settings = string(settingsJSON)
		db.Save(&plugin)

		return c.Redirect("/ShopPlugin/admin")
	})

	app.Get("/shop/:page?/:search_query?", func(c *fiber.Ctx) error {
		if !p.Enabled(db) {
			return c.Status(fiber.StatusNotFound).SendString("Plugin not enabled")
		}

		limit := 10
		page := c.Params("page")
		pageInt, err := strconv.Atoi(page)
		if err != nil {
			pageInt = 1
		}

		var totalProducts int64
		searchQuery := c.Params("search_query")

		if searchQuery == "" {
			db.Model(&Product{}).Count(&totalProducts)
		} else {
			/// convert to string and remove any special characters
			searchQuery = handlers.SanitizeText(searchQuery)
			print("Search query: ", searchQuery)
			db.Model(&Product{}).Where("name LIKE ?", "%"+searchQuery+"%").Count(&totalProducts)
		}

		totalPages := int(totalProducts / int64(limit))
		if totalPages == 0 {
			totalPages = 1
		}

		return c.Render("plugins/shop_plugin/shop", fiber.Map{
			"Title":       "Shop",
			"Settings":    c.Locals("Settings"),
			"TotalPages":  totalPages,
			"CurrentPage": pageInt,
			"SearchQuery": searchQuery,
		}, "main")
	})

	/// /search-products endpoint
	app.Get("/search-products/:page?/:search_query?", func(c *fiber.Ctx) error {
		if !p.Enabled(db) {
			return c.Status(fiber.StatusNotFound).SendString("Plugin not enabled")
		}
		searchQuery := c.Params("search_query")
		pageStr := c.Params("page")

		if pageStr == "" {
			pageStr = "1"
		}
		page, err := strconv.Atoi(pageStr)
		if err != nil || page < 1 {
			page = 1
			searchQuery = ""
		}

		limit := 10
		offset := (page - 1) * limit
		var products []Product
		var totalProducts int64
		query := db.Model(&Product{})
		if searchQuery != "" {
			query = query.Where("name LIKE ?", "%"+searchQuery+"%")
		}
		query.Count(&totalProducts)
		totalPages := int(totalProducts / int64(limit))
		if totalPages == 0 {
			totalPages = 1
		}
		query.Limit(limit).Offset(offset).Find(&products)
		return c.Render("plugins/shop_plugin/products_grid", fiber.Map{
			"Title":       "Shop",
			"Products":    products,
			"TotalPages":  totalPages,
			"CurrentPage": page,
			"SearchQuery": searchQuery,
			"Settings":    c.Locals("Settings"),
		})
	})

	/// addd search-products-json endpoint
	app.Get("/search-products-json/:search_query?", func(c *fiber.Ctx) error {
		if !p.Enabled(db) {
			return c.Status(fiber.StatusNotFound).SendString("Plugin not enabled")
		}
		searchQuery := c.Params("search_query")

		var products []Product
		query := db.Model(&Product{})
		if searchQuery != "" {
			query = query.Where("name LIKE ?", "%"+searchQuery+"%")
		}
		query.Find(&products)
		return c.JSON(products)
	})

	app.Get("/product/:id", func(c *fiber.Ctx) error {
		if !p.Enabled(db) {
			return c.Status(fiber.StatusNotFound).SendString("Plugin not enabled")
		}

		productID, _ := strconv.Atoi(c.Params("id"))
		product := Product{}
		if err := db.First(&product, productID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Product not found")
		}

		return c.Render("plugins/shop_plugin/product", fiber.Map{
			"Title":    "Product",
			"Product":  product,
			"Settings": c.Locals("Settings"),
		}, "main")
	})

	return nil
}

func (p *ShopPlugin) Teardown() error {
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
	plugin := &model.Plugin{}
	db.Where("name = ?", PluginName).First(plugin)

	settings := plugin.Settings

	if len(settings) == 0 {
		defaultSettingsJSON, err := json.Marshal(p.DefaultSettings())
		if err != nil {
			log.Printf("shop: marshaling default settings: %v", err)
			return p.DefaultSettings()
		}

		plugin.Settings = string(defaultSettingsJSON)
		db.Save(&plugin)

		return p.DefaultSettings()
	}

	mappedSettings := make(map[string]string)
	err := json.Unmarshal([]byte(settings), &mappedSettings)
	if err != nil {
		log.Printf("shop: unmarshaling settings: %v", err)
		return p.DefaultSettings()
	}

	return mappedSettings
}

func (p *ShopPlugin) Enabled(db *gorm.DB) bool {
	plugin := &model.Plugin{}
	db.Where("name = ?", PluginName).First(plugin)
	return plugin.Enabled
}
