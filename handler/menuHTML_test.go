package handlers

import (
	"bytes"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// partialSet parses views/partials into one template set, the same way the
// engine does, so these tests exercise the real files on disk.
func partialSet(t *testing.T, names ...string) *template.Template {
	t.Helper()
	root := filepath.Join("..", "views", "partials")
	set := template.New("partials")
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(root, name+".html"))
		if err != nil {
			t.Fatalf("read partial %s: %v", name, err)
		}
		if _, err := set.New("partials/" + name).Parse(string(raw)); err != nil {
			t.Fatalf("parse partial %s: %v", name, err)
		}
	}
	return set
}

func renderPartial(t *testing.T, set *template.Template, name string, data any) string {
	t.Helper()
	var buf bytes.Buffer
	if err := set.ExecuteTemplate(&buf, "partials/"+name, data); err != nil {
		t.Fatalf("render %s: %v", name, err)
	}
	return buf.String()
}

// hostileMenu is admin-supplied navigation content: it is attacker-controlled
// for anyone who can reach the menu editor, and it is rendered on every page
// for every visitor, so it must be escaped.
var hostileMenu = MenuData{
	Items: []MenuNode{
		{ID: 1, Title: `Home`, Link: "/", Position: 1, Active: true},
		{ID: 2, Title: `<script>alert(1)</script>`, Link: "/evil?a=1&b=2", Position: 2},
		{ID: 3, Title: "More", HasChildren: true, Position: 3, Children: []MenuNode{
			{ID: 4, Title: `<img src=x onerror=alert(2)>`, Link: "/x?q=\"&evil=\"", Position: 1},
		}},
	},
	RegistrationOpen: true,
}

// The menu is no longer built in Go, so escaping is html/template's job. This
// is the regression test for that: a Go string builder can only be safe by
// remembering to call html.EscapeString at every interpolation, and one missed
// call is stored XSS on every page of the site.
func TestMenuPartialEscapesAdminContent(t *testing.T) {
	out := renderPartial(t, partialSet(t, "menu"), "menu", map[string]any{"Menu": hostileMenu})

	if strings.Contains(out, "<script>") {
		t.Error("menu title not escaped")
	}
	// Note: "onerror=" alone is not a failure signal, because the literal text
	// survives inside the escaped form (&lt;img src=x onerror=alert(2)&gt;).
	// What matters is that no real tag reaches the browser.
	if strings.Contains(out, "<img") {
		t.Errorf("submenu item title not escaped: %s", out)
	}
	if strings.Contains(out, "javascript:") {
		t.Error("script URL not neutralised")
	}
	if !strings.Contains(out, "/evil?a=1&amp;b=2") {
		t.Error("menu link not escaped")
	}
	// Quotes in an href would otherwise break out of the attribute.
	if strings.Contains(out, `q="&evil="`) {
		t.Error("submenu link not escaped")
	}
}

func TestMenuPartialRendersSubmenusAndActiveState(t *testing.T) {
	out := renderPartial(t, partialSet(t, "menu"), "menu", map[string]any{"Menu": hostileMenu})

	if !strings.Contains(out, `id="menu-dropdown-3"`) {
		t.Error("submenu toggle missing or without a stable id")
	}
	if !strings.Contains(out, `aria-labelledby="menu-dropdown-3"`) {
		t.Error("submenu list not labelled by its toggle")
	}
	// The active entry is marked for both styling and assistive tech.
	if !strings.Contains(out, `class="nav-link active"`) {
		t.Error("active entry not marked")
	}
	if !strings.Contains(out, `aria-current="page"`) {
		t.Error("active entry not exposed to assistive tech")
	}
	if n := strings.Count(out, "<ul"); n != strings.Count(out, "</ul>") {
		t.Errorf("unbalanced lists: %d open, %d close", n, strings.Count(out, "</ul>"))
	}
}

// A theme may not include the menu partial at all and draw its own from
// .Menu.Items, so the partial must render without a Menu.
func TestMenuPartialToleratesMissingMenu(t *testing.T) {
	out := renderPartial(t, partialSet(t, "menu"), "menu", map[string]any{})
	if strings.Contains(out, "<ul") {
		t.Error("menu rendered without data")
	}
}

