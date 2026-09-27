package main

import (
	"fmt"
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
// csrfCookies returns the cookies a safe GET hands back for CSRF: the token
// cookie plus the session cookie the token is bound to. Both must be sent on
// the unsafe request, which is why this returns a slice.
func csrfCookies(t *testing.T, app *fiber.App, cookies ...*http.Cookie) []*http.Cookie {
	t.Helper()
	req := httptest.NewRequest("GET", "/login", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	var token, session *http.Cookie
	for _, c := range resp.Cookies() {
		switch c.Name {
		case "csrf_":
			token = c
		case "session_id":
			session = c
		}
	}
	if token == nil {
		t.Fatal("no csrf_ cookie set on GET /login")
	}
	if session == nil {
		t.Fatal("no session_id cookie set on GET /login")
	}
	return []*http.Cookie{token, session}
}

func postForm(t *testing.T, app *fiber.App, path string, form url.Values, csrfCookies []*http.Cookie, cookies ...*http.Cookie) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	for _, c := range csrfCookies {
		if c == nil {
			continue
		}
		if c.Name == "csrf_" {
			req.Header.Set("X-Csrf-Token", c.Value)
		}
		req.AddCookie(c)
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

	token := csrfCookies(t, app, auth)
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
	token := csrfCookies(t, app)

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

	// A fresh anonymous token: the one used for the successful login is dead
	// now that the session was rotated.
	resp, _ = postForm(t, app, "/login", url.Values{"username": {"alice"}, "password": {"wrong"}}, csrfCookies(t, app))
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("login with wrong password: got status %d, want 401", resp.StatusCode)
	}

	// The pre-login token must no longer be accepted anywhere: the token is
	// bound to the session and rotates on login.
	if resp, _ := postForm(t, app, "/login", url.Values{"username": {"alice"}, "password": {"password123"}}, token); resp.StatusCode != fiber.StatusForbidden {
		t.Errorf("pre-login CSRF token after login: got status %d, want 403", resp.StatusCode)
	}
}

// login performs a login the way a browser does and returns the cookies
// needed for subsequent unsafe requests.
//
// The CSRF token must be re-fetched after logging in: Login regenerates the
// session, and the token is bound to that session. That is deliberate — it is
// the token rotation on privilege change — so a pre-login token is expected to
// stop working, and this helper reproduces the redirect-then-refetch a browser
// performs.
func login(t *testing.T, app *fiber.App, username, password string, extra ...*http.Cookie) (*http.Cookie, []*http.Cookie) {
	t.Helper()
	pre := csrfCookies(t, app, extra...)
	resp, _ := postForm(t, app, "/login", url.Values{"username": {username}, "password": {password}}, pre, extra...)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("login: got status %d", resp.StatusCode)
	}
	var jwtCookie, newSession *http.Cookie
	for _, c := range resp.Cookies() {
		switch c.Name {
		case "jwt":
			jwtCookie = c
		case "session_id":
			newSession = c
		}
	}
	if jwtCookie == nil {
		t.Fatal("login did not set a jwt cookie")
	}
	if newSession == nil {
		t.Fatal("login did not rotate the session cookie")
	}
	carried := append([]*http.Cookie{}, extra...)
	carried = append(carried, jwtCookie, newSession)
	return jwtCookie, csrfCookies(t, app, carried...)
}

