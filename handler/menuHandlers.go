package handlers

import (
	"fmt"
	"goxcms/model"
	"math"
	"sort"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func AddMenu(c *fiber.Ctx, db *gorm.DB) error {
	title := c.FormValue("menu_title")
	primary := c.FormValue("menu_primary") == "on"
	parentIDStr := c.FormValue("parent_id")
	positionStr := c.FormValue("menu_position") // New: Get position value from form

	var parentID *uint
	if parentIDStr != "" {
		parentIDInt, err := strconv.Atoi(parentIDStr)
		if err != nil {
			return err
		}
		parentIDUint := uint(parentIDInt)
		parentID = &parentIDUint
	}

	position, posErr := strconv.Atoi(positionStr)
	if posErr != nil {
		return posErr // Handle error appropriately
	}

	menu := model.Menu{
		Title:    title,
		Primary:  primary,
		ParentID: parentID,
		Position: position,
	}

	var existingMenu model.Menu
	if err := db.Where("title = ?", menu.Title).First(&existingMenu).Error; err == nil {
		ShowToast(c, "Menu with title "+menu.Title+" already exists")
		return nil
	}

	// Change other primary menus to non-primary
	if menu.Primary {
		db.Model(&model.Menu{}).Where("primary = ?", true).Update("primary", false)
	}

	if err := db.Create(&menu).Error; err != nil {
		return err
	}

	ShowToast(c, "Menu added successfully")

	return nil
}

func AddMenuItem(c *fiber.Ctx, db *gorm.DB) error {
	title := c.FormValue("menu_item_title")
	link := c.FormValue("menu_item_link")
	menuIDStr := c.FormValue("menu_item_menu")
	positionStr := c.FormValue("item_position") // New: Get position value from form

	menuID, err := strconv.Atoi(menuIDStr)
	if err != nil {
		return err
	}

	// New: Parse position value
	position, posErr := strconv.Atoi(positionStr)
	if posErr != nil {
		return posErr // Handle error appropriately
	}

	menuIDUint := uint(menuID)
	var menuItem model.MenuItem
	menuItem.Title = title
	menuItem.Link = link
	menuItem.MenuID = &menuIDUint
	menuItem.Position = position // New: Set position

	if err := db.Create(&menuItem).Error; err != nil {
		return err
	}

	ShowToast(c, "Menu item added successfully")
	return nil
}

func DeleteMenu(c *fiber.Ctx, db *gorm.DB) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return err
	}

	if err := db.Where("id = ?", id).Delete(&model.Menu{}).Error; err != nil {
		return err
	}

	ShowToast(c, "Menu and associated menu items deleted successfully")
	return nil
}

func DeleteMenuItem(c *fiber.Ctx, db *gorm.DB) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return err
	}

	if err := db.Where("id = ?", id).Delete(&model.MenuItem{}).Error; err != nil {
		return err
	}

	ShowToast(c, "Menu item deleted successfully")
	return nil
}

func RemoveSubmenuFromMenu(c *fiber.Ctx, db *gorm.DB) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return err
	}

	if err := db.Model(&model.Menu{}).Where("id = ?", id).Update("parent_id", nil).Error; err != nil {
		return err
	}

	ShowToast(c, "Submenu removed from menu successfully")
	return nil
}

func EditMenu(c *fiber.Ctx, db *gorm.DB) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return err
	}

	var menu model.Menu
	if err := db.First(&menu, id).Error; err != nil {
		return err
	}

	menu.Title = c.FormValue("menu_title")
	menu.Primary = c.FormValue("menu_primary") == "on"
	position, posErr := strconv.Atoi(c.FormValue("menu_position"))
	if posErr != nil {
		return posErr
	}
	menu.Position = position

	optionel_parent_menu_id, err := strconv.Atoi(c.FormValue("parent_id"))
	if err != nil {
		/// set parent id to nil or 0
		menu.ParentID = nil
	}
	if optionel_parent_menu_id != 0 {
		parentID := uint(optionel_parent_menu_id)
		menu.ParentID = &parentID
	}

	/// change other primary menus to non-primary
	if menu.Primary {
		db.Model(&model.Menu{}).Where("primary = ?", true).Update("primary", false)
	}

	if err := db.Save(&menu).Error; err != nil {
		return err
	}

	ShowToast(c, "Menu edited successfully")
	return nil
}

