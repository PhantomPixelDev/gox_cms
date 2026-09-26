package handlers

import (
	"fmt"
	"goxcms/model"
	htmlstd "html"
	"sort"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func AddMenu(c *fiber.Ctx, db *gorm.DB) error {
	title := strings.TrimSpace(c.FormValue("menu_title"))
	if title == "" {
		ShowToastError(c, "Menu title is required")
		return c.Status(fiber.StatusBadRequest).SendString("Menu title is required")
	}
	primary := c.FormValue("menu_primary") == "on"
	parentIDStr := c.FormValue("parent_id")

	var parentID *uint
	if parentIDStr != "" {
		parentIDInt, err := strconv.Atoi(parentIDStr)
		if err != nil {
			ShowToastError(c, "Invalid parent menu")
			return c.Status(fiber.StatusBadRequest).SendString("Invalid parent menu")
		}
		parentIDUint := uint(parentIDInt)
		parentID = &parentIDUint
	}

	menu := model.Menu{
		Title:    title,
		Primary:  primary,
		ParentID: parentID,
		Position: optionalPosition(c.FormValue("menu_position"), db, nil),
	}

	var existingMenu model.Menu
	if err := db.Where("title = ?", menu.Title).First(&existingMenu).Error; err == nil {
		ShowToast(c, "Menu with title "+menu.Title+" already exists")
		return nil
	}

	// Change other primary menus to non-primary, atomically with the create.
	// NOTE: the column is is_primary (see model.Menu), not primary.
	err := db.Transaction(func(tx *gorm.DB) error {
		if menu.Primary {
			if err := tx.Model(&model.Menu{}).Where("is_primary = ?", true).Update("is_primary", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(&menu).Error
	})
	if err != nil {
		if isDupKeyError(err) {
			ShowToast(c, "Menu with title "+menu.Title+" already exists")
			return nil
		}
		ShowToastError(c, "Could not create menu")
		return c.Status(fiber.StatusInternalServerError).SendString("Could not create menu")
	}

	ShowToast(c, "Menu added successfully")

	return nil
}

func AddMenuItem(c *fiber.Ctx, db *gorm.DB) error {
	title := strings.TrimSpace(c.FormValue("menu_item_title"))
	link := strings.TrimSpace(c.FormValue("menu_item_link"))
	menuIDStr := c.FormValue("menu_item_menu")

	if title == "" || link == "" {
		ShowToastError(c, "Item title and link are required")
		return c.Status(fiber.StatusBadRequest).SendString("Item title and link are required")
	}

	menuID, err := strconv.Atoi(menuIDStr)
	if err != nil || menuID <= 0 {
		ShowToastError(c, "Pick a menu for the item")
		return c.Status(fiber.StatusBadRequest).SendString("Pick a menu for the item")
	}

	menuIDUint := uint(menuID)
	var menu model.Menu
	if err := db.First(&menu, menuIDUint).Error; err != nil {
		ShowToastError(c, "Menu not found")
		return c.Status(fiber.StatusNotFound).SendString("Menu not found")
	}

	menuItem := model.MenuItem{
		Title:    title,
		Link:     link,
		MenuID:   &menuIDUint,
		Position: optionalPosition(c.FormValue("item_position"), db, &menuIDUint),
	}

	if err := db.Create(&menuItem).Error; err != nil {
		ShowToastError(c, "Could not create menu item")
		return c.Status(fiber.StatusInternalServerError).SendString("Could not create menu item")
	}

	ShowToast(c, "Menu item added successfully")
	return nil
}

// MoveMenuItem swaps an item with its neighbour inside the same menu, so
// ordering never needs manual position numbers.
func MoveMenuItem(c *fiber.Ctx, db *gorm.DB) error {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid ID")
	}
	up := c.Params("direction") == "up"

	var item model.MenuItem
	if err := db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).SendString("Menu item not found")
	}

	var neighbour model.MenuItem
	q := db.Where("menu_id = ?", item.MenuID)
	if up {
		q = q.Where("position < ?", item.Position).Order("position DESC")
	} else {
		q = q.Where("position > ?", item.Position).Order("position ASC")
	}
	if err := q.First(&neighbour).Error; err != nil {
		ShowToast(c, "Already at the edge")
		return c.SendString("Already at the edge")
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.MenuItem{}).Where("id = ?", item.ID).Update("position", neighbour.Position).Error; err != nil {
			return err
		}
		return tx.Model(&model.MenuItem{}).Where("id = ?", neighbour.ID).Update("position", item.Position).Error
	})
	if err != nil {
		ShowToastError(c, "Could not reorder items")
		return c.Status(fiber.StatusInternalServerError).SendString("Could not reorder items")
	}

	ShowToast(c, "Menu item moved")
	return SearchMenuAdminTable(c, db)
}