func enableRegistration(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Model(&model.BasicWebsiteInfo{}).Where("id > ?", 0).Update("registration_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	handlers.ReloadSiteSettings(db)
}

func TestRegisterDisabledByDefault(t *testing.T) {
	app, _ := newTestApp(t)

	if resp, _ := do(t, app, "GET", "/register"); resp.StatusCode != fiber.StatusFound {
		t.Errorf("GET /register while disabled: got status %d, want 302 to /login", resp.StatusCode)
	}
	token := csrfCookies(t, app)
	form := url.Values{"username": {"newuser"}, "password": {"secret123"}, "first_name": {"New"}, "last_name": {"User"}}
	if resp, _ := postForm(t, app, "/register", form, token); resp.StatusCode != fiber.StatusForbidden {
		t.Errorf("POST /register while disabled: got status %d, want 403", resp.StatusCode)
	}
}

func TestRegisterWorksWithCaptchaDisabled(t *testing.T) {
	app, db := newTestApp(t)
	enableRegistration(t, db)
	token := csrfCookies(t, app)

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
	token := csrfCookies(t, app, auth)

	first := model.Post{Title: "First", Content: "one", Slug: "first"}
	second := model.Post{Title: "Second", Content: "two", Slug: "second"}
	db.Create(&first)
	db.Create(&second)
	cat := model.Category{Name: "Local", Slug: "local"}
	if err := db.Create(&cat).Error; err != nil {
		t.Fatalf("could not create category: %v", err)
	}

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
	token := csrfCookies(t, app, auth)

	form := url.Values{"title": {"Contact"}, "content": {"<p>Contact us</p>"}, "slug": {"contact"},
		"template": {"page"}, "published": {"on"}}
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

func TestUpdateSettings(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	form := url.Values{"name": {"New Name"}, "theme": {"darkly"}, "container_class": {"container"}}
	if resp, _ := postForm(t, app, "/update-settings", form, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("update settings: got status %d", resp.StatusCode)
	}
	var info model.BasicWebsiteInfo
	db.First(&info)
	if info.Name != "New Name" || info.Theme != "darkly" {
		t.Fatalf("settings not saved: %+v", info)
	}

	// A dark Bootswatch theme starts the page in dark mode, and the navbar
	// must not carry hardcoded colour classes that would fight it.
	if resp, body := do(t, app, "GET", "/"); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("home after settings update: got status %d", resp.StatusCode)
	} else {
		if !strings.Contains(body, `data-bs-theme="dark"`) {
			t.Error("dark Bootswatch theme did not render data-bs-theme=dark")
		}
		for _, bad := range []string{"navbar-light", "bg-light", "navbar-dark", "bg-dark"} {
			if strings.Contains(body, bad) {
				t.Errorf("navbar still hardcodes %q", bad)
			}
		}
	}
}

func TestTagCategoryCRUD(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	tagForm := url.Values{"tag_name": {"Go"}, "tag_slug": {"go"}}
	if resp, _ := postForm(t, app, "/add-tag", tagForm, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("add tag: got status %d", resp.StatusCode)
	}
	// Duplicate slug does not create a second row.
	postForm(t, app, "/add-tag", tagForm, token, auth)
	var tagCount int64
	db.Model(&model.Tag{}).Where("slug = ?", "go").Count(&tagCount)
	if tagCount != 1 {
		t.Fatalf("tag slug count = %d, want 1", tagCount)
	}

	catForm := url.Values{"category_name": {"News"}, "category_slug": {"news"}}
	// Note: the seed owns slug "news", so this must not create a duplicate.
	postForm(t, app, "/add-category", catForm, token, auth)
	var catCount int64
	db.Model(&model.Category{}).Where("slug = ?", "news").Count(&catCount)
	if catCount != 1 {
		t.Fatalf("category slug count = %d, want 1", catCount)
	}

	var tag model.Tag
	db.Where("slug = ?", "go").First(&tag)
	delReq := func(path string) *http.Response {
		t.Helper()
		req := httptest.NewRequest("DELETE", path+"?id="+strconv.Itoa(int(tag.ID)), nil)
		req.Header.Set("HX-Request", "true")
		req.AddCookie(auth)
		for _, c := range token {
			req.AddCookie(c)
			if c.Name == "csrf_" {
				req.Header.Set("X-Csrf-Token", c.Value)
			}
		}
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	if resp := delReq("/delete-tag"); resp.StatusCode != fiber.StatusOK {
		t.Errorf("delete tag: got status %d", resp.StatusCode)
	}
}

func TestCommentModeration(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)
	post := model.Post{Title: "P", Content: "x", Slug: "p", Published: true, UserID: admin.ID}
	db.Create(&post)
	comment := model.Comment{Content: "hello", UserID: admin.ID, PostID: post.ID, Status: "pending"}
	db.Create(&comment)

	toggle := "/toggle-comment-status/" + strconv.Itoa(int(comment.ID))
	if resp, _ := postForm(t, app, toggle, url.Values{}, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("toggle comment: got status %d", resp.StatusCode)
	}
	db.First(&comment, comment.ID)
	if comment.Status != "approved" {
		t.Fatalf("comment status = %q, want approved", comment.Status)
	}

	if resp, _ := deleteReq(t, app, "/delete-comment/"+strconv.Itoa(int(comment.ID)), token, auth); resp.StatusCode != fiber.StatusNoContent {
		t.Errorf("delete comment: got status %d, want 204", resp.StatusCode)
	}
}

func TestUploadRoundTrip(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)
	viper.Set("upload.max_size_mb", 1)

	// Minimal bytes that sniff as image/png.
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 200)...)
	var buf strings.Builder
	w := multipart.NewWriter(&buf)
	part, _ := w.CreateFormFile("file", "tiny.png")
	part.Write(png)
	w.Close()

	req := httptest.NewRequest("POST", "/upload-file", strings.NewReader(buf.String()))
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(auth)
	for _, c := range token {
		req.AddCookie(c)
		if c.Name == "csrf_" {
			req.Header.Set("X-Csrf-Token", c.Value)
		}
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("upload: got status %d", resp.StatusCode)
	}
	var files int64
	db.Model(&model.File{}).Where("name LIKE ?", "%tiny%").Count(&files)
	if files != 1 {
		t.Fatalf("uploaded files = %d, want 1", files)
	}
}

func TestPluginToggle(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	if resp, _ := postForm(t, app, "/admin/plugins/enable/LatestPostsPlugin", url.Values{}, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("toggle plugin: got status %d", resp.StatusCode)
	}
	var plugin model.Plugin
	db.Where("name = ?", "LatestPostsPlugin").First(&plugin)
	if !plugin.Enabled {
		t.Error("plugin not enabled after toggle")
	}
	if resp, _ := postForm(t, app, "/admin/plugins/enable/NoSuchPlugin", url.Values{}, token, auth); resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("toggle missing plugin: got status %d, want 404", resp.StatusCode)
	}
}

func TestClearCache(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	if resp, _ := postForm(t, app, "/clear-cache", url.Values{}, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Errorf("clear cache: got status %d, want 200", resp.StatusCode)
	}
}

func TestHeaderHasToggleAndSearch(t *testing.T) {
	app, _ := newTestApp(t)

	// NOTE: navbar color classes are asserted in TestUpdateSettings (the site
	// settings cache is process-global, so color here depends on test order).
	_, body := do(t, app, "GET", "/")
	for _, want := range []string{`id="btnSwitch"`, `action="/search"`, `id="main-content"`, `skip-link`} {
		if !strings.Contains(body, want) {
			t.Errorf("homepage missing %q", want)
		}
	}
}

func TestNavbarFollowsTheme(t *testing.T) {
	app, _ := newTestApp(t)

	_, body := do(t, app, "GET", "/")
	// No fixed palette on the navbar, and the theme comes from settings.
	for _, bad := range []string{"navbar-light", "navbar-dark", "bg-light", "bg-dark"} {
		if strings.Contains(body, bad) {
			t.Errorf("navbar hardcodes %q, which fights data-bs-theme", bad)
		}
	}
	if !strings.Contains(body, `data-bs-theme="light"`) {
		t.Error("expected a light default for a light Bootswatch theme")
	}
	// The theme switch must be wired to the delegated handler.
	if !strings.Contains(body, "data-theme-toggle") {
		t.Error("theme switch missing data-theme-toggle")
	}
}

func TestSiteSearch(t *testing.T) {
	app, _ := newTestApp(t)

	resp, body := do(t, app, "GET", "/search?q=welcome")
	if resp.StatusCode != fiber.StatusOK || !strings.Contains(body, "Welcome to GoX CMS") {
		t.Fatalf("search welcome: got status %d", resp.StatusCode)
	}
	// Wildcards must not break or broaden the search.
	if resp, _ := do(t, app, "GET", "/search?q=%"); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("search %%: got status %d", resp.StatusCode)
	}
	if resp, body := do(t, app, "GET", "/search?q=zzz-no-such-thing"); resp.StatusCode != fiber.StatusOK || !strings.Contains(body, "Nothing found") {
		t.Fatalf("empty search: got status %d", resp.StatusCode)
	}
}

