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

	got := buildMenuHTML(menu, false, false, "/")

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

	admin := buildMenuHTML(menu, true, true, "/blog")
	if !strings.Contains(admin, "Admin Dashboard") || !strings.Contains(admin, "<button") {
		t.Error("admin controls missing or not buttons")
	}
}

func TestNavbarClassForTheme(t *testing.T) {
	if got := navbarClassForTheme("darkly"); got != "navbar-dark bg-dark" {
		t.Errorf("darkly = %q", got)
	}
	if got := navbarClassForTheme("flatly"); got != "navbar-light bg-light" {
		t.Errorf("flatly = %q", got)
	}
	if got := navbarClassForTheme(""); got != "navbar-light bg-light" {
		t.Errorf("empty theme = %q", got)
	}
}