// menuParamsID parses a numeric :id param, answering 400 on garbage.
func menuParamsID(c *fiber.Ctx) (int, error) {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		ShowToastError(c, "Invalid ID")
		return 0, c.Status(fiber.StatusBadRequest).SendString("Invalid ID")
	}
	return id, nil
}

func menuNotFound(c *fiber.Ctx, what string) error {
	msg := what + " not found"
	ShowToastError(c, msg)
	return c.Status(fiber.StatusNotFound).SendString(msg)
}

func menuServerError(c *fiber.Ctx, what string) error {
	ShowToastError(c, what)
	return c.Status(fiber.StatusInternalServerError).SendString(what)
}

func DeleteMenu(c *fiber.Ctx, db *gorm.DB) error {
	id, err := menuParamsID(c)
	if err != nil {
		return err
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		var menu model.Menu
		if err := tx.First(&menu, id).Error; err != nil {
			return err
		}
		// Detach submenus instead of orphaning them.
		if err := tx.Model(&model.Menu{}).Where("parent_id = ?", id).Update("parent_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Where("menu_id = ?", id).Delete(&model.MenuItem{}).Error; err != nil {
			return err
		}
		return tx.Delete(&menu).Error
	})
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return menuNotFound(c, "Menu")
		}
		return menuServerError(c, "Could not delete menu")
	}

	ShowToast(c, "Menu and associated menu items deleted successfully")
	return nil
}

func DeleteMenuItem(c *fiber.Ctx, db *gorm.DB) error {
	id, err := menuParamsID(c)
	if err != nil {
		return err
	}

	if err := db.Where("id = ?", id).Delete(&model.MenuItem{}).Error; err != nil {
		return menuServerError(c, "Could not delete menu item")
	}

	ShowToast(c, "Menu item deleted successfully")
	return nil
}

func RemoveSubmenuFromMenu(c *fiber.Ctx, db *gorm.DB) error {
	id, err := menuParamsID(c)
	if err != nil {
		return err
	}

	if err := db.Model(&model.Menu{}).Where("id = ?", id).Update("parent_id", nil).Error; err != nil {
		return menuServerError(c, "Could not detach submenu")
	}

	ShowToast(c, "Submenu removed from menu successfully")
	return nil
}

func EditMenu(c *fiber.Ctx, db *gorm.DB) error {
	id, err := menuParamsID(c)
	if err != nil {
		return err
	}

	var menu model.Menu
	if err := db.First(&menu, id).Error; err != nil {
		return menuNotFound(c, "Menu")
	}

	menu.Title = strings.TrimSpace(c.FormValue("menu_title"))
	if menu.Title == "" {
		ShowToastError(c, "Menu title is required")
		return c.Status(fiber.StatusBadRequest).SendString("Menu title is required")
	}
	menu.Primary = c.FormValue("menu_primary") == "on"
	menu.Position = optionalPosition(c.FormValue("menu_position"), db, nil)

	optionel_parent_menu_id, err := strconv.Atoi(c.FormValue("parent_id"))
	if err != nil {
		/// set parent id to nil or 0
		menu.ParentID = nil
	}
	if optionel_parent_menu_id != 0 {
		parentID := uint(optionel_parent_menu_id)
		menu.ParentID = &parentID
	}

	/// change other primary menus to non-primary, atomically with the save.
	// NOTE: the column is is_primary (see model.Menu), not primary.
	if err := db.Transaction(func(tx *gorm.DB) error {
		if menu.Primary {
			if err := tx.Model(&model.Menu{}).Where("is_primary = ?", true).Update("is_primary", false).Error; err != nil {
				return err
			}
			menu.Primary = true
		}
		return tx.Save(&menu).Error
	}); err != nil {
		return menuServerError(c, "Could not update menu")
	}

	ShowToast(c, "Menu edited successfully")
	return nil
}

