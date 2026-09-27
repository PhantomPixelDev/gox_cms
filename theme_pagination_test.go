package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"goxcms/model"

	"github.com/gofiber/fiber/v2"
)

func TestPaginationPathsActuallyChangeThePage(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	// 25 posts at 10 per page is 3 pages.
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 25; i++ {
		p := model.Post{
			Title:     fmt.Sprintf("Paged post %02d", i),
			Slug:      fmt.Sprintf("paged-%02d", i),
			Content:   "body",
			Published: true,
			CreatedAt: base.Add(time.Duration(i) * time.Hour),
		}
		if err := db.Create(&p).Error; err != nil {
			t.Fatalf("create post %d: %v", i, err)
		}
	}

	_, page1 := do(t, app, "GET", "/blog", auth)
	_, page2 := do(t, app, "GET", "/blog/2", auth)
	_, page3 := do(t, app, "GET", "/blog/3", auth)

	// Each page must show different posts, and the ORDER BY must be stable, or
	// a post can repeat or vanish as the visitor pages.
	// Lists are newest first, so page 1 holds the most recent posts and page 3
	// the oldest.
	if !strings.Contains(page1, "Paged post 24") {
		t.Error("page 1 is missing the newest post")
	}
	if !strings.Contains(page2, "Paged post 10") {
		t.Error("page 2 is missing the 11th post")
	}
	if !strings.Contains(page3, "Paged post 00") {
		t.Error("page 3 is missing the oldest post")
	}
	// A post from another page must not leak in.
	if strings.Contains(page1, "Paged post 10") {
		t.Error("page 1 shows a post that belongs to page 2")
	}
	if strings.Contains(page3, "Paged post 10") {
		t.Error("page 3 shows a post that belongs to page 2")
	}

	// A page past the end redirects to the last page rather than rendering
	// nothing, so a stale bookmark still works.
	resp, _ := do(t, app, "GET", "/blog/99", auth)
	if resp.StatusCode != fiber.StatusFound && resp.StatusCode != fiber.StatusMovedPermanently {
		t.Errorf("GET /blog/99: status %d, want a redirect", resp.StatusCode)
	}
}

// The simple theme's paginator used to build links like
// /blog/category/foo?page=2 while the route is /blog/category/:slug/:page?.
// The query parameter was ignored, so every page number returned page 1 and the
// theme's pagination looked fine but did nothing. This is the regression test.
func TestThemePaginationUsesPathSegmentsNotQueryParams(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 25; i++ {
		p := model.Post{
			Title:     fmt.Sprintf("Cat post %02d", i),
			Slug:      fmt.Sprintf("cat-%02d", i),
			Content:   "body",
			Published: true,
			CreatedAt: base.Add(time.Duration(i) * time.Hour),
		}
		if err := db.Create(&p).Error; err != nil {
			t.Fatalf("create post: %v", err)
		}
	}
	cat := model.Category{Name: "Guides", Slug: "guides"}
	if err := db.Create(&cat).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	if err := db.Exec(
		"INSERT INTO post_categories (post_id, category_id) SELECT id, ? FROM posts", cat.ID,
	).Error; err != nil {
		t.Fatalf("link posts to category: %v", err)
	}

	for _, theme := range []string{"simple", "default"} {
		form := url.Values{"name": {"GoX CMS"}, "site_template": {theme}, "container_class": {"container"}}
		if resp, _ := postForm(t, app, "/update-settings", form, token, auth); resp.StatusCode != fiber.StatusOK {
			t.Fatalf("%s: switch theme: %d", theme, resp.StatusCode)
		}

		for _, path := range []string{"/blog", "/blog/category/guides"} {
			_, body := do(t, app, "GET", path, auth)
			pager := pagerRegion(t, body)
			if pager == "" {
				t.Errorf("%s %s: no paginator rendered for 25 posts over 3 pages", theme, path)
				continue
			}
			// A page number in a link's href must be a path segment. The hx-get
			// URL may legitimately carry ?page=, because a fragment takes its
			// page as a query parameter; that is checked separately.
			for _, chunk := range strings.Split(pager, `href="`)[1:] {
				href := chunk
				if i := strings.Index(href, `"`); i > 0 {
					href = href[:i]
				}
				if strings.Contains(href, "?page=") {
					t.Errorf("%s %s: link %q puts the page in the query string, which the route ignores.\npager: %s",
						theme, path, href, pager)
				}
			}
			if !strings.Contains(pager, "/2") {
				t.Errorf("%s %s: paginator has no link to page 2\npager: %s", theme, path, pager)
			}
		}

		// And following that link must actually show page 2.
		_, page2 := do(t, app, "GET", "/blog/category/guides/2", auth)
		if !strings.Contains(page2, "Cat post 10") {
			t.Errorf("%s: /blog/category/guides/2 did not show the second page of results", theme)
		}
	}
}

