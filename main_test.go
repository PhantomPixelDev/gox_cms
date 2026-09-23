package main

import (
	"io"
	"net/http"
	"net/http/httptest"
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
	token, err := handlers.GenerateJWT(userID)
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
	{"POST", "/admin/plugins/enable/ShopPlugin"},
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