// The navbar must not pin a colour palette: hardcoding navbar-light/bg-light
// left a white navbar on a dark page with unreadable menu links. A bare
// .navbar follows data-bs-theme, so no colour classes are emitted.
func TestNavbarClassForTheme(t *testing.T) {
	for _, theme := range []string{"anything", "flatly", ""} {
		if got := navbarClassForTheme(theme); got != "" {
			t.Errorf("navbarClassForTheme(%q) = %q, want empty", theme, got)
		}
	}
}

// The server always starts light: the visitor's saved choice takes over
// client-side via site.js.
func TestInitialTheme(t *testing.T) {
	for _, theme := range []string{"darkly", "flatly", ""} {
		if got := initialTheme(theme); got != "light" {
			t.Errorf("initialTheme(%q) = %q, want light", theme, got)
		}
	}
}

func headerData(menu MenuData) map[string]any {
	return map[string]any{
		"Settings": map[string]string{
			"Name":           "GoXCMS",
			"ContainerClass": "container",
			"NavbarClass":    "",
		},
		"Menu":       menu,
		"IsAdmin":    true,
		"IsLoggedIn": false,
	}
}

// The menu and the right-hand controls must not both carry auto margins:
// me-auto on one sibling and ms-auto on another cancels out and glues the
// menu to the logo. This moved from a Go string into partials/header.html, and
// the regression it guards is invisible until the page is looked at.
func TestHeaderPartialAutoMarginBalance(t *testing.T) {
	data := headerData(hostileMenu)
	data["IsLoggedIn"] = true
	set := partialSet(t, "menu", "header")
	out := renderPartial(t, set, "header", data)

	if !strings.Contains(out, `class="navbar-nav me-auto"`) {
		t.Error("menu list lost its me-auto")
	}
	// Exactly one ms-auto, so the auto margins cannot cancel.
	if n := strings.Count(out, "ms-auto"); n != 1 {
		t.Errorf("found %d ms-auto margins, want exactly 1", n)
	}
	if strings.Contains(out, `class="navbar-nav ms-auto"`) {
		t.Error("controls still emitted as a second navbar-nav list")
	}
}

// The header is rendered inline into every page now, so the account controls
// have to honour the logged-in and registration flags. The admin link is the
// one that matters most: it must not appear for anyone who is not an admin,
// because it points at the whole admin panel.
func TestHeaderPartialAccountControls(t *testing.T) {
	set := partialSet(t, "menu", "header")

	anon := headerData(MenuData{RegistrationOpen: false})
	anon["IsAdmin"] = false
	anon["IsLoggedIn"] = false
	out := renderPartial(t, set, "header", anon)
	if !strings.Contains(out, `href="/login"`) {
		t.Error("login control missing for anonymous visitors")
	}
	if strings.Contains(out, `href="/register"`) {
		t.Error("register link shown while registration is disabled")
	}
	if strings.Contains(out, `href="/admin"`) || strings.Contains(out, `href="/account"`) {
		t.Error("admin or account link shown to an anonymous visitor")
	}
	if strings.Contains(out, `href="/clear-cache"`) {
		t.Error("clear-cache must be a button, not a link")
	}

	member := headerData(MenuData{RegistrationOpen: true})
	member["IsLoggedIn"] = true
	out = renderPartial(t, set, "header", member)
	if !strings.Contains(out, `href="/account"`) || !strings.Contains(out, "/logout") {
		t.Error("account/logout controls missing for a signed-in user")
	}
	if !strings.Contains(out, `href="/admin"`) {
		t.Error("admin link missing for an admin, who should see the dashboard link")
	}

	// A signed-in non-admin must not get the admin link even though the
	// account controls are present.
	plain := headerData(MenuData{RegistrationOpen: true})
	plain["IsAdmin"] = false
	plain["IsLoggedIn"] = true
	out = renderPartial(t, set, "header", plain)
	if strings.Contains(out, `href="/admin"`) {
		t.Error("admin link shown to a signed-in non-admin")
	}
	if !strings.Contains(out, `href="/account"`) {
		t.Error("account link missing for a signed-in non-admin")
	}
}