func EditMenuItem(c *fiber.Ctx, db *gorm.DB) error {
	id, err := menuParamsID(c)
	if err != nil {
		return err
	}

	var menuItem model.MenuItem
	if err := db.First(&menuItem, id).Error; err != nil {
		return menuNotFound(c, "Menu item")
	}

	menuItem.Title = strings.TrimSpace(c.FormValue("menu_item_title"))
	menuItem.Link = strings.TrimSpace(c.FormValue("menu_item_link"))
	if menuItem.Title == "" || menuItem.Link == "" {
		ShowToastError(c, "Item title and link are required")
		return c.Status(fiber.StatusBadRequest).SendString("Item title and link are required")
	}
	menuIDUint, err := strconv.Atoi(c.FormValue("menu_item_menu"))
	if err != nil || menuIDUint <= 0 {
		ShowToastError(c, "Pick a menu for the item")
		return c.Status(fiber.StatusBadRequest).SendString("Pick a menu for the item")
	}
	menuIDUintConverted := uint(menuIDUint)
	menuItem.MenuID = &menuIDUintConverted

	menuItem.Position = optionalPosition(c.FormValue("item_position"), db, &menuIDUintConverted)

	if err := db.Save(&menuItem).Error; err != nil {
		return menuServerError(c, "Could not update menu item")
	}

	ShowToast(c, "Menu item updated successfully")
	return nil
}

func EditMenuView(c *fiber.Ctx, db *gorm.DB) error {
	id, err := menuParamsID(c)
	if err != nil {
		return err
	}

	var menu model.Menu
	if err := db.First(&menu, id).Error; err != nil {
		return menuNotFound(c, "Menu")
	}

	// Get all menus
	var menus []model.Menu
	if err := db.Find(&menus).Error; err != nil {
		return menuServerError(c, "Could not load menus")
	}
	menuID := uint(menu.ID)

	return c.Render("admin/menu/edit-menu", fiber.Map{
		"Menu":   menu,
		"MenuID": menuID,
		"Menus":  menus,
	})
}

func EditMenuItemView(c *fiber.Ctx, db *gorm.DB) error {
	id, err := menuParamsID(c)
	if err != nil {
		return err
	}

	var menuItem model.MenuItem
	if err := db.First(&menuItem, id).Error; err != nil {
		return menuNotFound(c, "Menu item")
	}

	// get all menus for the select dropdown
	var menus []model.Menu
	if err := db.Find(&menus).Error; err != nil {
		return menuServerError(c, "Could not load menus")
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
	pageSize := 10 // Default page size

	// Convert page string to int for pagination calculation
	pageInt := queryPage(c)

	// Search for menus with pagination and order them by position
	// Ensure to order both menus and their items by their position
	db.Where("title LIKE ? ESCAPE '\\'", likePattern(searchQuery)).
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
		Where("title LIKE ? ESCAPE '\\'", likePattern(searchQuery)).
		Count(&totalMatchingCount)
	totalPages := pageCount(totalMatchingCount, pageSize)

	// Link picker sources for the guided "add item" form, capped so the
	// admin search stays cheap no matter how big the site grows.
	var allMenus []model.Menu
	db.Order("position ASC").Limit(200).Find(&allMenus)
	var pages []model.CustomPage
	db.Order("title ASC").Limit(200).Find(&pages)
	var posts []model.Post
	db.Where("published = ?", true).Order("title ASC").Limit(200).Find(&posts)

	return c.Render("admin/table/menu-table", fiber.Map{
		"Menus":       menus, // No need to separate and recombine by primary status for ordering
		"TotalPages":  totalPages,
		"CurrentPage": pageInt,
		"SearchQuery": searchQuery,
		"AllMenus":    allMenus,
		"Pages":       pages,
		"Posts":       posts,
	})
}

// maxMenuPosition returns one past the highest used position within scope, so
// new entries land at the end without asking the user for a number.
func maxMenuPosition(db *gorm.DB, menuID *uint) int {
	var maxPos *int
	if menuID == nil {
		db.Model(&model.Menu{}).Where("parent_id IS NULL").Select("MAX(position)").Scan(&maxPos)
	} else {
		db.Model(&model.MenuItem{}).Where("menu_id = ?", *menuID).Select("MAX(position)").Scan(&maxPos)
	}
	if maxPos == nil {
		return 1
	}
	return *maxPos + 1
}

// optionalPosition parses an explicit position, falling back to end-of-list.
func optionalPosition(raw string, db *gorm.DB, menuID *uint) int {
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return n
	}
	return maxMenuPosition(db, menuID)
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

	htmlMenuString := buildMenuHTML(menu, isAdmin, userLoggedIn, c.Path(), registrationEnabled(db))

	return c.SendString(htmlMenuString)
}