func TestPreviewEndpoints(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	// Anonymous users cannot render previews.
	if resp, _ := postForm(t, app, "/preview-post", url.Values{"title": {"Hi"}}, token); !isDenied(resp.StatusCode) {
		t.Errorf("anon preview: got status %d, want denied", resp.StatusCode)
	}

	form := url.Values{"title": {"Draft Peek"}, "content": {"<p>Hello</p><script>alert(1)</script>"}, "image": {"/img.png"}}
	resp, body := postForm(t, app, "/preview-post", form, token, auth)
	if resp.StatusCode != fiber.StatusOK || !strings.Contains(body, "Draft Peek") {
		t.Fatalf("post preview: got status %d", resp.StatusCode)
	}
	if strings.Contains(body, "alert(1)") {
		t.Error("preview did not sanitize content")
	}
	if resp, _ := postForm(t, app, "/preview-page", url.Values{"title": {""}}, token, auth); resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("preview without title: got status %d, want 400", resp.StatusCode)
	}
	if resp, body := postForm(t, app, "/preview-page", url.Values{"title": {"About Draft"}, "content": {"<p>x</p>"}}, token, auth); resp.StatusCode != fiber.StatusOK || !strings.Contains(body, "About Draft") {
		t.Errorf("page preview: got status %d", resp.StatusCode)
	}
}

func TestAddTaxonomyInline(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	form := url.Values{"kind": {"tag"}, "name": {"Fresh Tag"}}
	resp, body := postForm(t, app, "/add-taxonomy", form, token, auth)
	if resp.StatusCode != fiber.StatusCreated || !strings.Contains(body, `"id"`) {
		t.Fatalf("create tag inline: got status %d body %q", resp.StatusCode, body)
	}
	// Duplicate name returns the existing row.
	if resp, _ := postForm(t, app, "/add-taxonomy", form, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Errorf("duplicate tag inline: got status %d, want 200", resp.StatusCode)
	}
	var n int64
	db.Model(&model.Tag{}).Where("slug = ?", "fresh-tag").Count(&n)
	if n != 1 {
		t.Errorf("tag rows = %d, want 1", n)
	}
	if resp, _ := postForm(t, app, "/add-taxonomy", url.Values{"kind": {"nope"}, "name": {"x"}}, token, auth); resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("bad kind: got status %d, want 400", resp.StatusCode)
	}
}

func TestBulkActions(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	p1 := model.Post{Title: "B1", Content: "x", Slug: "bulk-1", UserID: admin.ID}
	p2 := model.Post{Title: "B2", Content: "x", Slug: "bulk-2", UserID: admin.ID}
	db.Create(&p1)
	db.Create(&p2)
	ids := strconv.Itoa(int(p1.ID)) + "," + strconv.Itoa(int(p2.ID))

	pub := url.Values{"action": {"publish"}, "ids": {ids}}
	if resp, _ := postForm(t, app, "/bulk-posts", pub, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("bulk publish: got status %d", resp.StatusCode)
	}
	var published int64
	db.Model(&model.Post{}).Where("id IN ? AND published = ?", []uint{p1.ID, p2.ID}, true).Count(&published)
	if published != 2 {
		t.Fatalf("published = %d, want 2", published)
	}

	c1 := model.Comment{Content: "a", UserID: admin.ID, PostID: p1.ID, Status: "pending"}
	c2 := model.Comment{Content: "b", UserID: admin.ID, PostID: p1.ID, Status: "pending"}
	db.Create(&c1)
	db.Create(&c2)
	cids := strconv.Itoa(int(c1.ID)) + "," + strconv.Itoa(int(c2.ID))
	if resp, _ := postForm(t, app, "/bulk-comments", url.Values{"action": {"approve"}, "ids": {cids}}, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("bulk approve: got status %d", resp.StatusCode)
	}

	del := url.Values{"action": {"delete"}, "ids": {ids}}
	if resp, _ := postForm(t, app, "/bulk-posts", del, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("bulk delete: got status %d", resp.StatusCode)
	}
	var left int64
	db.Model(&model.Post{}).Where("id IN ?", []uint{p1.ID, p2.ID}).Count(&left)
	if left != 0 {
		t.Fatalf("posts left = %d, want 0", left)
	}
	var commentsLeft int64
	db.Model(&model.Comment{}).Where("post_id IN ?", []uint{p1.ID, p2.ID}).Count(&commentsLeft)
	if commentsLeft != 0 {
		t.Fatalf("orphan comments = %d, want 0", commentsLeft)
	}

	if resp, _ := postForm(t, app, "/bulk-posts", url.Values{"action": {"wipe"}, "ids": {ids}}, token, auth); resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("bad bulk action: got status %d, want 400", resp.StatusCode)
	}
}

func TestHeaderUsesSiteContainer(t *testing.T) {
	app, db := newTestApp(t)

	// The navbar inner wrapper must use the site's container class, not
	// container-fluid, or the header spans the viewport while the body
	// content stays centred.
	db.Model(&model.BasicWebsiteInfo{}).Where("1 = 1").Update("container_class", "container")
	_, body := do(t, app, "GET", "/")
	nav := strings.Index(body, "<nav")
	if nav < 0 {
		t.Fatal("no <nav> in the page")
	}
	head := body[nav:]
	if i := strings.Index(head, "</nav>"); i > 0 {
		head = head[:i]
	}
	if !strings.Contains(head, `<div class="container">`) {
		t.Errorf("navbar does not use the site container class:\n%s", head)
	}
	if strings.Contains(head, "container-fluid") {
		t.Errorf("navbar still uses container-fluid:\n%s", head)
	}
}

