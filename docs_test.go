package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	handlers "goxcms/handler"
	"goxcms/model"
)

// docsPath is the theme authoring guide.
const docsPath = "docs/THEMES.md"

// The guide tells theme authors which templates a theme must provide. If that
// list drifts from the code, authors build a theme that cannot be activated and
// the documentation is worse than none. This keeps the two in step.
func TestDocsRequiredTemplateListMatchesCode(t *testing.T) {
	doc, err := os.ReadFile(docsPath)
	if err != nil {
		t.Fatalf("read %s: %v", docsPath, err)
	}
	text := string(doc)

	// Every required template must be named in the guide.
	for _, rel := range handlers.RequiredThemeTemplates {
		if !strings.Contains(text, rel) {
			t.Errorf("%s does not mention the required template %s", docsPath, rel)
		}
	}

	// And every fragment endpoint must be documented, or an author cannot
	// discover the one they need.
	for _, route := range fragmentRoutes(t) {
		if !strings.Contains(text, route) {
			t.Errorf("%s does not document the fragment endpoint %s", docsPath, route)
		}
	}
}

// fragmentRoutes returns the /frag/ endpoints registered in the source, so the
// guide is checked against the routes rather than against itself.
func fragmentRoutes(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("routes", "fragments.go"))
	if err != nil {
		t.Fatalf("read routes/fragments.go: %v", err)
	}
	re := regexp.MustCompile(`app\.Get\("(/frag/[^"]*)"`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		// Documented without the parameter placeholders.
		out = append(out, strings.SplitN(m[1], ":", 2)[0])
	}
	if len(out) == 0 {
		t.Fatal("no /frag/ routes found; the test is not checking anything")
	}
	return out
}

// The fragment endpoints must actually respond. A route that 500s is worse than
// one that is missing, because a theme will happily use it.
func TestEveryDocumentedFragmentResponds(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	paths := []string{
		"/frag/menu",
		"/frag/blog",
		"/frag/blog?page=1",
		"/frag/search?q=a",
		// These three need a slug that does not exist, and must answer 404
		// rather than 500.
		"/frag/post/nope",
		"/frag/comments/nope",
		"/frag/page/nope",
		"/frag/blog/category/nope",
		"/frag/blog/tag/nope",
	}
	for _, p := range paths {
		resp, body := do(t, app, "GET", p, auth)
		if resp.StatusCode >= 500 {
			t.Errorf("GET %s: status %d, want a 2xx or 404", p, resp.StatusCode)
			continue
		}
		// A fragment must never be a whole document, whatever the status.
		lower := strings.ToLower(body)
		if strings.Contains(lower, "<html") || strings.Contains(lower, "<!doctype") {
			t.Errorf("GET %s returned a whole page, so it cannot be swapped into one", p)
		}
		// An hx-get that gets a 302 swaps the redirect target's body in.
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			t.Errorf("GET %s: redirects, which breaks an hx-get swap", p)
		}
	}
}
