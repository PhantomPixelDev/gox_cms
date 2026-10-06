package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"goxcms/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Site themes.
//
// A theme is a directory of public view templates, discovered from disk rather
// than declared in Go, so a user can create one by dropping files in
// views/site/<name>/ (or via git) without touching the codebase:
//
//	views/site/<name>/theme.json          optional manifest
//	views/site/<name>/layout.html          required
//	views/site/<name>/index.html           required
//	views/site/<name>/search.html          required
//	views/site/<name>/404.html             required
//	views/site/<name>/blog/blog.html       required
//	views/site/<name>/blog/blog_post.html  required
//	views/site/<name>/blog/blog_category.html
//	views/site/<name>/blog/blog_tag.html
//	views/site/<name>/page/page.html
//	views/site/<name>/page/page_sidebar.html
//	views/site/<name>/page/page_fullwidth.html
//
// Optional per-theme assets live in static/themes/<name>/ and are served
// straight from the existing static mount (the CSP already allows same-origin
// CSS/JS/images/fonts).
//
// This is separate from BasicWebsiteInfo.Theme, which is inert: the
// Bootswatch bundles it selected were removed in favour of a single
// vendored Bootstrap with a brand-color override.
const (
	// SiteTemplateDefault is the fallback when no theme is set or the stored
	// one has disappeared. It is also the name of the shipped Bootstrap theme.
	SiteTemplateDefault = "default"
	// ThemeDir is the directory scanned for themes, relative to the views root.
	ThemeDir = "site"
	// ThemeAssetDir is where a theme may ship its own assets, relative to the
	// static root. Not scanned; purely a convention used by the layout.
	ThemeAssetDir = "themes"
	// ThemeLayout is the layout file a theme must provide.
	ThemeLayout = "layout.html"
	// PublicLayoutFile is the shared layout at the views root. It is the
	// default theme's layout, and the one the auth screens use.
	PublicLayoutFile = "main.html"
	// ThemeManifest is the optional per-theme metadata file.
	ThemeManifest = "theme.json"
)

// RequiredThemeTemplates are the templates a theme must ship. A theme that is
// missing any of them cannot be activated, which is what makes "must be
// complete" a safe rule rather than a trap: the admin UI reports exactly which
// files are absent rather than the site 500-ing on a missing template.
//
// Exported so docs/THEMES.md can be tested against this list instead of
// restating it, since a guide that drifts from the code is worse than none.
var RequiredThemeTemplates = []string{
	ThemeLayout,
	"index.html",
	"search.html",
	"404.html",
	"blog/blog.html",
	"blog/blog_post.html",
	"blog/blog_category.html",
	"blog/blog_tag.html",
	"page/page.html",
	"page/page_sidebar.html",
	"page/page_fullwidth.html",
}

// ThemeManifestData is the optional theme.json.
type ThemeManifestData struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Author      string `json:"author"`
	Description string `json:"description"`
	Framework   string `json:"framework"`
}

// ThemeInfo describes one discovered theme.
type ThemeInfo struct {
	// Name is the directory name, and the value stored in the setting.
	Name string
	// Label is the human-facing name from the manifest, or the directory name.
	Label       string
	Version     string
	Author      string
	Description string
	// Framework is free text for the admin list, e.g. "bootstrap" or
	// "none". Never used for rendering.
	Framework string
	// Complete is true when every required template is present and parses.
	Complete bool
	// Missing lists absent required files.
	Missing []string
	// ParseErr is the first template parse failure, if any.
	ParseErr string
	// Active marks the theme the site is currently using.
	Active bool
}

// viewsRoot is the directory the template engine was configured with. Set once
// at startup; theme discovery is relative to it.
//
// Guarded by viewsMu because the test suite builds more than one app in one
// process, each with its own template root.
var (
	viewsMu   sync.RWMutex
	viewsRoot = utils.ViewsDir
)

// SetViewsRoot records where templates live so themes can be discovered.
func SetViewsRoot(dir string) {
	viewsMu.Lock()
	viewsRoot = dir
	viewsMu.Unlock()
}

// siteDir returns the current template root.
func siteDir() string {
	viewsMu.RLock()
	defer viewsMu.RUnlock()
	return viewsRoot
}

