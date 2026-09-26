package main

import (
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"goxcms/database"
	handlers "goxcms/handler"
	"goxcms/model"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const testSecret = "test-secret-0123456789abcdef0123456789"

// newTestApp builds the full application against a fresh in-memory database.
func newTestApp(t *testing.T) (*fiber.App, *gorm.DB) {
	t.Helper()

	viper.Reset()
	viper.Set("build.mode", "development")
	viper.Set("database.driver", "sqlite")
	viper.Set("database.sqlite.dsn", "file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared")
	viper.Set("app.secret", testSecret)
	viper.Set("app.url", "http://localhost:3000")
	viper.Set("server.body_limit", 10)
	viper.Set("captcha.enabled", false)
	viper.Set("ratelimiter.enabled", false)
	viper.Set("cors.allowed_origins", []string{"http://localhost:3000"})

	db := database.InitDB()
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	return setupFiberApp(db), db
}

func createUser(t *testing.T, db *gorm.DB, username string, role uint) model.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := model.User{Username: username, Password: string(hash), RoleID: role, FirstName: "Test", LastName: "User"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func authCookie(t *testing.T, userID uint) *http.Cookie {
	t.Helper()
	// Fresh test users have session version 0.
	token, err := handlers.GenerateJWT(userID, 0)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "jwt", Value: token}
}

func do(t *testing.T, app *fiber.App, method, path string, cookies ...*http.Cookie) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(body)
}

func isDenied(status int) bool {
	return status == fiber.StatusFound || status == fiber.StatusUnauthorized || status == fiber.StatusForbidden
}

var adminRoutes = []struct{ method, path string }{
	{"GET", "/admin"},
	{"GET", "/admin-settings"},
	{"GET", "/admin/post/add"},
	{"POST", "/admin/post/add"},
	{"GET", "/admin/post/edit/1"},
	{"POST", "/admin/post/edit"},
	{"GET", "/add-custompage"},
	{"GET", "/edit-custompage/1"},
	{"GET", "/search-posts"},
	{"GET", "/search-tags"},
	{"GET", "/search-menu"},
	{"GET", "/search-categories"},
	{"GET", "/search-users"},
	{"GET", "/search-comments"},
	{"GET", "/search-custompages"},
	{"GET", "/search-files"},
	{"POST", "/update-settings"},
	{"POST", "/toggle-post-status"},
	{"DELETE", "/delete-post/1"},
	{"DELETE", "/delete-user/1"},
	{"POST", "/upload-file"},
	{"POST", "/admin/plugins/enable/LatestPostsPlugin"},
}

func TestAdminRoutesRejectAnonymous(t *testing.T) {
	app, _ := newTestApp(t)

	for _, r := range adminRoutes {
		resp, _ := do(t, app, r.method, r.path)
		if !isDenied(resp.StatusCode) {
			t.Errorf("anonymous %s %s: got status %d, want 302/401/403", r.method, r.path, resp.StatusCode)
		}
	}
}

func TestAdminRoutesRejectRegularUser(t *testing.T) {
	app, db := newTestApp(t)
	user := createUser(t, db, "regular", model.RoleUser)
	cookie := authCookie(t, user.ID)

	for _, r := range adminRoutes {
		resp, _ := do(t, app, r.method, r.path, cookie)
		if !isDenied(resp.StatusCode) {
			t.Errorf("regular user %s %s: got status %d, want 302/401/403", r.method, r.path, resp.StatusCode)
		}
	}
}

func TestAdminCanOpenAdminPanel(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)

	resp, _ := do(t, app, "GET", "/admin", authCookie(t, admin.ID))
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("admin GET /admin: got status %d, want 200", resp.StatusCode)
	}
}