func EditMenuItem(c *fiber.Ctx, db *gorm.DB) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return err
	}

	var menuItem model.MenuItem
	if err := db.First(&menuItem, id).Error; err != nil {
		return err
	}

	menuItem.Title = c.FormValue("menu_item_title")
	menuItem.Link = c.FormValue("menu_item_link")
	menuID := c.FormValue("menu_item_menu")
	menuIDUint, err := strconv.Atoi(menuID)
	if err != nil {
		return err
	}
	menuIDUintConverted := uint(menuIDUint)
	menuItem.MenuID = &menuIDUintConverted

	position, posErr := strconv.Atoi(c.FormValue("item_position"))
	if posErr != nil {
		return ShowToastError(c, "Failed to update menu item: "+posErr.Error())
	}
	menuItem.Position = position

	if err := db.Save(&menuItem).Error; err != nil {
		return ShowToastError(c, "Failed to update menu item: "+err.Error())
	}

	ShowToast(c, "Menu item updated successfully")
	return nil
}

func EditMenuView(c *fiber.Ctx, db *gorm.DB) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return err
	}

	var menu model.Menu
	if err := db.First(&menu, id).Error; err != nil {
		return err
	}

	// Get all menus
	var menus []model.Menu
	if err := db.Find(&menus).Error; err != nil {
		return err
	}
	menuID := uint(menu.ID)

	return c.Render("admin/menu/edit-menu", fiber.Map{
		"Menu":   menu,
		"MenuID": menuID,
		"Menus":  menus,
	})
}

func EditMenuItemView(c *fiber.Ctx, db *gorm.DB) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return err
	}

	var menuItem model.MenuItem
	if err := db.First(&menuItem, id).Error; err != nil {
		return err
	}

	// get all menus for the select dropdown
	var menus []model.Menu
	if err := db.Find(&menus).Error; err != nil {
		return err
	}

	// Convert ID to the same type as MenuID
	menuItemID := uint(menuItem.ID)

	return c.Render("admin/menu/edit-menu-item", fiber.Map{
		"MenuItem":   menuItem,
		"Menus":      menus,
		"MenuItemID": menuItemID,
	})
}

func SearchMenuAdminTable(c *fiber.Ctx, db *gorm.DB) error {
	var menus []model.Menu
	searchQuery := c.Query("query")
	page := c.Query("page", "1")
	pageSize := 10 // Default page size

	// Convert page string to int for pagination calculation
	pageInt, err := strconv.Atoi(page)
	if err != nil || pageInt < 1 {
		pageInt = 1
	}

	// Search for menus with pagination and order them by position
	// Ensure to order both menus and their items by their position
	db.Where("title LIKE ?", "%"+searchQuery+"%").
		Order("position ASC"). // Order menus by position
		Preload("MenuItems", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC") // Order menu items by position within each menu
		}).
		Preload("SubMenus", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC") // Order sub-menus by position
		}).
		Limit(pageSize).
		Offset((pageInt - 1) * pageSize).
		Find(&menus)

	// Count total menus that match the search query for pagination
	var totalMatchingCount int64
	db.Model(&model.Menu{}).
		Where("title LIKE ?", "%"+searchQuery+"%").
		Count(&totalMatchingCount)
	totalPages := int(math.Ceil(float64(totalMatchingCount) / float64(pageSize)))

	return c.Render("admin/admin-menu-table", fiber.Map{
		"Menus":       menus, // No need to separate and recombine by primary status for ordering
		"TotalPages":  totalPages,
		"CurrentPage": pageInt,
		"SearchQuery": searchQuery,
	})
}