// ReloadThemes is called after a theme is activated, so a theme activated in
// this request is immediately reported as active.
//
// There is no longer a discovery cache. There used to be a 15 second one, on
// the grounds that discovery reads the filesystem and parses templates. But
// discovery is only called from the admin settings page and from theme
// activation, never on a public render, so it was never on a hot path: the
// cache saved nothing and cost correctness. A theme created by editing files
// did not appear for up to 15 seconds, which is exactly the surprise the
// filesystem approach is supposed to remove.
func ReloadThemes() {}

// InitThemeSystem points discovery at the template root. Call once at startup,
// before the first render.
func InitThemeSystem() {
	SetViewsRoot(utils.ViewsDir)
}

// DiscoverThemes scans ThemeDir and returns every theme found, with the active
// one flagged. Themes that are incomplete or do not parse are included and
// flagged, not hidden: an author needs to be told why their theme is not
// offered, and a silently absent theme is indistinguishable from one that was
// never seen.
func DiscoverThemes(active string) []ThemeInfo {
	return markActive(scanThemes(), active)
}

func markActive(themes []ThemeInfo, active string) []ThemeInfo {
	out := make([]ThemeInfo, len(themes))
	copy(out, themes)
	for i := range out {
		out[i].Active = out[i].Name == active
	}
	return out
}

// scanThemes walks views/site/* and validates each candidate.
func scanThemes() []ThemeInfo {
	root := filepath.Join(siteDir(), ThemeDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []ThemeInfo
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if !hasThemeLayout(e.Name()) {
			continue
		}
		out = append(out, inspectTheme(root, e.Name()))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// hasThemeLayout reports whether a theme has the layout it needs.
//
// The default theme predates per-theme layouts: its shell is the shared
// views/main.html, which the login, register and account screens also use. That
// stays as it is, so the default theme is accepted without a layout of its own.
func hasThemeLayout(name string) bool {
	if _, err := os.Stat(filepath.Join(siteDir(), ThemeDir, name, ThemeLayout)); err == nil {
		return true
	}
	if name == SiteTemplateDefault {
		_, err := os.Stat(filepath.Join(siteDir(), PublicLayoutFile))
		return err == nil
	}
	return false
}

// inspectTheme checks one theme directory for completeness and parseability.
func inspectTheme(root, name string) ThemeInfo {
	info := ThemeInfo{Name: name, Label: name, Complete: true}
	dir := filepath.Join(root, name)

	// Optional manifest; a malformed one is a warning, not a rejection.
	if raw, err := os.ReadFile(filepath.Join(dir, ThemeManifest)); err == nil {
		var m ThemeManifestData
		if err := json.Unmarshal(raw, &m); err == nil {
			if m.Name != "" {
				info.Label = m.Name
			}
			info.Version = m.Version
			info.Author = m.Author
			info.Description = m.Description
			info.Framework = m.Framework
		}
	}

	for _, rel := range RequiredThemeTemplates {
		if rel == ThemeLayout && name == SiteTemplateDefault {
			// The default theme uses the shared public layout.
			if hasThemeLayout(name) {
				continue
			}
		}
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			info.Complete = false
			info.Missing = append(info.Missing, rel)
		}
	}
	if len(info.Missing) > 0 {
		// No point parsing a theme that cannot be used.
		return info
	}

	if err := parseTheme(root, name, dir); err != nil {
		info.Complete = false
		info.ParseErr = err.Error()
	}
	return info
}

// themeTemplateRef matches {{template "name"}} in a theme file, capturing the
// referenced template name.
var themeTemplateRef = regexp.MustCompile(`\{\{-?\s*template\s+"([^"]+)"`)

// validationFuncs is the template function set used to parse-check a theme.
//
// It is the engine's set plus a stub for "embed". embed is not a normal
// function: the engine injects it per render as the layout's content hook, so a
// layout template that uses it is correct but cannot be parsed without a stub.
func validationFuncs() template.FuncMap {
	fm := utils.TemplateFuncMap()
	fm["embed"] = func() (string, error) { return "", nil }
	return fm
}

// parseTheme parses a theme's files together with the shared partials.
//
// Parsing each file on its own would reject any theme that includes a partial,
// because a standalone template cannot resolve {{template "partials/header"}}
// — the engine only errors on that at execution time, so this has to be at
// least as permissive. It also checks that every referenced template actually
// exists, which is the single most common way a hand-written theme breaks.
func parseTheme(root, name, dir string) error {
	set := template.New("__validate__").Funcs(validationFuncs())

	// Shared partials first, so a theme can include them by name.
	partialsRoot := filepath.Join(siteDir(), "partials")
	known := map[string]bool{}
	_ = filepath.WalkDir(partialsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		// Named without the extension, because that is how templates are
		// referenced ({{template "partials/header"}}) and how the engine
		// names them.
		tname := "partials/" + strings.TrimSuffix(relativeThemePath(partialsRoot, path), ".html")
		known[tname] = true
		if _, perr := set.New(tname).Parse(string(raw)); perr != nil {
			return fmt.Errorf("shared partial %s: %v", tname, perr)
		}
		return nil
	})

	for _, rel := range RequiredThemeTemplates {
		tname := name + "/" + rel
		known[tname] = true
	}
	known[name] = true

	for _, rel := range RequiredThemeTemplates {
		raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			// The default theme's layout lives at the views root.
			if rel == ThemeLayout && name == SiteTemplateDefault {
				raw, err = os.ReadFile(filepath.Join(siteDir(), PublicLayoutFile))
			}
			if err != nil {
				return fmt.Errorf("%s: %v", rel, err)
			}
		}
		tname := name + "/" + rel
		if _, err := set.New(tname).Parse(string(raw)); err != nil {
			return fmt.Errorf("%s: %v", rel, err)
		}
		// A missing include parses fine and only fails at render time, which
		// on a live site means a 500. Catch it here instead.
		for _, m := range themeTemplateRef.FindAllStringSubmatch(string(raw), -1) {
			ref := m[1]
			if !known[ref] && set.Lookup(ref) == nil {
				return fmt.Errorf("%s includes %q, which does not exist", rel, ref)
			}
		}
	}
	return nil
}

