package handlers

import (
	"goxcms/model"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func AddMenu(c *fiber.Ctx, db *gorm.DB) error {
	// The navigation tree is cached; drop it before the change so the next
	// page load rebuilds it.
	InvalidateMenuCache()

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
	// The navigation tree is cached; drop it before the change so the next
	// page load rebuilds it.
	InvalidateMenuCache()

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
	// The navigation tree is cached; drop it before the change so the next
	// page load rebuilds it.
	InvalidateMenuCache()

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
	// The navigation tree is cached; drop it before the change so the next
	// page load rebuilds it.
	InvalidateMenuCache()

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
	// The navigation tree is cached; drop it before the change so the next
	// page load rebuilds it.
	InvalidateMenuCache()

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
	// The navigation tree is cached; drop it before the change so the next
	// page load rebuilds it.
	InvalidateMenuCache()

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
	// The navigation tree is cached; drop it before the change so the next
	// page load rebuilds it.
	InvalidateMenuCache()

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

	// Parent selection. A menu may not be its own parent, nor be moved under
	// one of its own submenus: both produce a cycle, and a self-parented
	// primary menu renders as its own submenu in the navbar.
	if raw := c.FormValue("parent_id"); raw != "" && raw != "0" {
		parentID, err := strconv.Atoi(raw)
		if err != nil || parentID <= 0 {
			ShowToastError(c, "Invalid parent menu")
			return c.Status(fiber.StatusBadRequest).SendString("Invalid parent menu")
		}
		if uint(parentID) == menu.ID {
			ShowToastError(c, "A menu cannot be its own parent")
			return c.Status(fiber.StatusBadRequest).SendString("A menu cannot be its own parent")
		}
		if menuDescendantIDs(db, menu.ID)[uint(parentID)] {
			ShowToastError(c, "A menu cannot be moved under its own submenu")
			return c.Status(fiber.StatusBadRequest).SendString("A menu cannot be moved under its own submenu")
		}
		parent := uint(parentID)
		menu.ParentID = &parent
	} else {
		menu.ParentID = nil
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
	// The navigation tree is cached; drop it before the change so the next
	// page load rebuilds it.
	InvalidateMenuCache()

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

	// Only primary menus can act as a parent (the navbar renders one level of
	// submenus). The menu being edited is filtered out in the template.
	var primaryMenus []model.Menu
	if err := db.Where("is_primary = ?", true).Order("position ASC").Find(&primaryMenus).Error; err != nil {
		return menuServerError(c, "Could not load menus")
	}

	return c.Render("admin/menu/edit-menu", fiber.Map{
		"Menu":         menu,
		"MenuID":       menu.ID,
		"PrimaryMenus": primaryMenus,
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

	// Every menu is a valid target, including submenus, so an item can be
	// re-homed from the modal.
	var allMenus []model.Menu
	if err := db.Order("position ASC").Find(&allMenus).Error; err != nil {
		return menuServerError(c, "Could not load menus")
	}

	return c.Render("admin/menu/edit-menu-item", fiber.Map{
		"MenuItem":   menuItem,
		"AllMenus":   allMenus,
		"MenuItemID": menuItem.ID,
	})
}

func SearchMenuAdminTable(c *fiber.Ctx, db *gorm.DB) error {
	// Search matches the menu title OR any of its items (the field is
	// labelled "search menus and items"), so a matching item keeps its
	// parent menu visible.
	searchQuery := c.Query("query")
	pageSize := 10 // Default page size

	var menus []model.Menu
	// Convert page string to int for pagination calculation
	pageInt := queryPage(c)

	// Order menus and their items by position
	db.Where("title LIKE ? ESCAPE '\\'", likePattern(searchQuery)).
		Or("id IN (?)", db.Model(&model.MenuItem{}).
			Select("menu_id").
			Where("title LIKE ? ESCAPE '\\' OR link LIKE ? ESCAPE '\\'",
						likePattern(searchQuery), likePattern(searchQuery))).
		Order("position ASC"). // Order menus by position
		Preload("MenuItems", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC") // Order menu items by position within each menu
		}).
		Preload("SubMenus", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC") // Order sub-menus by position
		}).
		// Nested preload: without it sub-menu items were never loaded, so the
		// admin tree rendered each submenu as an empty heading.
		Preload("SubMenus.MenuItems", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC")
		}).
		Limit(pageSize).
		Offset((pageInt - 1) * pageSize).
		Find(&menus)

	// Count total menus that match the search query for pagination
	var totalMatchingCount int64
	db.Model(&model.Menu{}).
		Where("title LIKE ? ESCAPE '\\'", likePattern(searchQuery)).
		Or("id IN (?)", db.Model(&model.MenuItem{}).
			Select("menu_id").
			Where("title LIKE ? ESCAPE '\\' OR link LIKE ? ESCAPE '\\'",
				likePattern(searchQuery), likePattern(searchQuery))).
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

	// Item counts (own + nested) drive the "N items" summary in the header
	// of each menu card and the delete confirmation.
	counts := map[uint]int{}
	for _, m := range menus {
		total := len(m.MenuItems)
		for _, sub := range m.SubMenus {
			total += len(sub.MenuItems)
		}
		counts[m.ID] = total
	}

	// Only these menus can be a submenu's parent: the tree is rendered one
	// level deep, and attaching a submenu under a submenu would hide items.
	var primaryMenus []model.Menu
	db.Where("is_primary = ?", true).Order("position ASC").Find(&primaryMenus)

	return c.Render("admin/table/menu-table", fiber.Map{
		"Menus":        menus, // No need to separate and recombine by primary status for ordering
		"TotalPages":   totalPages,
		"CurrentPage":  pageInt,
		"SearchQuery":  searchQuery,
		"AllMenus":     allMenus,
		"PrimaryMenus": primaryMenus,
		"ItemCounts":   counts,
		"Pages":        pages,
		"Posts":        posts,
	})
}

// menuDescendantIDs walks a menu's submenu chain and returns every ID below
// it, so a menu can never be re-parented under its own descendant.
func menuDescendantIDs(db *gorm.DB, rootID uint) map[uint]bool {
	seen := map[uint]bool{}
	var walk func(uint)
	walk = func(id uint) {
		var children []model.Menu
		if err := db.Where("parent_id = ?", id).Find(&children).Error; err != nil {
			return
		}
		for _, child := range children {
			if seen[child.ID] {
				continue
			}
			seen[child.ID] = true
			walk(child.ID)
		}
	}
	walk(rootID)
	return seen
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

// GetPrimaryMenuRender serves the navigation as a layout-less fragment.
//
// It used to return a Go-built Bootstrap string that the header fetched with
// htmx on every page load. The menu is now rendered inline from the active
// theme, and this endpoint is kept for themes that want to refresh the nav
// without a full page load.
func GetPrimaryMenuRender(c *fiber.Ctx, db *gorm.DB) error {
	return RenderSharedFragment(c, "menu", fiber.Map{
		"Title": "Menu",
		"Menu":  BuildMenuData(db, c),
	})
}

// optionalPosition parses an explicit position, falling back to end-of-list.
func optionalPosition(raw string, db *gorm.DB, menuID *uint) int {
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return n
	}
	return maxMenuPosition(db, menuID)
}