func TestCommentTableEscapesUserHTML(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	// A visitor's comment is stored HTML-escaped by SanitizeText. The admin
	// table must render it as text, never unescaped (that was a stored XSS).
	post := model.Post{Title: "Hostile", Content: "x", Slug: "hostile", UserID: admin.ID}
	db.Create(&post)
	// SanitizeText strips complete tags and escapes the rest, so a payload
	// that survives storage is one with a bare "<" and no closing ">".
	hostile := `<img src=x onerror=alert(1)`
	if got := SanitizeTextForTest(hostile); got == hostile {
		t.Fatalf("test payload was not escaped on the way in: %q", got)
	}
	if err := db.Create(&model.Comment{
		Content: SanitizeTextForTest(hostile),
		UserID:  admin.ID,
		PostID:  post.ID,
		Status:  "pending",
	}).Error; err != nil {
		t.Fatalf("could not seed comment: %v", err)
	}

	resp, body := do(t, app, "GET", "/search-comments", auth)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("comments table: got status %d", resp.StatusCode)
	}
	// The row must never render the payload as live markup.
	if strings.Contains(body, "<img src=x onerror") {
		t.Error("admin comments table rendered comment content as live markup")
	}
	// It must also be readable: the table used `unescape` on already-escaped
	// text, so admins saw &lt;img... instead of <img...
	if !strings.Contains(body, "&lt;img src=x onerror=alert(1)") {
		t.Errorf("comment text was not shown decoded once:\n%s", body)
	}
}

// SanitizeTextForTest mirrors handlers.SanitizeText, which is unexported to
// other packages.
func SanitizeTextForTest(s string) string {
	return handlers.SanitizeText(s)
}

func TestAccountProfileUpdate(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	// The page must show the signed-in user, not just a password form.
	_, page := do(t, app, "GET", "/account", auth)
	for _, want := range []string{"boss", "At a glance", "Change password", "account-avatar"} {
		if !strings.Contains(page, want) {
			t.Errorf("/account missing %q", want)
		}
	}

	form := url.Values{
		"first_name": {"Ada"},
		"last_name":  {"Lovelace"},
		"username":   {"adalove"},
		"email":      {"ada@example.com"},
	}
	resp, _ := postForm(t, app, "/account/profile", form, token, auth)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("update profile: got status %d", resp.StatusCode)
	}
	var fresh model.User
	db.First(&fresh, admin.ID)
	if fresh.Username != "adalove" || fresh.FirstName != "Ada" || fresh.LastName != "Lovelace" {
		t.Errorf("profile not saved: %+v", fresh)
	}
	if fresh.Email == nil || *fresh.Email != "ada@example.com" {
		t.Errorf("email not saved: %+v", fresh.Email)
	}

	// A username already taken by somebody else must be rejected.
	other := createUser(t, db, "taken", model.RoleUser)
	bad := url.Values{
		"first_name": {"Ada"}, "last_name": {"L"}, "username": {other.Username}, "email": {""},
	}
	if resp, body := postForm(t, app, "/account/profile", bad, token, auth); resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("duplicate username: got status %d want 400, body=%q", resp.StatusCode, body)
	}
	// Non-alphanumeric usernames are rejected.
	bad2 := url.Values{
		"first_name": {"Ada"}, "last_name": {"L"}, "username": {"has space!"}, "email": {""},
	}
	if resp, _ := postForm(t, app, "/account/profile", bad2, token, auth); resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("invalid username: got status %d, want 400", resp.StatusCode)
	}
}

// The post editor used to open <form> inside the first column and close it
// inside the second, so the </div> closing the column popped the form and the
// image/slug/categories/tags controls were never submitted: the editor could
// not save anything. This parses the served markup instead of posting fields
// directly, which is what missed the bug originally.
func TestPostEditorSidebarInputsAreInsideForm(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	post := model.Post{Title: "Editable", Content: "x", Slug: "editable", UserID: admin.ID}
	db.Create(&post)

	for _, path := range []string{"/admin/post/add", "/admin/post/edit/" + strconv.Itoa(int(post.ID))} {
		resp, body := do(t, app, "GET", path, auth)
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("GET %s: got status %d", path, resp.StatusCode)
		}

		// Anchor on the post form itself: the admin layout has a logout form
		// in its topbar that appears earlier in the document.
		open := strings.Index(body, `<form id="post-form"`)
		if open < 0 {
			t.Fatalf("GET %s: no post form", path)
		}
		rel := strings.Index(body[open:], "</form>")
		if rel < 0 {
			t.Fatalf("GET %s: form never closed", path)
		}
		end := open + rel

		// The real invariant: every sidebar control is inside the form.
		for _, name := range []string{`name="image"`, `name="post_slug"`, `name="categories_input"`, `name="tags_input"`} {
			idx := strings.Index(body, name)
			if idx < 0 {
				t.Errorf("GET %s: missing %s", path, name)
				continue
			}
			if idx < open || idx > end {
				t.Errorf("GET %s: %s is outside the post <form>", path, name)
			}
		}

		// Both columns must be direct flex children of the form, not wrapped
		// in a div that would close before the second one starts.
		formHTML := body[open:end]
		if !strings.Contains(formHTML, `<div class="col-12 col-lg-8">`) ||
			!strings.Contains(formHTML, `<div class="col-12 col-lg-4">`) {
			t.Errorf("GET %s: both editor columns are not inside the form", path)
		}
	}
}

// An <a> cannot be a <form>, and only one #result may exist or htmx targets
// the wrong one.
func TestPostEditorMarkupSanity(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	for _, path := range []string{"/admin/post/add", "/add-custompage"} {
		_, body := do(t, app, "GET", path, auth)
		if n := strings.Count(body, `id="result"`); n > 1 {
			t.Errorf("GET %s: %d elements share id=result", path, n)
		}
		// The dead #toolbar div was never used: editor.js inits Quill without
		// a container option, so Quill creates its own toolbar.
		if strings.Contains(body, `id="toolbar"`) {
			t.Errorf("GET %s: dead #toolbar div is back", path)
		}
		// The label pointed at #content, which does not exist.
		if strings.Contains(body, `for="content"`) {
			t.Errorf("GET %s: label still points at the non-existent #content", path)
		}
	}
}