// buildMenuHTML renders the inner content of the navbar collapse wrapper (the
// wrapper itself lives in views/partials/header.html so the mobile toggler
// works before HTMX loads). Titles and links are escaped: menu content is
// admin input rendered into every page.
func buildMenuHTML(menu model.Menu, isAdmin bool, userLoggedIn bool, currentPath string, registrationOpen bool) string {
	htmlMenuString := "<ul class=\"navbar-nav me-auto\">\n"

	// Sort MenuItems and SubMenus together based on position
	sort.SliceStable(menu.MenuItems, func(i, j int) bool {
		return menu.MenuItems[i].Position < menu.MenuItems[j].Position
	})

	// Loop through top-level MenuItems
	for _, menuItem := range menu.MenuItems {
		active := ""
		aria := ""
		if menuItem.Link == currentPath {
			active = " active"
			aria = " aria-current=\"page\""
		}
		htmlMenuString += fmt.Sprintf("\t\t<li class=\"nav-item\"><a class=\"nav-link%s\"%s href=\"%s\">%s</a></li>\n",
			active, aria, htmlstd.EscapeString(menuItem.Link), htmlstd.EscapeString(menuItem.Title))
	}

	// Render SubMenus if available
	for _, subMenu := range menu.SubMenus {
		dropdownID := "navbarDropdownMenuLink-" + strconv.Itoa(int(subMenu.ID))
		htmlMenuString += "\t\t<li class=\"nav-item dropdown\">\n"
		htmlMenuString += "\t\t\t<a class=\"nav-link dropdown-toggle\" href=\"#\" id=\"" + dropdownID + "\" role=\"button\" data-bs-toggle=\"dropdown\" aria-expanded=\"false\">" + htmlstd.EscapeString(subMenu.Title) + "</a>\n"
		htmlMenuString += "\t\t\t<ul class=\"dropdown-menu\" aria-labelledby=\"" + dropdownID + "\">\n"
		for _, subMenuItem := range subMenu.MenuItems {
			htmlMenuString += "\t\t\t\t<li><a class=\"dropdown-item\" href=\"" + htmlstd.EscapeString(subMenuItem.Link) + "\">" + htmlstd.EscapeString(subMenuItem.Title) + "</a></li>\n"
		}
		htmlMenuString += "\t\t\t</ul>\n"
		htmlMenuString += "\t\t</li>\n"
	}

	htmlMenuString += "</ul>\n"

	// Admin and user controls
	if isAdmin {
		htmlMenuString += adminControls()
	}

	if userLoggedIn {
		htmlMenuString += userControls(true, true)
	} else {
		htmlMenuString += userControls(false, registrationOpen)
	}

	return htmlMenuString
}
func adminControls() string {
	return `<div class="d-flex gap-2 my-2 my-lg-0 me-lg-2" role="group" aria-label="Admin group">
		<a href="/admin" class="btn btn-sm btn-outline-primary">Admin Dashboard</a>
		<button class="btn btn-sm btn-outline-primary" hx-post="/clear-cache" hx-trigger="click" hx-confirm="Are you sure you want to clear the cache?" hx-swap="none" hx-headers='{"X-No-Cache": "true"}'>Clear Cache</button>
		</div>`
}

func userControls(loggedIn bool, registrationOpen bool) string {
	if loggedIn {
		return `<ul class="navbar-nav ms-auto">
			<li class="nav-item me-lg-2"><a class="btn btn-sm btn-outline-secondary my-2 my-lg-0" href="/account">Account</a></li>
			<li class="nav-item"><button hx-post="/logout" hx-swap="none" hx-target="body" hx-headers='{"X-No-Cache": "true"}' hx-confirm="Log out?" class="btn btn-sm btn-outline-secondary my-2 my-lg-0">Logout</button></li>
			</ul>`
	}
	out := `<ul class="navbar-nav ms-auto">
		<li class="nav-item me-lg-2"><a class="btn btn-sm btn-outline-primary my-2 my-lg-0" href="/login">Login</a></li>`
	if registrationOpen {
		out += `
		<li class="nav-item"><a class="btn btn-sm btn-primary my-2 my-lg-0" href="/register">Register</a></li>`
	}
	return out + `
		</ul>`
}
