package handlers

import (
	"github.com/gofiber/fiber/v2"
)

// Site template sets. Each is a directory of public view templates under
// views/site/<name>/, so switching a site's template set swaps the markup of
// every public page, not just its colours.
//
// This is separate from BasicWebsiteInfo.Theme, which only selects a
// Bootswatch stylesheet.
//
//	default - the Bootstrap 5 + Bootswatch set (site/<set>/bootstrap/...)
//	simple  - framework-free: plain HTML with one small stylesheet
const (
	SiteTemplateDefault = "default"
	SiteTemplateSimple  = "simple"
)

// siteTemplateSets maps the stored value to its views directory and layout.
// Only whitelisted names are honoured, so a hand-edited database value can
// never make the renderer point at a missing or unintended template.
var siteTemplateSets = map[string]struct {
	views  string
	layout string
}{
	SiteTemplateDefault: {views: "site/default", layout: "main"},
	SiteTemplateSimple:  {views: "site/simple", layout: "site/simple/layout"},
}

// SiteTemplateSets lists the selectable values, for the settings form.
func SiteTemplateSets() []string {
	return []string{SiteTemplateDefault, SiteTemplateSimple}
}

// SiteTemplateSet normalises the stored setting, defaulting to "default" when
// it is empty or unknown (including for rows created before the column
// existed).
func SiteTemplateSet(name string) string {
	if _, ok := siteTemplateSets[name]; ok {
		return name
	}
	return SiteTemplateDefault
}

// SiteView resolves a public template name for the given set, e.g.
// ("simple", "blog/blog_post") -> "site/simple/blog/blog_post".
func SiteView(set, name string) string {
	return siteTemplateSets[SiteTemplateSet(set)].views + "/" + name
}

// SiteLayout returns the layout name for the given set. Each set brings its
// own shell: the simple set must not inherit the Bootstrap navbar and footer.
func SiteLayout(set string) string {
	return siteTemplateSets[SiteTemplateSet(set)].layout
}

// RenderNotFound renders the 404 view from the site's template set. Handlers
// that answer 404 directly must use this rather than naming a template, or a
// site on the "simple" set would get a missing-template error (a 500) where a
// 404 was intended.
func RenderNotFound(c *fiber.Ctx) error {
	c.Status(fiber.StatusNotFound)
	return RenderSite(c, "404", fiber.Map{
		"Title":    "404 - Page Not Found",
		"Settings": c.Locals("Settings"),
	})
}

// RenderSite renders a public view from the template set configured in the
// site's settings, using that set's layout.
func RenderSite(c *fiber.Ctx, name string, data fiber.Map) error {
	set := SiteTemplateDefault
	if settings, ok := c.Locals("Settings").(map[string]string); ok {
		set = settings["SiteTemplate"]
	}
	set = SiteTemplateSet(set)
	return c.Render(SiteView(set, name), data, SiteLayout(set))
}