// The admin layout replaces the public navbar/footer, which contain no link
// back to /admin.
func TestAdminLayoutDropsPublicChrome(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	for _, path := range []string{"/admin", "/admin/post/add", "/add-custompage"} {
		_, body := do(t, app, "GET", path, auth)
		if !strings.Contains(body, `class="container-fluid admin-shell"`) &&
			!strings.Contains(body, "admin-shell") {
			t.Errorf("GET %s: not rendered with the admin layout", path)
		}
		if !strings.Contains(body, "View site") {
			t.Errorf("GET %s: no way back to the public site", path)
		}
		// The public navbar partial is the thing being removed.
		if strings.Contains(body, "get-primary-menu") {
			t.Errorf("GET %s: still loads the public navbar", path)
		}
		if strings.Contains(body, "<footer") {
			t.Errorf("GET %s: still renders the public footer", path)
		}
	}
}

// The site template set swaps the public markup. "simple" must be
// framework-free, which is the whole point of adding it.
func TestSiteTemplateSwitch(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	_, defaultBody := do(t, app, "GET", "/")
	if !strings.Contains(defaultBody, "navbar-expand-lg") {
		t.Error("default set should render the Bootstrap navbar")
	}

	form := url.Values{"name": {"GoX CMS"}, "site_template": {"simple"}, "container_class": {"container"}}
	if resp, _ := postForm(t, app, "/update-settings", form, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("switch to simple: got status %d", resp.StatusCode)
	}

	_, simple := do(t, app, "GET", "/")
	if !strings.Contains(simple, "site-simple.css") {
		t.Error("simple set did not load its own stylesheet")
	}
	if strings.Contains(simple, "bootstrap.min.css") {
		t.Error("simple set loaded a Bootstrap stylesheet; it is meant to be framework-free")
	}
	if strings.Contains(simple, "navbar-expand-lg") {
		t.Error("simple set still renders the Bootstrap navbar")
	}
	if !strings.Contains(simple, "<main id=\"main-content\"") {
		t.Error("simple layout is missing the main landmark")
	}

	// Blog, search and 404 must all follow the switch, not just the home page.
	for _, path := range []string{"/blog", "/search?q=welcome"} {
		_, body := do(t, app, "GET", path)
		if !strings.Contains(body, "site-simple.css") {
			t.Errorf("GET %s: simple set not applied", path)
		}
	}
	resp, notFound := do(t, app, "GET", "/definitely-not-a-page")
	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("404 handler: got status %d", resp.StatusCode)
	}
	if !strings.Contains(notFound, "site-simple.css") {
		t.Error("404 did not follow the template set")
	}

	// An unknown value must fall back rather than 500 on a missing template.
	db.Model(&model.BasicWebsiteInfo{}).Where("1 = 1").Update("site_template", "does-not-exist")
	handlers.ReloadSiteSettings(db)
	if resp, _ := do(t, app, "GET", "/"); resp.StatusCode != fiber.StatusOK {
		t.Errorf("unknown template set: got status %d, want a default-set fallback", resp.StatusCode)
	}
}

// A custom page resolves inside the active set.
func TestCustomPageFollowsTemplateSet(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	form := url.Values{
		"title": {"Contact"}, "content": {"<p>Reach us</p>"}, "slug": {"contact"},
		"template": {"page"}, "published": {"on"},
	}
	if resp, _ := postForm(t, app, "/add-custompage", form, token, auth); resp.StatusCode >= 400 {
		t.Fatalf("add custom page: got status %d", resp.StatusCode)
	}

	_, def := do(t, app, "GET", "/contact")
	if strings.Contains(def, "site-simple.css") {
		t.Fatal("custom page rendered the simple set before the switch")
	}

	settingsForm := url.Values{"name": {"GoX CMS"}, "site_template": {"simple"}, "container_class": {"container"}}
	if resp, _ := postForm(t, app, "/update-settings", settingsForm, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("switch: got status %d", resp.StatusCode)
	}

	resp, simple := do(t, app, "GET", "/contact")
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET /contact: got status %d", resp.StatusCode)
	}
	if !strings.Contains(simple, "site-simple.css") {
		t.Error("custom page did not follow the template set")
	}
	if !strings.Contains(simple, "Reach us") {
		t.Error("custom page content missing")
	}
}

// GORM treats a non-numeric string passed to First() as a raw SQL fragment,
// and an *empty* one as no condition at all. Both were reachable from three
// admin endpoints, so this asserts injection and no-id are rejected and that
// nothing is mutated.
func TestIDParamsAreNotRawSQL(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	older := model.Post{Title: "Older", Content: "x", Slug: "older", UserID: admin.ID, Published: true}
	newer := model.Post{Title: "Newer", Content: "x", Slug: "newer", UserID: admin.ID, Published: false}
	db.Create(&older)
	db.Create(&newer)
	tag := model.Tag{Name: "Keep", Slug: "keep"}
	db.Create(&tag)
	cat := model.Category{Name: "KeepCat", Slug: "keepcat"}
	db.Create(&cat)

	// Injection attempts and an absent id, all of which used to be executed
	// verbatim (and the empty one hit row #1).
	for _, id := range []string{"1 OR 1=1", "1; DROP TABLE posts", "", "0", "-1", "abc", "1 UNION SELECT 1"} {
		esc := url.QueryEscape(id)
		if resp, _ := postForm(t, app, "/toggle-post-status",
			url.Values{"id": {id}}, token, auth); resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("toggle-post-status id=%q: got status %d, want 400", id, resp.StatusCode)
		}
		if resp, _ := deleteReq(t, app, "/delete-tag?id="+esc, token, auth); resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("delete-tag id=%q: got status %d, want 400", id, resp.StatusCode)
		}
		if resp, _ := deleteReq(t, app, "/delete-category?id="+esc, token, auth); resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("delete-category id=%q: got status %d, want 400", id, resp.StatusCode)
		}
	}

	// Nothing was touched: no row flipped, nothing deleted. Counted by slug
	// because newTestApp seeds demo tags and categories.
	var published, draft int64
	db.Model(&model.Post{}).Where("slug = ? AND published = ?", "older", true).Count(&published)
	db.Model(&model.Post{}).Where("slug = ? AND published = ?", "newer", false).Count(&draft)
	if published != 1 || draft != 1 {
		t.Errorf("posts were mutated: older published=%d newer draft=%d", published, draft)
	}
	var keepTag, keepCat int64
	db.Model(&model.Tag{}).Where("slug = ?", "keep").Count(&keepTag)
	db.Model(&model.Category{}).Where("slug = ?", "keepcat").Count(&keepCat)
	if keepTag != 1 || keepCat != 1 {
		t.Errorf("rows were deleted: tag=%d category=%d", keepTag, keepCat)
	}

	// A real id still works, so the fix did not break the happy path.
	if resp, _ := postForm(t, app, "/toggle-post-status",
		url.Values{"id": {strconv.Itoa(int(newer.ID))}}, token, auth); resp.StatusCode != fiber.StatusOK {
		t.Errorf("valid toggle: got status %d, want 200", resp.StatusCode)
	}
}