func TestAdminSettingsRenders(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)

	resp, body := do(t, app, "GET", "/admin-settings", authCookie(t, admin.ID))
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("admin GET /admin-settings: got status %d, want 200", resp.StatusCode)
	}
	// Logo/favicon hold site-relative paths, so they must not be url-typed
	// (browsers block submit on prefilled relative values).
	for _, want := range []string{`id="logo_url"`, `id="favicon_url"`, `id="privacy_policy"`, `id="terms_of_service"`} {
		if !strings.Contains(body, want) {
			t.Errorf("settings form missing %s", want)
		}
	}
	if strings.Contains(body, `id="logo_url"`) && strings.Contains(body, `type="url" class="form-control" id="logo_url"`) {
		t.Error("logo_url must not be type=url")
	}
}

func TestAdminOverviewShowsCounts(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	db.Create(&model.Post{Title: "One", Content: "x", Slug: "one", Published: true})
	db.Create(&model.Post{Title: "Two", Content: "x", Slug: "two"})

	resp, body := do(t, app, "GET", "/admin", authCookie(t, admin.ID))
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("admin GET /admin: got status %d, want 200", resp.StatusCode)
	}
	for _, want := range []string{"Overview", "stat-card", "Pending Comments", "Plugins"} {
		if !strings.Contains(body, want) {
			t.Errorf("admin panel missing %q", want)
		}
	}
}

func TestForgedJWTIsRejected(t *testing.T) {
	sign := func(t *testing.T, method jwt.SigningMethod, key interface{}, claims jwt.MapClaims) string {
		token, err := jwt.NewWithClaims(method, claims).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	exp := func() int64 { return time.Now().Add(time.Hour).Unix() }

	cases := map[string]func(t *testing.T, userID uint) string{
		"empty key": func(t *testing.T, id uint) string {
			return sign(t, jwt.SigningMethodHS256, []byte(""), jwt.MapClaims{"user_id": id, "exp": exp()})
		},
		"wrong key": func(t *testing.T, id uint) string {
			return sign(t, jwt.SigningMethodHS256, []byte("not-the-secret"), jwt.MapClaims{"user_id": id, "exp": exp()})
		},
		"alg none": func(t *testing.T, id uint) string {
			return sign(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, jwt.MapClaims{"user_id": id, "exp": exp()})
		},
		"no expiry": func(t *testing.T, id uint) string {
			return sign(t, jwt.SigningMethodHS256, []byte(testSecret), jwt.MapClaims{"user_id": id})
		},
		"expired": func(t *testing.T, id uint) string {
			return sign(t, jwt.SigningMethodHS256, []byte(testSecret), jwt.MapClaims{"user_id": id, "exp": time.Now().Add(-time.Hour).Unix()})
		},
	}

	for name, forge := range cases {
		t.Run(name, func(t *testing.T) {
			// A fresh app per case, so no state from one request can mask another.
			app, db := newTestApp(t)
			admin := createUser(t, db, "boss", model.RoleAdmin)

			resp, _ := do(t, app, "GET", "/admin", &http.Cookie{Name: "jwt", Value: forge(t, admin.ID)})
			if !isDenied(resp.StatusCode) {
				t.Errorf("got status %d, want the token to be rejected", resp.StatusCode)
			}
		})
	}
}

func TestAdminResponsesAreNotCachedForAnonymousUsers(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)

	for _, path := range []string{"/admin", "/search-users"} {
		if resp, _ := do(t, app, "GET", path, authCookie(t, admin.ID)); resp.StatusCode != fiber.StatusOK {
			t.Fatalf("admin GET %s: got status %d", path, resp.StatusCode)
		}
		resp, body := do(t, app, "GET", path)
		if !isDenied(resp.StatusCode) {
			t.Errorf("anonymous GET %s after admin visit: got status %d", path, resp.StatusCode)
		}
		if strings.Contains(body, "boss") {
			t.Errorf("anonymous GET %s leaked admin content", path)
		}
	}
}

