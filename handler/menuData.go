package handlers

import (
	"sort"
	"strings"
	"sync"
	"time"

	"goxcms/model"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// MenuNode is one entry in the site's navigation tree.
//
// The navbar used to be generated as a Go string with hard-coded Bootstrap
// classes, which locked every theme to Bootstrap markup: a framework-free theme
// could not restyle or restructure the menu at all. Themes now render this
// data themselves, from partials/menu.html or their own markup.
type MenuNode struct {
	// ID is the menu or menu-item id, for hx-targets and keys.
	ID uint
	// Title and Link are raw; html/template escapes them on output.
	Title string
	Link  string
	// Active is true for the entry matching the current path.
	Active bool
	// HasChildren marks a submenu parent.
	HasChildren bool
	// Position is the sort order within the menu.
	Position int
	// Children are the submenu's items.
	Children []MenuNode
}

// MenuData is everything a theme needs to draw the navbar.
type MenuData struct {
	// Items are the top-level links and submenus, in position order.
	Items []MenuNode
	// CurrentPath is the request path, so a theme can mark the active entry
	// itself if it wants a different rule than exact matching.
	CurrentPath string
	// IsAdmin, IsLoggedIn and RegistrationOpen drive the account controls.
	IsAdmin          bool
	IsLoggedIn       bool
	RegistrationOpen bool
}

// menuCache holds the built tree. Menus change only from the admin menu
// editor, so the query is cached and invalidated by the menu mutations.
//
// Before this, every page load issued four menu queries plus an extra HTTP
// round trip to /get-primary-menu, and the {{timestamp}} cache-buster in that
// URL changed every second, which defeated HTTP caching by construction.
var menuCache struct {
	sync.RWMutex
	data      []MenuNode
	loadedAt  time.Time
	hasCached bool
	// owner is the db handle the cached tree was built from. Without it, two
	// databases in one process (which is what the test suite has, one per
	// test) would share a cache and a menu built from one would be served to
	// the other.
	owner *gorm.DB
}

const menuCacheTTL = 60 * time.Second

// InvalidateMenuCache drops the cached menu. Called by every menu mutation so
// an edit shows on the next page load rather than after a minute.
func InvalidateMenuCache() {
	menuCache.Lock()
	menuCache.hasCached = false
	menuCache.data = nil
	menuCache.owner = nil
	menuCache.Unlock()
}

// cachedMenuTree returns the navigation tree, querying at most once per TTL.
func cachedMenuTree(db *gorm.DB) []MenuNode {
	menuCache.RLock()
	fresh := menuCache.hasCached && menuCache.owner == db && time.Since(menuCache.loadedAt) < menuCacheTTL
	cached := menuCache.data
	menuCache.RUnlock()
	if fresh {
		return cached
	}

	tree := loadMenuTree(db)

	menuCache.Lock()
	menuCache.data = tree
	menuCache.loadedAt = time.Now()
	menuCache.hasCached = true
	menuCache.owner = db
	menuCache.Unlock()
	return tree
}

// loadMenuTree reads the primary menu and its one level of submenus.
func loadMenuTree(db *gorm.DB) []MenuNode {
	byPosition := func(db *gorm.DB) *gorm.DB {
		return db.Order("position ASC")
	}

	var menu model.Menu
	// Both preloads are required. Without Preload("MenuItems") here the
	// top-level links are simply absent, and the navigation renders as an empty
	// list with no error anywhere.
	if err := byPosition(db.Where("is_primary = ?", true)).
		Preload("MenuItems", byPosition).
		First(&menu).Error; err != nil {
		return nil
	}
	// One level of submenus, with their items, in two queries. This used to be
	// skipped, so submenu items were never loaded either.
	if err := byPosition(db.Where("parent_id = ?", menu.ID)).
		Preload("MenuItems", byPosition).
		Find(&menu.SubMenus).Error; err != nil {
		return nil
	}

	// The preloads already arrive ordered by position; the explicit sort is a
	// safety net so a changed preload scope cannot silently reorder the nav.
	sortItems := func(items []*model.MenuItem) []*model.MenuItem {
		out := make([]*model.MenuItem, len(items))
		copy(out, items)
		sort.SliceStable(out, func(i, j int) bool { return out[i].Position < out[j].Position })
		return out
	}

	tree := make([]MenuNode, 0, len(menu.MenuItems)+len(menu.SubMenus))
	for _, item := range sortItems(menu.MenuItems) {
		tree = append(tree, MenuNode{
			ID: item.ID, Title: item.Title, Link: item.Link, Position: item.Position,
		})
	}
	for _, sub := range menu.SubMenus {
		node := MenuNode{
			ID:          sub.ID,
			Title:       sub.Title,
			Position:    sub.Position,
			HasChildren: true,
		}
		for _, item := range sortItems(sub.MenuItems) {
			node.Children = append(node.Children, MenuNode{
				ID: item.ID, Title: item.Title, Link: item.Link, Position: item.Position,
			})
		}
		tree = append(tree, node)
	}
	// Links and submenus are merged by position, which is what the position
	// field implies: a submenu can sit between two plain links.
	sort.SliceStable(tree, func(i, j int) bool {
		if tree[i].Position != tree[j].Position {
			return tree[i].Position < tree[j].Position
		}
		return !tree[i].HasChildren && tree[j].HasChildren
	})
	return tree
}

// BuildMenuData assembles the navbar data for a request, marking the active
// entry by exact path match with a trailing-slash-insensitive comparison.
func BuildMenuData(db *gorm.DB, c *fiber.Ctx) MenuData {
	items := cachedMenuTree(db)
	// Deep copy before marking. A shallow copy shares the Children slices
	// with the cache, so marking a submenu child would write back into it and
	// the next visitor would be shown someone else's active menu item.
	marked := deepCopyMenu(items)
	markMenuActive(marked, c.Path())

	return MenuData{
		Items:            marked,
		CurrentPath:      c.Path(),
		IsAdmin:          IsTrue(c, "isAdmin"),
		IsLoggedIn:       IsTrue(c, "isLoggedin"),
		RegistrationOpen: registrationEnabled(db),
	}
}

// deepCopyMenu copies the tree including each node's Children slice, so callers
// can mark per-request state without touching the cached original.
func deepCopyMenu(items []MenuNode) []MenuNode {
	if items == nil {
		return nil
	}
	out := make([]MenuNode, len(items))
	for i, it := range items {
		out[i] = it
		if it.Children != nil {
			out[i].Children = make([]MenuNode, len(it.Children))
			copy(out[i].Children, it.Children)
		}
	}
	return out
}

// markMenuActive sets Active on the entry whose link matches the current path.
func markMenuActive(items []MenuNode, path string) {
	want := normalisePath(path)
	for i := range items {
		// A submenu parent has no link of its own. Comparing its empty link
		// would normalise to "/" and mark it active on the home page.
		if items[i].Link != "" && normalisePath(items[i].Link) == want {
			items[i].Active = true
		}
		for j := range items[i].Children {
			if normalisePath(items[i].Children[j].Link) == want {
				// A child match also highlights its parent, so a theme that
				// only styles .active gets the right result.
				items[i].Active = true
				items[i].Children[j].Active = true
			}
		}
	}
}

// normalisePath makes "/" and "/blog/" compare equal to the unslashed form, so
// a link matches the current page whichever way the visitor typed it.
func normalisePath(p string) string {
	if p == "" {
		return "/"
	}
	// Cut every trailing slash, not just one: strings.TrimSuffix("/blog///",
	// "/") would leave "/blog//". Trimming the query string as well would
	// break /search?q=1, so it is left alone.
	if i := strings.IndexByte(p, '?'); i >= 0 {
		base, query := p[:i], p[i:]
		if trimmed := strings.TrimRight(base, "/"); trimmed != "" {
			return trimmed + query
		}
		return "/" + query
	}
	trimmed := strings.TrimRight(p, "/")
	if trimmed == "" {
		return "/"
	}
	return trimmed
}

// IsTrue lives in authHandlers.go; it is reused here rather than duplicated.

// MenuForRender returns the navbar data for embedding in a page render.
func MenuForRender(db *gorm.DB, c *fiber.Ctx) MenuData {
	return BuildMenuData(db, c)
}
