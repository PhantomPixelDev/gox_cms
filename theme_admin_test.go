package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goxcms/model"

	"github.com/gofiber/fiber/v2"
)

// viewsDir is the template root, relative to the package directory.
const viewsDir = "views"

// themeDir is where site themes live.
const themeDir = "views/site"

// writeTheme creates a minimal complete theme, so a test can then break it in
// one specific way.
func writeTheme(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(themeDir, name)
	if err := os.MkdirAll(filepath.Join(dir, "blog"), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "page"), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	files := map[string]string{
		"layout.html":              "<html><body>{{embed}}</body></html>",
		"index.html":               "<p>home</p>",
		"search.html":              "<p>search</p>",
		"404.html":                 "<p>gone</p>",
		"blog/blog.html":           "<p>blog</p>",
		"blog/blog_post.html":      "<p>post</p>",
		"blog/blog_category.html":  "<p>cat</p>",
		"blog/blog_tag.html":       "<p>tag</p>",
		"page/page.html":           "<p>page</p>",
		"page/page_sidebar.html":   "<p>side</p>",
		"page/page_fullwidth.html": "<p>wide</p>",
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s/%s: %v", dir, rel, err)
		}
	}
	return dir
}

// A theme is discovered by existing on disk. There is no registry to add it to
// and no code to change, which is the whole point of the filesystem approach.
func TestThemeIsDiscoveredByExistingOnDisk(t *testing.T) {
	dir := writeTheme(t, "discovered-theme")
	defer os.RemoveAll(dir)

	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	// Discovery happens when the page renders, so a theme dropped in while the
	// server is running shows up on the next page load with no restart.
	resp, body := do(t, app, "GET", "/admin-settings", auth)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET /admin-settings: status %d", resp.StatusCode)
	}
	if !strings.Contains(body, `value="discovered-theme"`) {
		t.Error("a theme created on disk is not offered in the selector")
	}
}

// An incomplete theme must be visible but not selectable, and activating it must
// be refused with an explanation. A theme that half renders is worse than one
// that is obviously not ready.
func TestIncompleteThemeIsRejected(t *testing.T) {
	dir := writeTheme(t, "halfbuilt-theme")
	defer os.RemoveAll(dir)

	// Remove one required template, and break another so the parse error is
	// reported too.
	if err := os.Remove(filepath.Join(dir, "page", "page_fullwidth.html")); err != nil {
		t.Fatalf("remove template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("{{ if .X }}unclosed"), 0o644); err != nil {
		t.Fatalf("break template: %v", err)
	}

	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	// The selector shows it, and says why it cannot be used.
	_, page := do(t, app, "GET", "/admin-settings", auth)
	if !strings.Contains(page, `value="halfbuilt-theme"`) {
		t.Error("the incomplete theme is not listed at all, so there is nothing to explain")
	}
	if !strings.Contains(page, "(incomplete)") {
		t.Error("the incomplete theme is not marked as incomplete")
	}
	if !strings.Contains(page, "page_fullwidth.html") {
		t.Error("the admin UI does not say which template is missing")
	}
	// And the option must be disabled, not merely labelled. The tag carries
	// conditional attributes, so compare on the element rather than on a fixed
	// string.
	opt := optionTag(t, page, "halfbuilt-theme")
	if !strings.Contains(opt, "disabled") {
		t.Errorf("the incomplete theme is selectable: %s", opt)
	}

	// A complete theme's option must not be disabled, or nothing could ever be
	// switched to.
	opt = optionTag(t, page, "simple")
	if strings.Contains(opt, "disabled") {
		t.Errorf("a complete theme is disabled: %s", opt)
	}

	// Activating it anyway must be refused and must not change the setting.
	form := url.Values{"name": {"GoX CMS"}, "site_template": {"halfbuilt-theme"}, "container_class": {"container"}}
	resp, msg := postForm(t, app, "/update-settings", form, token, auth)
	if resp.StatusCode == fiber.StatusOK && strings.Contains(msg, "activated") {
		t.Error("an incomplete theme was activated")
	}

	var info model.BasicWebsiteInfo
	if err := db.Order("id ASC").First(&info).Error; err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if info.SiteTemplate == "halfbuilt-theme" {
		t.Error("the site is now on a theme that cannot render")
	}

	// The site must still work.
	resp, _ = do(t, app, "GET", "/", auth)
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("the home page is broken after a rejected theme activation: status %d", resp.StatusCode)
	}
}