// A custom page is a draft until published. The catch-all used to filter on
// slug only, so every page an admin saved went live immediately.
func TestUnpublishedCustomPageIsNotServed(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	draft := url.Values{"title": {"Draft"}, "content": {"<p>secret</p>"}, "slug": {"draft"},
		"template": {"page"}}
	if resp, _ := postForm(t, app, "/add-custompage", draft, token, auth); resp.StatusCode >= 400 {
		t.Fatalf("add draft: got status %d", resp.StatusCode)
	}
	resp, body := do(t, app, "GET", "/draft")
	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("draft page: got status %d, want 404", resp.StatusCode)
	}
	if strings.Contains(body, "secret") {
		t.Error("draft page content leaked into the 404 response")
	}

	// Publishing it makes it public.
	page := model.CustomPage{}
	db.Where("slug = ?", "draft").First(&page)
	page.Published = true
	db.Save(&page)
	if resp, body := do(t, app, "GET", "/draft"); resp.StatusCode != fiber.StatusOK || !strings.Contains(body, "secret") {
		t.Errorf("published page: got status %d", resp.StatusCode)
	}
}

// The CSRF token is bound to the session, so a token minted for one visitor
// must not authenticate a request carrying a different session.
func TestCSRFTokenIsSessionBound(t *testing.T) {
	app, _ := newTestApp(t)

	victimSession := csrfCookies(t, app)
	attackerSession := csrfCookies(t, app)

	var victimToken string
	for _, c := range victimSession {
		if c.Name == "csrf_" {
			victimToken = c.Value
		}
	}
	if victimToken == "" {
		t.Fatal("no victim csrf token")
	}

	req := httptest.NewRequest("POST", "/login", strings.NewReader("username=admin&password=admin1234"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// The attacker's session cookie, but the victim's token value.
	for _, c := range attackerSession {
		req.AddCookie(c)
	}
	req.Header.Set("X-Csrf-Token", victimToken)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Errorf("cross-session token: got status %d, want 403", resp.StatusCode)
	}
}

// Public list routes took c.Params("page") and only rejected < 1, so a huge
// page number became an offset that forces a full table scan.
func TestPublicPaginationIsClamped(t *testing.T) {
	app, db := newTestApp(t)
	for i := 0; i < 25; i++ {
		db.Create(&model.Post{
			Title:     "P" + strconv.Itoa(i),
			Content:   "x",
			Slug:      "p" + strconv.Itoa(i),
			Published: true,
		})
	}
	cat := model.Category{Name: "News", Slug: "news"}
	db.Create(&cat)
	db.Exec("INSERT INTO post_categories (post_id, category_id) SELECT id, ? FROM posts", cat.ID)

	for _, path := range []string{"/blog/99999999", "/blog/0", "/blog/-5", "/blog/abc",
		"/blog/category/news/99999999", "/blog/tag/nope/99999999"} {
		resp, body := do(t, app, "GET", path)
		if resp.StatusCode >= 500 {
			t.Errorf("GET %s: got status %d", path, resp.StatusCode)
		}
		if resp.StatusCode == fiber.StatusOK && len(body) > 400_000 {
			t.Errorf("GET %s: response was %d bytes; the page cap did not apply", path, len(body))
		}
	}
	// A real page still renders posts. Newest first, so page 1 is the highest
	// indices, not "P0".
	resp, body := do(t, app, "GET", "/blog/1")
	if resp.StatusCode != fiber.StatusOK || !strings.Contains(body, "/blog/post/p") {
		t.Errorf("blog page 1: got status %d, no post links", resp.StatusCode)
	}
}

// A one-character query would be a full scan of every content blob.
func TestSearchRequiresTwoCharacters(t *testing.T) {
	app, db := newTestApp(t)
	db.Create(&model.Post{Title: "Findable", Content: "needle here", Slug: "findable", Published: true})

	resp, body := do(t, app, "GET", "/search?q=a")
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("short search: got status %d", resp.StatusCode)
	}
	if strings.Contains(body, "Findable") {
		t.Error("a one-character query ran the search instead of being rejected")
	}
	if !strings.Contains(body, "at least 2 characters") {
		t.Error("short query gave no guidance")
	}
	if _, body := do(t, app, "GET", "/search?q=needle"); !strings.Contains(body, "Findable") {
		t.Error("a real query no longer finds anything")
	}
}

func TestAdminResponsesAreNotStored(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	req := httptest.NewRequest("GET", "/admin", nil)
	req.AddCookie(auth)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Header.Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Errorf("GET /admin Cache-Control = %q, want no-store", got)
	}
	if got := resp.Header.Get("Vary"); !strings.Contains(got, "Cookie") {
		t.Errorf("GET /admin Vary = %q, want Cookie", got)
	}
	// The extra hardening headers.
	if resp.Header.Get("Cross-Origin-Opener-Policy") != "same-origin" {
		t.Error("missing Cross-Origin-Opener-Policy")
	}
	if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "script-src-attr 'none'") {
		t.Error("CSP is missing script-src-attr 'none'")
	}
}