func TestUnpublishedPostIs404ForAnonymous(t *testing.T) {
	app, db := newTestApp(t)
	post := model.Post{Title: "Draft", Content: "secret draft", Slug: "draft", Published: false}
	if err := db.Create(&post).Error; err != nil {
		t.Fatal(err)
	}

	resp, body := do(t, app, "GET", "/blog/post/draft")
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("got status %d, want 404", resp.StatusCode)
	}
	if strings.Contains(body, "secret draft") {
		t.Fatal("unpublished post content leaked")
	}

	// The server must still be serving requests afterwards.
	if resp, _ := do(t, app, "GET", "/"); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET / after draft request: got status %d", resp.StatusCode)
	}
}

// csrfCookie fetches a page to obtain the CSRF cookie that unsafe requests
// must echo back in the X-Csrf-Token header.
func csrfCookie(t *testing.T, app *fiber.App, cookies ...*http.Cookie) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest("GET", "/login", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range resp.Cookies() {
		if c.Name == "csrf_" {
			return c
		}
	}
	t.Fatal("no csrf_ cookie set on GET /login")
	return nil
}

func postForm(t *testing.T, app *fiber.App, path string, form url.Values, csrf *http.Cookie, cookies ...*http.Cookie) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	if csrf != nil {
		req.AddCookie(csrf)
		req.Header.Set("X-Csrf-Token", csrf.Value)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(body)
}

func TestCSRFTokenRequiredForUnsafeRequests(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	post := model.Post{Title: "Hello", Content: "world", Slug: "hello"}
	if err := db.Create(&post).Error; err != nil {
		t.Fatal(err)
	}
	form := url.Values{"id": {strconv.Itoa(int(post.ID))}}

	if resp, _ := postForm(t, app, "/toggle-post-status", form, nil, auth); resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("POST without CSRF token: got status %d, want 403", resp.StatusCode)
	}

	token := csrfCookie(t, app, auth)
	if resp, _ := postForm(t, app, "/toggle-post-status", form, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("POST with CSRF token: got status %d, want 200", resp.StatusCode)
	}

	db.First(&post, post.ID)
	if !post.Published {
		t.Fatal("post was not published")
	}
}

func TestLoginSetsSessionCookie(t *testing.T) {
	app, db := newTestApp(t)
	createUser(t, db, "alice", model.RoleUser)
	token := csrfCookie(t, app)

	resp, _ := postForm(t, app, "/login", url.Values{"username": {"alice"}, "password": {"password123"}}, token)
	if resp.StatusCode != fiber.StatusOK || resp.Header.Get("HX-Redirect") != "/" {
		t.Fatalf("login: got status %d, HX-Redirect %q", resp.StatusCode, resp.Header.Get("HX-Redirect"))
	}

	var jwtCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "jwt" {
			jwtCookie = c
		}
	}
	if jwtCookie == nil || !jwtCookie.HttpOnly {
		t.Fatalf("login did not set an HttpOnly jwt cookie: %+v", jwtCookie)
	}

	resp, _ = postForm(t, app, "/login", url.Values{"username": {"alice"}, "password": {"wrong"}}, token)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("login with wrong password: got status %d, want 401", resp.StatusCode)
	}
}

func TestRegisterWorksWithCaptchaDisabled(t *testing.T) {
	app, db := newTestApp(t)
	token := csrfCookie(t, app)

	form := url.Values{
		"username":   {"newuser"},
		"password":   {"secret123"},
		"first_name": {"New"},
		"last_name":  {"User"},
	}
	resp, body := postForm(t, app, "/register", form, token)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("register: got status %d, body %q", resp.StatusCode, body)
	}

	var user model.User
	if err := db.Where("username = ?", "newuser").First(&user).Error; err != nil {
		t.Fatalf("user not created: %v", err)
	}
	if user.RoleID != model.RoleUser {
		t.Fatalf("new user has role %d, want %d", user.RoleID, model.RoleUser)
	}
}

