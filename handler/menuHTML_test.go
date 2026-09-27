package handlers

import (
	"strings"
	"testing"

	"goxcms/model"
)

func TestBuildMenuHTML(t *testing.T) {
	menu := model.Menu{ID: 1, Title: "Primary", Primary: true}
	menu.MenuItems = []*model.MenuItem{
		{ID: 1, Title: "Home", Link: "/", Position: 1},
		{ID: 2, Title: `<script>alert(1)</script>`, Link: "/evil?a=1&b=2", Position: 2},
	}

	got := buildMenuHTML(menu, false, false, "/", true)

	if strings.Contains(got, "<script>") {
		t.Error("menu title not escaped")
	}
	if !strings.Contains(got, "/evil?a=1&amp;b=2") {
		t.Error("menu link not escaped")
	}
	if !strings.Contains(got, `class="nav-link active" aria-current="page"`) {
		t.Error("current path not marked active")
	}
	if strings.Contains(got, "</ul></div>") || strings.Count(got, "<ul") != strings.Count(got, "</ul>") {
		t.Error("unbalanced list markup")
	}
	if !strings.Contains(got, `href="/login"`) || !strings.Contains(got, `href="/register"`) {
		t.Error("anonymous controls missing")
	}
	if strings.Contains(got, `href="/clear-cache"`) {
		t.Error("clear-cache must be a button, not a link")
	}

	admin := buildMenuHTML(menu, true, true, "/blog", true)
	if !strings.Contains(admin, "Admin Dashboard") || !strings.Contains(admin, "<button") {
		t.Error("admin controls missing or not buttons")
	}

	closed := buildMenuHTML(menu, false, false, "/", false)
	if strings.Contains(closed, `href="/register"`) {
		t.Error("register link shown while registration is disabled")
	}
}

// The navbar must not pin a colour palette: hardcoding navbar-light/bg-light
// left a white navbar on a dark page with unreadable menu links. A bare
// .navbar follows data-bs-theme, so no colour classes are emitted.
func TestNavbarClassForTheme(t *testing.T) {
	for _, theme := range []string{"darkly", "flatly", ""} {
		if got := navbarClassForTheme(theme); got != "" {
			t.Errorf("navbarClassForTheme(%q) = %q, want empty", theme, got)
		}
	}
}

// Dark Bootswatch themes start in dark mode; light ones start light.
func TestInitialTheme(t *testing.T) {
	for _, theme := range []string{"darkly", "cyborg", "slate", "vapor", "superhero", "solar"} {
		if got := initialTheme(theme); got != "dark" {
			t.Errorf("initialTheme(%q) = %q, want dark", theme, got)
		}
	}
	for _, theme := range []string{"flatly", "litera", ""} {
		if got := initialTheme(theme); got != "light" {
			t.Errorf("initialTheme(%q) = %q, want light", theme, got)
		}
	}
}

// The menu and the right-hand controls must not both carry auto margins:
// me-auto on one sibling and ms-auto on another cancels out and glues the
// menu to the logo.
func TestMenuHTMLControlGroupLayout(t *testing.T) {
	menu := model.Menu{ID: 1, Title: "Main"}
	menu.MenuItems = []*model.MenuItem{{ID: 1, Title: "Home", Link: "/", Position: 1}}

	html := buildMenuHTML(menu, true, true, "/", false)

	if !strings.Contains(html, `class="navbar-nav me-auto"`) {
		t.Error("menu list lost its me-auto")
	}
	if !strings.Contains(html, `ms-auto ms-lg-3`) {
		t.Error("right-hand control group is missing")
	}
	// Exactly one ms-auto, so the auto margins cannot cancel.
	if n := strings.Count(html, "ms-auto"); n != 1 {
		t.Errorf("found %d ms-auto margins, want exactly 1", n)
	}
	// Controls are grouped, not loose siblings.
	if strings.Contains(html, `class="navbar-nav ms-auto"`) {
		t.Error("controls still emitted as a second navbar-nav list")
	}
	if !strings.Contains(html, `href="/account"`) || !strings.Contains(html, `href="/admin"`) {
		t.Error("account/admin controls missing")
	}
}