// database.InitDB appends the pragmas to the DSN and refuses to start if
// foreign keys are not actually on. GORM's DisableForeignKeyConstraintWhen-
// Migrating=false only creates the constraints; SQLite ignores them unless the
// PRAGMA says otherwise, so without the DSN flags the safety net the delete
// handlers rely on is absent.
func TestSQLiteForeignKeysAreEnforced(t *testing.T) {
	_, db := newTestApp(t)

	var fk int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&fk).Error; err != nil {
		t.Fatalf("could not read the pragma: %v", err)
	}
	if fk != 1 {
		t.Fatalf("PRAGMA foreign_keys = %d, want 1", fk)
	}

	// A comment whose post is deleted directly must be refused by the
	// database, not silently orphaned.
	admin := createUser(t, db, "boss", model.RoleAdmin)
	post := model.Post{Title: "FK", Content: "x", Slug: "fk", UserID: admin.ID, Published: true}
	db.Create(&post)
	db.Create(&model.Comment{Content: "hi", UserID: admin.ID, PostID: post.ID, Status: "approved"})

	if err := db.Delete(&model.Post{}, post.ID).Error; err == nil {
		t.Error("deleting a post with comments was allowed; foreign keys are inert")
	}
	var comments int64
	db.Model(&model.Comment{}).Where("post_id = ?", post.ID).Count(&comments)
	if comments != 1 {
		t.Errorf("comment was orphaned anyway: count = %d", comments)
	}
}

// sqliteDSN must produce a URI carrying every pragma, and must not clobber an
// override the operator already set.
func TestSQLiteDSNPragmas(t *testing.T) {
	got := database.SQLiteDSN("./data/database.sqlite")
	for _, want := range []string{"_foreign_keys=on", "_journal_mode=WAL", "_busy_timeout=5000", "_synchronous=NORMAL"} {
		if !strings.Contains(got, want) {
			t.Errorf("plain DSN missing %q: %s", want, got)
		}
	}
	if !strings.HasPrefix(got, "file:") {
		t.Errorf("plain path was not converted to a URI: %s", got)
	}

	// Already-a-URI, with a pragma already present, must not be duplicated.
	preset := "file:/tmp/x.db?_foreign_keys=on&cache=shared"
	got = database.SQLiteDSN(preset)
	if strings.Count(got, "_foreign_keys=on") != 1 {
		t.Errorf("existing pragma was duplicated: %s", got)
	}
	if !strings.Contains(got, "cache=shared") || !strings.Contains(got, "_journal_mode=WAL") {
		t.Errorf("existing params lost or new ones missing: %s", got)
	}
}

func TestMenuCreatorRendersNestedItems(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	parent := model.Menu{Title: "Header", Primary: true, Position: 1}
	db.Create(&parent)
	child := model.Menu{Title: "More", Position: 1, ParentID: &parent.ID}
	db.Create(&child)
	childID := child.ID
	db.Create(&model.MenuItem{Title: "TopLevel", Link: "/a", MenuID: &parent.ID, Position: 1})
	db.Create(&model.MenuItem{Title: "NestedDeep", Link: "/b", MenuID: &childID, Position: 1})

	_, body := do(t, app, "GET", "/search-menu", auth)
	// Submenu items were never preloaded before, so they rendered as an
	// empty heading. Both levels must now appear.
	for _, want := range []string{"TopLevel", "NestedDeep", "More", "menu-tree-nested"} {
		if !strings.Contains(body, want) {
			t.Errorf("menu tree missing %q", want)
		}
	}
	// The add-form and edit-modal controls must no longer share IDs.
	if strings.Count(body, `id="new-item-link"`) != 1 {
		t.Errorf("expected exactly one #new-item-link, got %d", strings.Count(body, `id="new-item-link"`))
	}
	if strings.Contains(body, `id="menu_item_link"`) {
		t.Error("stale duplicated #menu_item_link id still present")
	}
}

func TestMenuItemSearchMatchesItems(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)

	m := model.Menu{Title: "Alpha", Position: 1}
	other := model.Menu{Title: "Beta", Position: 2}
	db.Create(&m)
	db.Create(&other)
	id := m.ID
	db.Create(&model.MenuItem{Title: "Needle Item", Link: "/needle", MenuID: &id, Position: 1})

	// The field is labelled "search menus and items": an item match must keep
	// its parent menu visible.
	req := httptest.NewRequest("GET", "/search-menu?query=Needle", nil)
	req.AddCookie(auth)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Needle Item") {
		t.Error("searching by item title did not return the item")
	}
	if !strings.Contains(string(body), "Alpha") {
		t.Error("searching by item title did not keep the parent menu visible")
	}
}

func TestMenuCannotBeItsOwnParent(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

	m := model.Menu{Title: "Loop", Position: 1}
	db.Create(&m)
	sub := model.Menu{Title: "Sub", Position: 2, ParentID: &m.ID}
	db.Create(&sub)

	selfForm := url.Values{"menu_title": {"Loop"}, "parent_id": {strconv.Itoa(int(m.ID))}}
	if resp, _ := postForm(t, app, fmt.Sprintf("/edit-menu/%d", m.ID), selfForm, token, auth); resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("self parent: got status %d, want 400", resp.StatusCode)
	}
	// A menu must not be moved under its own descendant either.
	cycleForm := url.Values{"menu_title": {"Loop"}, "parent_id": {strconv.Itoa(int(sub.ID))}}
	if resp, _ := postForm(t, app, fmt.Sprintf("/edit-menu/%d", m.ID), cycleForm, token, auth); resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("cycle parent: got status %d, want 400", resp.StatusCode)
	}

	var fresh model.Menu
	db.First(&fresh, m.ID)
	if fresh.ParentID != nil {
		t.Errorf("menu ended up parented to %v", *fresh.ParentID)
	}
}

func TestMenuBuilderFlow(t *testing.T) {
	app, db := newTestApp(t)
	// Fresh test apps seed a primary menu; start clean for this flow.
	db.Exec("DELETE FROM menu_items")
	db.Exec("DELETE FROM menus")
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

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
	csp := resp.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "object-src 'none'") {
		t.Errorf("Content-Security-Policy missing object-src 'none': %q", csp)
	}
	// Scripts must not allow inline/eval code; styles keep unsafe-inline for
	// the Quill editor's runtime styles.
	scriptSrc := csp
	if i := strings.Index(csp, "script-src"); i >= 0 {
		scriptSrc = csp[i:]
		if j := strings.Index(scriptSrc, ";"); j >= 0 {
			scriptSrc = scriptSrc[:j]
		}
	}
	if strings.Contains(scriptSrc, "unsafe-inline") || strings.Contains(scriptSrc, "unsafe-eval") {
		t.Errorf("script-src must not allow inline/eval code: %q", scriptSrc)
	}
}