func TestPostSlugMustBeUnique(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookie(t, app, auth)

	first := model.Post{Title: "First", Content: "one", Slug: "first"}
	second := model.Post{Title: "Second", Content: "two", Slug: "second"}
	db.Create(&first)
	db.Create(&second)
	cat := model.Category{Name: "News", Slug: "news"}
	db.Create(&cat)

	form := url.Values{"title": {"Dup"}, "content": {"x"}, "post_slug": {"first"}, "image": {"/img.png"}}
	postForm(t, app, "/admin/post/add", form, token, auth)
	var count int64
	db.Model(&model.Post{}).Where("slug = ?", "first").Count(&count)
	if count != 1 {
		t.Fatalf("adding a post with a duplicate slug created it (count %d)", count)
	}

	form = url.Values{"id": {strconv.Itoa(int(second.ID))}, "title": {"Second"}, "content": {"two"}, "post_slug": {"first"}, "image": {"/img.png"}}
	postForm(t, app, "/admin/post/edit", form, token, auth)
	db.First(&second, second.ID)
	if second.Slug != "second" {
		t.Fatalf("editing a post to a duplicate slug succeeded")
	}

	form = url.Values{
		"id": {strconv.Itoa(int(second.ID))}, "title": {"Renamed"}, "content": {"two"},
		"post_slug": {"second-renamed"}, "image": {"/img.png"}, "categories_input": {strconv.Itoa(int(cat.ID))},
	}
	if resp, body := postForm(t, app, "/admin/post/edit", form, token, auth); resp.StatusCode != fiber.StatusOK || !strings.Contains(body, "updated") {
		t.Fatalf("valid edit failed: %d %q", resp.StatusCode, body)
	}
	db.Preload("Categories").First(&second, second.ID)
	if second.Title != "Renamed" || second.Slug != "second-renamed" || len(second.Categories) != 1 {
		t.Fatalf("edit not applied: %+v", second)
	}
}

func TestCustomPagesAreServedWithoutRestart(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookie(t, app, auth)

	form := url.Values{"title": {"Contact"}, "content": {"<p>Contact us</p>"}, "slug": {"contact"}, "template": {"page"}}
	postForm(t, app, "/add-custompage", form, token, auth)

	resp, body := do(t, app, "GET", "/contact")
	if resp.StatusCode != fiber.StatusOK || !strings.Contains(body, "Contact us") {
		t.Fatalf("GET /contact: got status %d", resp.StatusCode)
	}

	form = url.Values{"title": {"Evil"}, "content": {"x"}, "slug": {"evil"}, "template": {"../admin/admin"}}
	postForm(t, app, "/add-custompage", form, token, auth)
	var count int64
	db.Model(&model.CustomPage{}).Where("slug = ?", "evil").Count(&count)
	if count != 0 {
		t.Fatal("custom page with a disallowed template was created")
	}

	if resp, _ := do(t, app, "GET", "/does-not-exist"); resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("unknown path: got status %d, want 404", resp.StatusCode)
	}
}

func TestSeedDemoContent(t *testing.T) {
	app, db := newTestApp(t)

	var posts int64
	db.Model(&model.Post{}).Where("published = ?", true).Count(&posts)
	if posts < 3 {
		t.Errorf("seeded published posts = %d, want >= 3", posts)
	}

	var menu model.Menu
	if err := db.Preload("MenuItems").Where("is_primary = ?", true).First(&menu).Error; err != nil {
		t.Fatalf("no primary menu seeded: %v", err)
	}
	if len(menu.MenuItems) < 3 {
		t.Errorf("primary menu items = %d, want >= 3", len(menu.MenuItems))
	}

	var files int64
	db.Model(&model.File{}).Count(&files)
	if files < 3 {
		t.Errorf("seeded files = %d, want >= 3", files)
	}

	// Seeded pages are live without restart.
	if resp, body := do(t, app, "GET", "/about"); resp.StatusCode != fiber.StatusOK || !strings.Contains(body, "custom page") {
		t.Errorf("GET /about: got status %d", resp.StatusCode)
	}

	// Seeded posts render with their images.
	if resp, body := do(t, app, "GET", "/blog/post/welcome-to-gox-cms"); resp.StatusCode != fiber.StatusOK || !strings.Contains(body, "/static/uploads/seed-1.jpg") {
		t.Errorf("GET seeded post: got status %d", resp.StatusCode)
	}
}

