package main

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"goxcms/model"

	"github.com/gofiber/fiber/v2"
)

// The navigation is rendered inline into every page now, from the database,
// rather than fetched from /get-primary-menu with htmx. This checks the markup
// really comes from the menu the admin built.
//
// Note the seed already creates a primary menu with Home/Blog/About, and a
// second primary menu is not chosen over it: the site uses the first by
// position. So this test edits the menu that is actually in use rather than
// creating a competing one.
func TestMenuIsRenderedInlineFromTheDatabase(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	var menu model.Menu
	if err := db.Where("is_primary = ?", true).Order("position ASC").First(&menu).Error; err != nil {
		t.Fatalf("no primary menu: %v", err)
	}

	add := func(title, link string) {
		t.Helper()
		form := url.Values{
			"menu_item_title": {title},
			"menu_item_link":  {link},
			"menu_item_menu":  {strconv.Itoa(int(menu.ID))},
		}
		if resp, _ := postForm(t, app, "/add-menu-item", form, token, auth); resp.StatusCode != fiber.StatusOK {
			t.Fatalf("add %q: %d", title, resp.StatusCode)
		}
	}
	add("Alpha", "/alpha")

	_, body := do(t, app, "GET", "/", auth)
	nav := navRegion(t, body)

	// The new item must be in the server-rendered page, with no client-side
	// fetch needed to see it.
	if !strings.Contains(nav, `href="/alpha"`) {
		t.Errorf("navigation missing the item just added\nnav: %s", nav)
	}
	// The seeded items must be there too: the menu is data, not a hard-coded
	// list of Home/Blog/Search.
	if !strings.Contains(nav, `href="/blog"`) {
		t.Errorf("navigation missing the seeded Blog item\nnav: %s", nav)
	}
	// The old implementation pulled the menu in with this request. If it is
	// still there the round trip is still happening.
	if strings.Contains(body, "get-primary-menu") {
		t.Error("page still fetches the menu over HTTP; it should be server-rendered")
	}
}

// A menu edit has to be visible immediately. The tree is cached for a minute,
// so every mutation has to drop the cache; without that an admin renames a
// link and the site keeps showing the old one for up to 60 seconds.
func TestMenuEditInvalidatesTheCachedTree(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	var menu model.Menu
	if err := db.Where("is_primary = ?", true).Order("position ASC").First(&menu).Error; err != nil {
		t.Fatalf("no primary menu: %v", err)
	}

	form := url.Values{
		"menu_item_title": {"Fresh"},
		"menu_item_link":  {"/fresh"},
		"menu_item_menu":  {strconv.Itoa(int(menu.ID))},
	}
	if resp, _ := postForm(t, app, "/add-menu-item", form, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("add item: %d", resp.StatusCode)
	}

	// First render populates the cache.
	if _, body := do(t, app, "GET", "/", auth); !strings.Contains(navRegion(t, body), "/fresh") {
		t.Fatal("item missing on the first render")
	}

	// Rename it directly in the database, bypassing the handler, then mutate
	// through the handler: the handler's invalidation is what is under test.
	var item model.MenuItem
	db.Where("title = ?", "Fresh").First(&item)
	db.Model(&model.MenuItem{}).Where("id = ?", item.ID).Update("title", "Renamed")

	// Any menu mutation drops the cache. Move the item up.
	if resp, _ := postForm(t, app, "/move-menu-item/"+strconv.Itoa(int(item.ID))+"/up", nil, token, auth); resp.StatusCode == 0 {
		t.Fatal("move returned no response")
	}

	_, body := do(t, app, "GET", "/", auth)
	nav := navRegion(t, body)
	if !strings.Contains(nav, "Renamed") {
		t.Errorf("menu edit not visible on the next page load; cache was not invalidated\nnav: %s", nav)
	}
}