// A complete theme created on disk must actually be usable end to end, without
// a rebuild or restart. That is the promise the filesystem discovery makes.
func TestThemeOnDiskIsUsableWithoutRestart(t *testing.T) {
	dir := writeTheme(t, "live-theme")
	defer os.RemoveAll(dir)

	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	form := url.Values{"name": {"GoX CMS"}, "site_template": {"live-theme"}, "container_class": {"container"}}
	if resp, msg := postForm(t, app, "/update-settings", form, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("activate: status %d: %s", resp.StatusCode, msg)
	}

	// Every public page renders from the new theme, with no restart.
	for _, path := range []string{"/", "/blog", "/search?q=x", "/no-such-page"} {
		resp, body := do(t, app, "GET", path, auth)
		if resp.StatusCode >= 500 {
			t.Errorf("GET %s: status %d", path, resp.StatusCode)
			continue
		}
		if !strings.Contains(body, "home") && !strings.Contains(body, "blog") &&
			!strings.Contains(body, "search") && !strings.Contains(body, "gone") {
			t.Errorf("GET %s: the new theme's templates did not render", path)
		}
		// It must not fall back to the default theme's Bootstrap shell.
		if strings.Contains(body, "navbar-expand-lg") {
			t.Errorf("GET %s: still rendering the default Bootstrap theme", path)
		}
	}

	// And the fragments resolve to the new theme's templates too.
	resp, frag := do(t, app, "GET", "/frag/blog", auth)
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("GET /frag/blog: status %d", resp.StatusCode)
	}
	if !strings.Contains(frag, "blog") {
		t.Errorf("GET /frag/blog did not use the new theme: %q", frag)
	}
}

// Every incomplete theme must be reported, not just the first one. An author
// with two half-built themes used to be told about one of them and left guessing
// about the other.
func TestEveryIncompleteThemeIsReported(t *testing.T) {
	first := writeTheme(t, "broken-one")
	defer os.RemoveAll(first)
	second := writeTheme(t, "broken-two")
	defer os.RemoveAll(second)

	// Break each one differently: a missing file, and a parse error.
	if err := os.Remove(filepath.Join(first, "404.html")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.WriteFile(filepath.Join(second, "search.html"), []byte("{{ range }}"), 0o644); err != nil {
		t.Fatalf("break: %v", err)
	}

	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	_, page := do(t, app, "GET", "/admin-settings", auth)
	for _, name := range []string{"broken-one", "broken-two"} {
		if !strings.Contains(page, name+`" disabled`) && !strings.Contains(page, `value="`+name+`"`) {
			t.Errorf("theme %q is not listed", name)
		}
	}
	// Both problems are explained: a missing file and a parse error.
	if !strings.Contains(page, "404.html") {
		t.Error("the missing 404.html of broken-one is not reported")
	}
	if !strings.Contains(page, "Template error") {
		t.Error("the parse error in broken-two is not reported")
	}
}

// optionTag returns the opening <option ...> tag for a given value, so an
// assertion about an element's attributes is not a comparison against a fixed
// string that whitespace changes invalidate.
func optionTag(t *testing.T, page, value string) string {
	t.Helper()
	needle := `<option value="` + value + `"`
	i := strings.Index(page, needle)
	if i < 0 {
		t.Fatalf("no option with value %q on the page", value)
	}
	j := strings.Index(page[i:], ">")
	if j < 0 {
		t.Fatalf("option tag for %q is never closed", value)
	}
	return page[i : i+j+1]
}