func TestMenuBuilderFlow(t *testing.T) {
	app, db := newTestApp(t)
	// Fresh test apps seed a primary menu; start clean for this flow.
	db.Exec("DELETE FROM menu_items")
	db.Exec("DELETE FROM menus")
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookie(t, app, auth)

	// New menus land at the end without a position number.
	if resp, _ := postForm(t, app, "/add-menu", url.Values{"menu_title": {"Main"}}, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("add menu: got status %d", resp.StatusCode)
	}
	var menu model.Menu
	if err := db.Where("title = ?", "Main").First(&menu).Error; err != nil {
		t.Fatalf("menu not created: %v", err)
	}

	addItem := func(title, link string) {
		t.Helper()
		form := url.Values{"menu_item_title": {title}, "menu_item_link": {link}, "menu_item_menu": {strconv.Itoa(int(menu.ID))}}
		if resp, _ := postForm(t, app, "/add-menu-item", form, token, auth); resp.StatusCode != fiber.StatusOK {
			t.Fatalf("add item %q: got status %d", title, resp.StatusCode)
		}
	}
	addItem("Alpha", "/alpha")
	addItem("Beta", "/beta")

	// Missing title is a 400, not a 500.
	bad := url.Values{"menu_item_title": {""}, "menu_item_link": {"/x"}, "menu_item_menu": {strconv.Itoa(int(menu.ID))}}
	if resp, _ := postForm(t, app, "/add-menu-item", bad, token, auth); resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("invalid item: got status %d, want 400", resp.StatusCode)
	}

	// Move Beta above Alpha.
	var beta model.MenuItem
	db.Where("title = ?", "Beta").First(&beta)
	movePath := "/move-menu-item/" + strconv.Itoa(int(beta.ID)) + "/up"
	if resp, _ := postForm(t, app, movePath, url.Values{}, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("move item: got status %d", resp.StatusCode)
	}
	var items []model.MenuItem
	db.Where("menu_id = ?", menu.ID).Order("position ASC").Find(&items)
	if len(items) != 2 || items[0].Title != "Beta" || items[1].Title != "Alpha" {
		t.Fatalf("wrong order after move: %+v", items)
	}
}

func TestHomepageLeaksNoCredentials(t *testing.T) {
	app, _ := newTestApp(t)

	resp, body := do(t, app, "GET", "/")
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET /: got status %d", resp.StatusCode)
	}
	for _, bad := range []string{"admin1234", "admin_password", "Password:"} {
		if strings.Contains(body, bad) {
			t.Errorf("homepage contains %q", bad)
		}
	}
}

func TestSitemap(t *testing.T) {
	app, db := newTestApp(t)
	db.Create(&model.Post{Title: "Live", Content: "x", Slug: "live", Published: true})
	db.Create(&model.Post{Title: "Draft", Content: "x", Slug: "draft"})

	resp, body := do(t, app, "GET", "/sitemap.xml")
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("got status %d", resp.StatusCode)
	}
	for _, want := range []string{"http://localhost:3000/</loc>", "/blog</loc>", "/blog/post/live</loc>"} {
		if !strings.Contains(body, want) {
			t.Errorf("sitemap missing %q", want)
		}
	}
	for _, unwanted := range []string{"/blog/post/draft", "/user/"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("sitemap contains %q", unwanted)
		}
	}
}

func TestHealthz(t *testing.T) {
	app, _ := newTestApp(t)

	resp, body := do(t, app, "GET", "/healthz")
	if resp.StatusCode != fiber.StatusOK || !strings.Contains(body, `"ok"`) {
		t.Fatalf("GET /healthz: got status %d, body %q", resp.StatusCode, body)
	}
}

