package shop_plugin

import (
	"encoding/json"
	"fmt"
	"goxcms/model"
	"html/template"
	"math/rand"
	"regexp"
	"strconv"
	"time"

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
	product.Name = sanitizeHTML(c.FormValue("name"))
	price, _ := strconv.Atoi(c.FormValue("price"))
	product.Description = sanitizeHTML(c.FormValue("description"))
	product.Picture = c.FormValue("picture")
	product.MorePictures = c.FormValue("more_pictures")
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

func sanitizeHTML(input string) string {
	// Remove any HTML tags and attributes
	sanitized := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(input, "")

	// Replace special characters with their HTML entities
	sanitized = template.HTMLEscapeString(sanitized)

	return sanitized
}

func generateSlugFromProductName(productName string) string {
	return regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(productName, "-")
}

func generateRandomProducts(db *gorm.DB) error {
	rand.Seed(time.Now().UnixNano())

	for i := 0; i < 30000; i++ {
		println("Generating product ", i+1)
		product := Product{
			Name:              "Product " + strconv.Itoa(i+1),
			Price:             uint(rand.Float64() * 100),
			Description:       "Product " + strconv.Itoa(i+1) + " description",
			Status:            "pending",
			ProductCategory:   ProductCategory{ID: 1},
			Slug:              generateSlugFromProductName("Product " + strconv.Itoa(i+1)),
			ProductCategoryID: 1,
			Picture:           "https://placehold.co/600x400/EEE/31343C",
			MorePictures:      "https://placehold.co/600x400/EEE/31343C",
		}
		db.Create(&product)
	}

	return nil
}

func (p *ShopPlugin) Setup(app *fiber.App, db *gorm.DB, engine *html.Engine) error {
	fmt.Println("ShopPlugin setup")

	db.AutoMigrate(&Product{})
	db.AutoMigrate(&ProductCategory{})

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

	app.Post("/ShopPlugin/add_product", func(c *fiber.Ctx) error {
		if !p.Enabled(db) {
			return c.Status(404).SendString("Plugin not enabled")
		}
		return p.AddProduct(c, db)
	})

	app.Get("/ShopPlugin/admin", func(c *fiber.Ctx) error {
		if !p.Enabled(db) {
			return c.Status(fiber.StatusNotFound).SendString("Plugin not enabled")
		}

		products := []Product{}
		if err := db.Find(&products).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Error fetching products")
		}

		c.Set("Cache-Control", "no-store, no-cache, must-revalidate, post-check=0, pre-check=0")

		return c.Render("plugins/shop_plugin/admin", fiber.Map{
			"Title":    "Shop Admin",
			"Products": products,
			"Settings": c.Locals("Settings"),
		}, "main")
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
			println("Total products: ", totalProducts)
		} else {
			/// convert to string and remove any special characters
			searchQuery = sanitizeHTML(searchQuery)
			println("Search query: ", searchQuery)
			print("Search query: ", searchQuery)
			db.Model(&Product{}).Where("name LIKE ?", "%"+searchQuery+"%").Count(&totalProducts)
		}

		println("Search query: ", searchQuery, " Page: ", pageInt)

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

		htmlMessage := template.HTML("<div class='alert alert-success'>Product</div>")
		htmlMessage += template.HTML("<div class='alert alert-info'>Your product</div>")
		return c.Render("plugins/shop_plugin/product", fiber.Map{
			"Title":    "Product",
			"Product":  product,
			"Settings": c.Locals("Settings"),
		}, "main")
	})

	app.Get("/ShopPlugin/admin/settings", func(c *fiber.Ctx) error {
		if !p.Enabled(db) {
			return c.Status(fiber.StatusNotFound).SendString("Plugin not enabled")
		}

		pluginSettings := p.Settings(db)

		return c.Render("plugins/shop_plugin/settings", fiber.Map{
			"Title":          "Shop Plugin Settings",
			"PluginName":     p.Name(),
			"Settings":       c.Locals("Settings"),
			"PluginSettings": pluginSettings,
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
	plugin := &model.Plugin{}
	db.Where("name = ?", PluginName).First(plugin)
	fmt.Println(PluginName, "settings:", plugin.Settings)
	settings := plugin.Settings
	if len(settings) == 0 {
		return p.DefaultSettings()
	}

	mappedSettings := make(map[string]string)
	json.Unmarshal([]byte(settings), &mappedSettings)

	return mappedSettings
}

func (p *ShopPlugin) Enabled(db *gorm.DB) bool {
	plugin := &model.Plugin{}
	db.Where("name = ?", PluginName).First(plugin)
	fmt.Println(PluginName, "enabled status:", plugin.Enabled)
	return plugin.Enabled
}