// relativeThemePath returns a slash-separated path relative to base.
func relativeThemePath(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// ThemeExists reports whether a theme with this name was discovered and is
// complete enough to activate.
func ThemeExists(name string) bool {
	for _, t := range DiscoverThemes("") {
		if t.Name == name {
			return t.Complete
		}
	}
	return false
}

// ThemeByName returns the discovered theme, or false.
func ThemeByName(name string) (ThemeInfo, bool) {
	for _, t := range DiscoverThemes("") {
		if t.Name == name {
			return t, true
		}
	}
	return ThemeInfo{}, false
}

// SiteTemplateSet normalises a stored theme name against what is on disk.
//
// A stored value whose directory is gone falls back to default so a deleted
// theme cannot take the site down, but an unknown name is still reported by
// ValidateTheme when an admin tries to activate it.
func SiteTemplateSet(name string) string {
	if name == "" {
		return SiteTemplateDefault
	}
	dir := filepath.Join(siteDir(), ThemeDir, name, ThemeLayout)
	if _, err := os.Stat(dir); err != nil {
		return SiteTemplateDefault
	}
	return name
}

// ValidateTheme returns a friendly error if the theme cannot be used, or "" if
// it is fine. Called before saving the setting so an incomplete theme is
// rejected with a reason instead of 500-ing every public page.
func ValidateTheme(name string) string {
	if name == "" {
		return ""
	}
	info, ok := ThemeByName(name)
	if !ok {
		return "No theme directory named \"" + name + "\" in views/" + ThemeDir +
			". Create it, or pick another theme."
	}
	if len(info.Missing) > 0 {
		return "Theme \"" + name + "\" is missing " + strings.Join(info.Missing, ", ")
	}
	if info.ParseErr != "" {
		return "Theme \"" + name + "\" has a template error: " + info.ParseErr
	}
	return ""
}

// SiteView resolves a public template name for a theme, e.g.
// ("simple", "blog/blog_post") -> "site/simple/blog/blog_post".
func SiteView(set, name string) string {
	return ThemeDir + "/" + SiteTemplateSet(set) + "/" + name
}

// SharedView is the template name of a shared partial under views/partials.
//
// Shared partials are not part of a theme, so they resolve from the views root
// rather than the theme directory. RenderFragment needs this: a fragment named
// "partials/menu" is a shared partial, and going through SiteView would look
// for views/site/<theme>/partials/menu, which no theme has.
func SharedView(name string) string {
	return "partials/" + name
}

// RenderFragment renders a shared partial with NO layout, for HTMX swaps.
func RenderSharedFragment(c *fiber.Ctx, name string, data fiber.Map) error {
	return c.Render(SharedView(name), themeContext(c, data))
}

// SiteLayout returns the layout template name for a theme. Each theme brings its
// own shell, so a framework-free theme does not inherit the Bootstrap navbar.
func SiteLayout(set string) string {
	set = SiteTemplateSet(set)
	if set == SiteTemplateDefault {
		// The default theme's layout lives at the views root, which is the
		// name the auth screens and the old tests already use.
		return "main"
	}
	return SiteView(set, "layout")
}

// ThemeAssetBase is the URL prefix a theme's own assets live under.
func ThemeAssetBase(set string) string {
	return "/static/" + ThemeAssetDir + "/" + SiteTemplateSet(set)
}

// activeThemeName reads the theme from the request's settings locals.
func activeThemeName(c *fiber.Ctx) string {
	if settings, ok := c.Locals("Settings").(map[string]string); ok {
		return settings["SiteTemplate"]
	}
	return ""
}

// themeContext adds the values every theme template can rely on: the theme's
// own name and asset base, and the site settings. Keeps the contract stable so
// a theme never has to guess a path.
func themeContext(c *fiber.Ctx, data fiber.Map) fiber.Map {
	if data == nil {
		data = fiber.Map{}
	}
	set := SiteTemplateSet(activeThemeName(c))
	data["Theme"] = fiber.Map{
		"Name":   set,
		"Label":  set,
		"Assets": ThemeAssetBase(set),
		"View":   SiteView(set, ""),
	}
	if _, ok := data["Settings"]; !ok {
		data["Settings"] = c.Locals("Settings")
	}
	// The navigation tree is attached here, once, so no handler has to
	// remember to pass it. Layouts reach it as {{ .Menu }}.
	if _, ok := data["Menu"]; !ok {
		if d, dok := c.Locals("db").(*gorm.DB); dok && d != nil {
			data["Menu"] = BuildMenuData(d, c)
		} else {
			data["Menu"] = MenuData{Items: []MenuNode{}}
		}
	}
	return data
}

// RenderSite renders a public view from the site's active theme, with that
// theme's layout.
func RenderSite(c *fiber.Ctx, name string, data fiber.Map) error {
	set := SiteTemplateSet(activeThemeName(c))
	return c.Render(SiteView(set, name), themeContext(c, data), SiteLayout(set))
}

// RenderFragment renders a public view with NO layout, for HTMX swaps.
//
// hx-get on a normal page URL injects a second <html> document into the page,
// which is why themes could not do partial updates. Fragments are the supported
// way: GET /frag/blog?page=2 renders the theme's blog list on its own.
func RenderFragment(c *fiber.Ctx, name string, data fiber.Map) error {
	set := SiteTemplateSet(activeThemeName(c))
	return c.Render(SiteView(set, name), themeContext(c, data))
}

// RenderNotFound renders the 404 view from the active theme. Handlers that
// answer 404 directly must use this rather than naming a template, or a site on
// a theme without a 404.html would 500 where a 404 was intended.
func RenderNotFound(c *fiber.Ctx) error {
	c.Status(fiber.StatusNotFound)
	return RenderSite(c, "404", fiber.Map{
		"Title":      "404 - Page Not Found",
		"IsLoggedIn": c.Locals("isLoggedin"),
		"IsAdmin":    c.Locals("isAdmin"),
	})
}

// RenderFragmentNotFound is the 404 for a fragment request.
//
// It renders the theme's 404 view with no layout, because RenderNotFound
// returns a whole document. A fragment whose slug does not exist used to answer
// with a full <html> page, which is exactly the thing fragments exist to avoid:
// htmx would splice an entire document into whatever panel the request came
// from. The status is still 404, so the calling code and any monitoring can see
// it.
func RenderFragmentNotFound(c *fiber.Ctx) error {
	c.Status(fiber.StatusNotFound)
	return RenderFragment(c, "404", fiber.Map{
		"Title":      "404 - Page Not Found",
		"IsLoggedIn": c.Locals("isLoggedin"),
		"IsAdmin":    c.Locals("isAdmin"),
	})
}