func TestSecurityHeaders(t *testing.T) {
	app, _ := newTestApp(t)

	resp, _ := do(t, app, "GET", "/")
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := resp.Header.Get("Content-Security-Policy"); !strings.Contains(got, "object-src 'none'") {
		t.Errorf("Content-Security-Policy missing object-src 'none': %q", got)
	}
}

func TestLogoutRevokesJWT(t *testing.T) {
	app, db := newTestApp(t)
	createUser(t, db, "alice", model.RoleUser)

	token := csrfCookie(t, app)
	resp, _ := postForm(t, app, "/login", url.Values{"username": {"alice"}, "password": {"password123"}}, token)
	var jwtCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "jwt" {
			jwtCookie = c
		}
	}
	if jwtCookie == nil {
		t.Fatal("login did not set a jwt cookie")
	}

	if resp, _ := postForm(t, app, "/logout", url.Values{}, token, jwtCookie); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("logout: got status %d, want 200", resp.StatusCode)
	}

	// The same token must no longer authenticate.
	if resp, _ := do(t, app, "GET", "/admin", jwtCookie); !isDenied(resp.StatusCode) {
		t.Errorf("stale token after logout: got status %d, want it rejected", resp.StatusCode)
	}
}

func TestLoginThrottleBlocksBruteForce(t *testing.T) {
	app, db := newTestApp(t)
	createUser(t, db, "alice", model.RoleUser)
	handlers.ResetLoginAttemptsForIP("192.0.2.1")
	viper.Set("auth.login_max_attempts", 3)
	t.Cleanup(func() {
		viper.Set("auth.login_max_attempts", 10)
		handlers.ResetLoginAttemptsForIP("192.0.2.1")
	})
	token := csrfCookie(t, app)
	bad := url.Values{"username": {"nobody"}, "password": {"wrong"}}

	for i := 0; i < 3; i++ {
		if resp, _ := postForm(t, app, "/login", bad, token); resp.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("bad login %d: got status %d, want 401", i+1, resp.StatusCode)
		}
	}
	if resp, _ := postForm(t, app, "/login", bad, token); resp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("login after 3 failures: got status %d, want 429", resp.StatusCode)
	}
	// Even correct credentials are refused while blocked.
	good := url.Values{"username": {"alice"}, "password": {"password123"}}
	if resp, _ := postForm(t, app, "/login", good, token); resp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("good login while blocked: got status %d, want 429", resp.StatusCode)
	}
}

func TestPostContentIsSanitized(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookie(t, app, auth)

	form := url.Values{
		"title":     {"XSS"},
		"content":   {`<script>alert(1)</script><p>Safe</p>`},
		"post_slug": {"xss-test"},
		"image":     {"/img.png"},
	}
	if resp, _ := postForm(t, app, "/admin/post/add", form, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("add post: got status %d", resp.StatusCode)
	}

	resp, body := do(t, app, "GET", "/blog/post/xss-test", auth)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET post: got status %d", resp.StatusCode)
	}
	// NOTE: the page layout itself ships inline <script> blocks, so only the
	// stored payload may be asserted on.
	if strings.Contains(body, "alert(1)") {
		t.Error("stored script survived sanitization")
	}
	if !strings.Contains(body, "Safe") {
		t.Error("safe markup was lost during sanitization")
	}
}

func TestUploadRejectsNonImageContent(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookie(t, app, auth)
	viper.Set("upload.max_size_mb", 1)

	var buf strings.Builder
	w := multipart.NewWriter(&buf)
	part, _ := w.CreateFormFile("file", "evil.PNG")
	part.Write([]byte("<html><script>alert(1)</script></html>"))
	w.Close()

	req := httptest.NewRequest("POST", "/upload-file", strings.NewReader(buf.String()))
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-Csrf-Token", token.Value)
	req.AddCookie(token)
	req.AddCookie(auth)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("got status %d, want 400", resp.StatusCode)
	}
}
