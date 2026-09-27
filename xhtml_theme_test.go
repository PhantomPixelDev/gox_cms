package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"goxcms/handler"
	"goxcms/model"

	"github.com/gofiber/fiber/v2"
)

// The xhtml theme claims that browsers parse it as XML, so any mistake is a
// hard parse error rather than a silently broken layout. That claim is only
// worth something if the markup that actually ships is well formed, so this
// renders every public page through the real engine and parses the response
// with a strict XML parser.
//
// This is deliberately an end-to-end test rather than a file check. The markup
// only exists after the Go template has run, so a stray {{ end }} or a missing
// close tag is invisible in the source and obvious in the output.
func TestXHTMLThemeOutputIsWellFormedXML(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	// Content of every kind, so each template is exercised with real data
	// rather than the empty state.
	//
	// The content goes through the real sanitizers first, because that is the
	// only way it ever reaches the database. This matters here: bluemonday and
	// HTMLEscapeString both escape a bare & to &amp;, and an unescaped & is a
	// fatal XML error. Inserting raw text straight into the database, as an
	// earlier version of this test did, produces markup the CMS can never
	// actually serve.
	base := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	post := model.Post{
		Title: "XHTML & <Markup>", Slug: "xhtml-post",
		Content:   handlers.SanitizeRichHTML(`<p>Body with an & ampersand</p>`),
		Published: true, CreatedAt: base, ImageURL: "/static/img/x.png",
	}
	if err := db.Create(&post).Error; err != nil {
		t.Fatalf("create post: %v", err)
	}
	cat := model.Category{Name: "Notes & Ideas", Slug: "notes"}
	db.Create(&cat)
	db.Exec("INSERT INTO post_categories (post_id, category_id) VALUES (?, ?)", post.ID, cat.ID)
	tag := model.Tag{Name: "Go & Tools", Slug: "go-tools"}
	db.Create(&tag)
	db.Exec("INSERT INTO post_tags (post_id, tag_id) VALUES (?, ?)", post.ID, tag.ID)

	comment := model.Comment{
		PostID: post.ID, Content: handlers.SanitizeText("A comment with <b>markup</b> & an ampersand"),
		Status: "approved", UserID: admin.ID,
	}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("create comment: %v", err)
	}

	// 25 posts so the paginators render, including the ellipsis markers.
	for i := 0; i < 25; i++ {
		p := model.Post{
			Title: fmt.Sprintf("Bulk %02d", i), Slug: fmt.Sprintf("bulk-%02d", i),
			Content: "body", Published: true, CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		if err := db.Create(&p).Error; err != nil {
			t.Fatalf("create post: %v", err)
		}
	}

	for _, layout := range []string{"page", "page_sidebar", "page_fullwidth"} {
		if err := db.Model(&model.CustomPage{}).Where("1 = 1").Delete(nil).Error; err != nil {
			t.Fatalf("clear pages: %v", err)
		}
		page := model.CustomPage{
			Title: "About & Contact", Slug: "about",
			Content: "<p>Say hello.</p>", Published: true, Template: layout,
		}
		if err := db.Create(&page).Error; err != nil {
			t.Fatalf("create page: %v", err)
		}

		form := url.Values{"name": {"GoX CMS"}, "site_template": {"xhtml"}, "container_class": {"container"}}
		if resp, _ := postForm(t, app, "/update-settings", form, token, auth); resp.StatusCode != fiber.StatusOK {
			t.Fatalf("%s: switch to xhtml: %d", layout, resp.StatusCode)
		}

		paths := []string{
			"/", "/blog", "/blog/2", "/blog/post/xhtml-post",
			"/blog/category/notes", "/blog/tag/go-tools",
			"/search?q=Bulk", "/about", "/no-such-page-here",
		}
		for _, path := range paths {
			resp, body := do(t, app, "GET", path, auth)
			if resp.StatusCode >= 500 {
				t.Errorf("%s %s: status %d", layout, path, resp.StatusCode)
				continue
			}
			if resp.StatusCode == fiber.StatusNotFound {
				// The 404 page is a whole document, so it must still parse.
				assertWellFormedXML(t, path, body)
				continue
			}
			assertWellFormedXML(t, path, body)
		}
	}
}

// assertWellFormedXML parses a rendered page. On failure it reports the line,
// because "XML syntax error" alone does not say where to look.
func assertWellFormedXML(t *testing.T, path, body string) {
	t.Helper()

	// The XML prologue is optional for a parser but the doctype is not, and
	// this is precisely the thing under test, so parse what was served.
	decoder := xml.NewDecoder(strings.NewReader(body))
	decoder.Strict = true

	var sawRoot bool
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			line := 0
			if se, ok := err.(*xml.SyntaxError); ok {
				line = se.Line
			}
			lines := strings.Split(body, "\n")
			near := ""
			if line > 0 && line <= len(lines) {
				near = strings.TrimSpace(lines[line-1])
				if len(near) > 150 {
					near = near[:150]
				}
			}
			t.Errorf("%s: served markup is not well-formed XML: %v\n  line %d: %s", path, err, line, near)
			return
		}
		if _, ok := tok.(xml.StartElement); ok {
			sawRoot = true
		}
	}
	if !sawRoot {
		t.Errorf("%s: response has no XML elements at all (%d bytes)", path, len(body))
	}
}

// The theme must actually be XHTML, not HTML wearing an XHTML DOCTYPE.
func TestXHTMLThemeServedAsXHTML(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	form := url.Values{"name": {"GoX CMS"}, "site_template": {"xhtml"}, "container_class": {"container"}}
	if resp, _ := postForm(t, app, "/update-settings", form, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("switch to xhtml: %d", resp.StatusCode)
	}

	_, body := do(t, app, "GET", "/", auth)

	// The doctype is the whole point: it is what makes a browser parse the
	// document as XML instead of HTML.
	if !strings.Contains(body, `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN"`) {
		t.Errorf("xhtml theme is not declaring the XHTML 1.0 Strict doctype")
	}
	// The namespace is what makes XHTML elements XHTML rather than unknown.
	if !strings.Contains(body, `xmlns="http://www.w3.org/1999/xhtml"`) {
		t.Error("xhtml theme is missing the XHTML namespace on the root element")
	}
	// Void elements must be self-closed, or the XML parse fails outright.
	if strings.Contains(body, "<meta charset=\"utf-8\">") {
		t.Error("found an unclosed <meta>; XHTML requires <meta ... />")
	}
	// Named entities are not defined in XML without a DTD load, and browsers
	// do not load it, so they are fatal.
	for _, bad := range []string{"&hellip;", "&middot;", "&mdash;", "&ndash;", "&rsquo;"} {
		if strings.Contains(body, bad) {
			t.Errorf("found %q, which is not valid in XML; use a numeric reference", bad)
		}
	}
	// And it must still be framework-free, like every site theme.
	if strings.Contains(body, "bootstrap") {
		t.Error("xhtml theme loaded Bootstrap; site themes are meant to be framework-free")
	}
}

// The theme list in the admin UI has to include every discovered theme, or an
// author cannot tell a theme they forgot a file in from one that is not being
// seen at all.
func TestXHTMLThemeIsDiscoverable(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	resp, body := do(t, app, "GET", "/admin-settings", auth)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET /admin-settings: status %d", resp.StatusCode)
	}
	for _, want := range []string{"default", "simple", "xhtml"} {
		if !strings.Contains(body, `value="`+want+`"`) {
			t.Errorf("theme %q is not offered in the template set selector", want)
		}
	}
}