// A long archive must not render one link per page. A blog with hundreds of
// pages produced hundreds of <li> elements on every list page, for a control
// that shows five numbers at a time.
func TestPaginatorIsWindowed(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// 300 posts at 10 per page is 30 pages.
	for i := 0; i < 300; i++ {
		p := model.Post{
			Title:     fmt.Sprintf("Bulk %03d", i),
			Slug:      fmt.Sprintf("bulk-%03d", i),
			Content:   "body",
			Published: true,
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		if err := db.Create(&p).Error; err != nil {
			t.Fatalf("create post %d: %v", i, err)
		}
	}

	_, body := do(t, app, "GET", "/blog/15", auth)
	pager := pagerRegion(t, body)
	if pager == "" {
		t.Fatal("no paginator on a 30-page blog")
	}

	// Count the numbered links. Windowed: 15 +/- 2, plus first and last, plus
	// prev/next is at most ~9.
	links := strings.Count(pager, "page-link")
	if links > 20 {
		t.Errorf("paginator rendered %d links for 30 pages; it should be windowed to a handful", links)
	}
	// The window must still reach the current page and both ends of the
	// archive. The last page number is not hard-coded: the seeded posts add to
	// the count, so assert that some link reaches well past page 20.
	if !strings.Contains(pager, "/blog/15") {
		t.Errorf("paginator cannot reach the current page\npager: %s", pager)
	}
	if !strings.Contains(pager, `"/blog/1"`) {
		t.Errorf("paginator cannot reach page 1\npager: %s", pager)
	}
	if !reachesPage(pager, 20) {
		t.Errorf("paginator cannot reach the far end of a 30+ page archive\npager: %s", pager)
	}
	// Ellipses mark the gaps.
	if !strings.Contains(pager, "&hellip;") {
		t.Errorf("paginator has gaps but no ellipsis\npager: %s", pager)
	}
}

// reachesPage reports whether any link in the pager points at page n or beyond.
func reachesPage(pager string, n int) bool {
	best := 0
	for _, chunk := range strings.Split(pager, `href="/blog/`)[1:] {
		digits := strings.TrimLeft(chunk, "0123456789")
		digits = chunk[:len(chunk)-len(digits)]
		if v, err := strconv.Atoi(digits); err == nil && v > best {
			best = v
		}
	}
	return best >= n
}

// Paging via htmx must target a fragment endpoint. hx-get on a normal page URL
// injects a second complete <html> document into the current page.
func TestPaginatorHtmxTargetsFragments(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 25; i++ {
		p := model.Post{
			Title:     fmt.Sprintf("Htx %02d", i),
			Slug:      fmt.Sprintf("htx-%02d", i),
			Content:   "body",
			Published: true,
			CreatedAt: base.Add(time.Duration(i) * time.Hour),
		}
		if err := db.Create(&p).Error; err != nil {
			t.Fatalf("create post: %v", err)
		}
	}

	_, body := do(t, app, "GET", "/blog", auth)
	pager := pagerRegion(t, body)
	if pager == "" {
		t.Fatal("no paginator rendered")
	}

	if !strings.Contains(pager, "hx-get") {
		t.Error("paginator has no htmx links, so paging cannot be a fragment swap")
	}
	// Every hx-get must point at a fragment endpoint, never a page URL.
	for _, chunk := range strings.Split(pager, `hx-get="`)[1:] {
		target := chunk
		if i := strings.IndexAny(target, `" `); i > 0 {
			target = target[:i]
		}
		if !strings.HasPrefix(target, "/frag/") {
			t.Errorf("hx-get points at %q, not a /frag/ endpoint; this injects a whole page", target)
		}
	}
	// And the swap target must exist in the same page, or nothing happens.
	if !strings.Contains(body, `id="post-list"`) {
		t.Error("pager targets #post-list but no such element is in the page")
	}
}

// pagerRegion extracts the pagination block so an assertion cannot accidentally
// match a post title or a link elsewhere on the page.
func pagerRegion(t *testing.T, body string) string {
	t.Helper()
	lower := strings.ToLower(body)
	var start int
	switch {
	case strings.Contains(lower, `id="post-list-pager"`):
		i := strings.Index(lower, `id="post-list-pager"`)
		start = strings.LastIndex(lower[:i], "<nav")
	default:
		start = strings.Index(lower, "<nav")
	}
	if start < 0 {
		return ""
	}
	end := strings.Index(lower[start:], "</nav>")
	if end < 0 {
		return body[start:]
	}
	return body[start : start+end+6]
}

var _ = strconv.Itoa