func TestLogoutRevokesJWT(t *testing.T) {
	app, db := newTestApp(t)
	createUser(t, db, "alice", model.RoleUser)

	jwtCookie, token := login(t, app, "alice", "password123")

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
	token := csrfCookies(t, app)
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

func deleteReq(t *testing.T, app *fiber.App, path string, csrfCookies []*http.Cookie, cookies ...*http.Cookie) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest("DELETE", path, nil)
	req.Header.Set("HX-Request", "true")
	for _, c := range csrfCookies {
		if c == nil {
			continue
		}
		if c.Name == "csrf_" {
			req.Header.Set("X-Csrf-Token", c.Value)
		}
		req.AddCookie(c)
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

func TestDeleteUserGuards(t *testing.T) {
	app, db := newTestApp(t)
	boss := createUser(t, db, "boss", model.RoleAdmin)
	other := createUser(t, db, "second", model.RoleAdmin)
	regular := createUser(t, db, "pleb", model.RoleUser)
	auth := authCookie(t, boss.ID)
	token := csrfCookies(t, app, auth)

	// Cannot delete yourself (also the last-admin guard's first line).
	if resp, _ := deleteReq(t, app, "/delete-user/"+strconv.Itoa(int(boss.ID)), token, auth); resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("delete self: got status %d, want 400", resp.StatusCode)
	}
	// Bad ID and missing user.
	if resp, _ := deleteReq(t, app, "/delete-user/abc", token, auth); resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("delete bad id: got status %d, want 400", resp.StatusCode)
	}
	if resp, _ := deleteReq(t, app, "/delete-user/99999", token, auth); resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("delete missing: got status %d, want 404", resp.StatusCode)
	}
	// Regular user and second admin can go (fresh apps also seed a default
	// admin, so boss plus the seed admin remain).
	if resp, _ := deleteReq(t, app, "/delete-user/"+strconv.Itoa(int(regular.ID)), token, auth); resp.StatusCode != fiber.StatusOK {
		t.Errorf("delete regular: got status %d, want 200", resp.StatusCode)
	}
	if resp, _ := deleteReq(t, app, "/delete-user/"+strconv.Itoa(int(other.ID)), token, auth); resp.StatusCode != fiber.StatusOK {
		t.Errorf("delete second admin: got status %d, want 200", resp.StatusCode)
	}
	var admins int64
	db.Model(&model.User{}).Where("role_id = ?", model.RoleAdmin).Count(&admins)
	if admins != 2 {
		t.Errorf("admins left = %d, want 2", admins)
	}
}

func TestChangePassword(t *testing.T) {
	app, db := newTestApp(t)
	createUser(t, db, "alice", model.RoleUser)

	before, token := login(t, app, "alice", "password123")

	change := url.Values{"current_password": {"password123"}, "new_password": {"newsecret1"}, "confirm_password": {"newsecret1"}}
	resp, _ := postForm(t, app, "/change-password", change, token, before)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("change password: got status %d", resp.StatusCode)
	}
	var fresh *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "jwt" {
			fresh = c
		}
	}
	if fresh == nil {
		t.Fatal("password change did not re-issue a jwt cookie")
	}

	// Mismatched confirmation is rejected (with the still-valid session).
	mismatch := url.Values{"current_password": {"newsecret1"}, "new_password": {"x"}, "confirm_password": {"y"}}
	if resp, _ := postForm(t, app, "/change-password", mismatch, token, fresh); resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("mismatch: got status %d, want 400", resp.StatusCode)
	}

	// The pre-change token was revoked with the other sessions.
	if resp, _ := do(t, app, "GET", "/account", before); !isDenied(resp.StatusCode) {
		t.Errorf("old token after password change: got status %d, want it rejected", resp.StatusCode)
	}

	// The new password works, the old one does not. A fresh token is needed
	// for the second attempt: the successful login above rotated the session,
	// which is the intended CSRF rotation.
	token2 := csrfCookies(t, app, fresh)
	if resp, _ := postForm(t, app, "/login", url.Values{"username": {"alice"}, "password": {"newsecret1"}}, token2); resp.StatusCode != fiber.StatusOK {
		t.Errorf("login with new password: got status %d, want 200", resp.StatusCode)
	}
	if resp, _ := postForm(t, app, "/login", url.Values{"username": {"alice"}, "password": {"password123"}}, csrfCookies(t, app)); resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("login with old password: got status %d, want 401", resp.StatusCode)
	}
}

func TestLegacyShopCleanup(t *testing.T) {
	_, db := newTestApp(t)

	db.Exec("CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT)")
	db.Exec("CREATE TABLE product_categories (id INTEGER PRIMARY KEY, name TEXT)")
	db.Exec("INSERT INTO plugins (name, author, version, enabled) VALUES ('ShopPlugin', 'x', '1.0', 0)")

	// Second startup pass runs the cleanup.
	setupFiberApp(db)

	for _, table := range []string{"products", "product_categories"} {
		if db.Migrator().HasTable(table) {
			t.Errorf("legacy table %s still exists", table)
		}
	}
	var n int64
	db.Model(&model.Plugin{}).Where("name = ?", "ShopPlugin").Count(&n)
	if n != 0 {
		t.Error("legacy ShopPlugin row still exists")
	}
}

func TestPostContentIsSanitized(t *testing.T) {
	app, db := newTestApp(t)
	admin := createUser(t, db, "boss", model.RoleAdmin)
	auth := authCookie(t, admin.ID)
	token := csrfCookies(t, app, auth)

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
	token := csrfCookies(t, app, auth)
	viper.Set("upload.max_size_mb", 1)

	var buf strings.Builder
	w := multipart.NewWriter(&buf)
	part, _ := w.CreateFormFile("file", "evil.PNG")
	part.Write([]byte("<html><script>alert(1)</script></html>"))
	w.Close()

	req := httptest.NewRequest("POST", "/upload-file", strings.NewReader(buf.String()))
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(auth)
	for _, c := range token {
		req.AddCookie(c)
		if c.Name == "csrf_" {
			req.Header.Set("X-Csrf-Token", c.Value)
		}
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("got status %d, want 400", resp.StatusCode)
	}
}