// Fragments are the supported way to do partial updates. hx-get on a normal
// page URL injects a second full <html> document, which is the problem they
// exist to solve, so every fragment must come back layout-free.
func TestFragmentsReturnLayoutlessMarkup(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	// A published post and a page, so the fragment routes have something to
	// resolve.
	now := time.Now()
	post := model.Post{
		Title: "Fragment Post", Slug: "fragment-post",
		Content: "<p>Body</p>", Published: true, CreatedAt: now,
	}
	if err := db.Create(&post).Error; err != nil {
		t.Fatalf("create post: %v", err)
	}
	page := model.CustomPage{
		Title: "Fragment Page", Slug: "fragment-page",
		Content: "<p>Page body</p>", Published: true,
	}
	if err := db.Create(&page).Error; err != nil {
		t.Fatalf("create page: %v", err)
	}

	cases := []struct {
		path string
		want string
	}{
		{"/frag/menu", "navbar-nav"},
		{"/frag/blog", "post"},
		{"/frag/blog?page=1", "post"},
		{"/frag/post/fragment-post", "Body"},
		{"/frag/comments/fragment-post", "comment"},
		{"/frag/page/fragment-page", "Page body"},
		{"/frag/search?q=Fragment", "Fragment Post"},
	}

	for _, tc := range cases {
		resp, body := do(t, app, "GET", tc.path, auth)
		if resp.StatusCode != fiber.StatusOK {
			t.Errorf("GET %s: status %d", tc.path, resp.StatusCode)
			continue
		}
		// The whole point: no document chrome, or a swap nests one <html>
		// inside another. Matched precisely, because a plain "<head" also
		// matches <header, which every page legitimately has.
		for _, banned := range []string{"<html", "<head>", "<head ", "<body", "<!doctype"} {
			if strings.Contains(strings.ToLower(body), banned) {
				t.Errorf("GET %s: fragment contains %q, so it cannot be swapped in cleanly", tc.path, banned)
			}
		}
		if !strings.Contains(body, tc.want) {
			t.Errorf("GET %s: body missing %q", tc.path, tc.want)
		}
	}
}

// A fragment that redirects breaks htmx: hx-get follows the 302 and swaps the
// target's response, which is a whole page, into the panel.
func TestFragmentsDoNotRedirectOnOutOfRangePage(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	if err := db.Create(&model.Post{
		Title: "Only", Slug: "only", Content: "x", Published: true, CreatedAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("create post: %v", err)
	}

	// The full page redirects, which is right for a bookmark.
	resp, _ := do(t, app, "GET", "/blog/500", auth)
	if resp.StatusCode != fiber.StatusFound && resp.StatusCode != fiber.StatusMovedPermanently {
		t.Errorf("GET /blog/500: status %d, want a redirect to the last page", resp.StatusCode)
	}

	// The fragment must not.
	resp, body := do(t, app, "GET", "/frag/blog?page=500", auth)
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("GET /frag/blog?page=500: status %d, want 200 (a fragment must not redirect)", resp.StatusCode)
	}
	if strings.Contains(strings.ToLower(body), "<html") {
		t.Error("out-of-range fragment returned a whole page")
	}
}

// A draft custom page must not be reachable through the fragment endpoint
// either. Adding a new read path is exactly how a leak gets reintroduced.
func TestFragmentCustomPageHidesDrafts(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	if err := db.Create(&model.CustomPage{
		Title: "Secret", Slug: "secret", Content: "<p>classified</p>", Published: false,
	}).Error; err != nil {
		t.Fatalf("create draft: %v", err)
	}

	resp, body := do(t, app, "GET", "/frag/page/secret", auth)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("draft page via fragment: status %d, want 404", resp.StatusCode)
	}
	if strings.Contains(body, "classified") {
		t.Error("draft page content served through the fragment endpoint")
	}
}

// navRegion extracts the <nav> element, so an assertion about the menu cannot
// accidentally match a link in the page body or the footer.
func navRegion(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, "<nav")
	if i < 0 {
		t.Fatalf("no <nav> in page")
	}
	j := strings.Index(body[i:], "</nav>")
	if j < 0 {
		return body[i:]
	}
	return body[i : i+j+6]
}