func GetPrimaryMenuRender(c *fiber.Ctx, db *gorm.DB) error {

	var menu model.Menu
	// Attempt to preload MenuItems and directly associated SubMenus
	if err := db.Where("is_primary = ?", true).Order("position ASC").Preload("MenuItems").First(&menu).Error; err != nil {
		// Set menu to default value if no menu is found
		menu = model.Menu{ID: 1}
	}

	// Manually load and order SubMenus if necessary
	if err := db.Where("parent_id = ?", menu.ID).Order("position ASC").Preload("MenuItems").Find(&menu.SubMenus).Error; err != nil {
		menu = model.Menu{ID: 1}
	}

	userLoggedIn, ok := c.Locals("isLoggedin").(bool)
	if !ok {
		userLoggedIn = false
	}

	isAdmin, ok := c.Locals("isAdmin").(bool)
	if !ok {
		isAdmin = false
	}

	htmlMenuString := buildMenuHTML(menu, isAdmin, userLoggedIn)

	return c.SendString(htmlMenuString)
}

// Separating the HTML building into its own function for clarity
func buildMenuHTML(menu model.Menu, isAdmin bool, userLoggedIn bool) string {
	htmlMenuString := "<div class=\"collapse navbar-collapse\" id=\"navbarNavDropdown\">\n"
	htmlMenuString += "\t<ul class=\"navbar-nav me-auto\">\n"

	// Sort MenuItems and SubMenus together based on position
	sort.SliceStable(menu.MenuItems, func(i, j int) bool {
		return menu.MenuItems[i].Position < menu.MenuItems[j].Position
	})

	// Loop through top-level MenuItems
	for _, menuItem := range menu.MenuItems {
		htmlMenuString += fmt.Sprintf("\t\t<li class=\"nav-item\"><a class=\"nav-link\" href=\"%s\">%s</a></li>\n", menuItem.Link, menuItem.Title)
	}

	// Render SubMenus if available
	for _, subMenu := range menu.SubMenus {
		htmlMenuString += "\t\t<li class=\"nav-item dropdown\">\n"
		htmlMenuString += "\t\t\t<a class=\"nav-link dropdown-toggle\" href=\"#\" id=\"navbarDropdownMenuLink-" + strconv.Itoa(int(subMenu.ID)) + "\" role=\"button\" data-bs-toggle=\"dropdown\" aria-expanded=\"false\">" + subMenu.Title + "</a>\n"
		htmlMenuString += "\t\t\t<ul class=\"dropdown-menu\" aria-labelledby=\"navbarDropdownMenuLink\">\n"
		for _, subMenuItem := range subMenu.MenuItems {
			htmlMenuString += "\t\t\t\t<li><a class=\"dropdown-item\" href=\"" + subMenuItem.Link + "\">" + subMenuItem.Title + "</a></li>\n"
		}
		htmlMenuString += "\t\t\t</ul>\n"
		htmlMenuString += "\t\t</li>\n"
	}

	htmlMenuString += "\t</ul>\n"

	// Admin and user controls
	if isAdmin {
		htmlMenuString += adminControls()
	}

	if userLoggedIn {
		htmlMenuString += userControls(true)
	} else {
		htmlMenuString += userControls(false)
	}

	htmlMenuString += "</div></nav>"

	return htmlMenuString
}
func adminControls() string {
	return `<div class="btn-group" role="group" aria-label="Admin group">
		<a href="/admin" class="btn btn-sm btn-outline-primary px-4 my-2">Admin Dashboard</a>
		<a href="/clear-cache" class="btn btn-sm btn-outline-primary px-4 my-2" hx-post="/clear-cache" hx-trigger="click" hx-confirm="Are you sure you want to clear the cache?" hx-swap="none" hx-headers='{"X-No-Cache": "true"}'>Clear Cache</a>
		</div>`
}

func userControls(loggedIn bool) string {
	if loggedIn {
		return `<ul class="navbar-nav ms-auto">
			<li class="nav-item"><button hx-post="/logout" hx-swap="none" hx-target="body" hx-headers='{"X-No-Cache": "true"}' class="btn btn-link nav-link" style="text-decoration: none; color: inherit;">Logout</button></li>
			</ul>`
	}
	return `<ul class="navbar-nav ms-auto">
		<li class="nav-item"><a class="nav-link" href="/login">Login</a></li>
		<li class="nav-item"><a class="nav-link" href="/register">Register</a></li>
		</ul>`
}
