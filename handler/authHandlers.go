package handlers

import (
	"encoding/json"
	"errors"
	"goxcms/model"
	"goxcms/utils"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/go-resty/resty/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type HCaptchaResponse struct {
	Success     bool     `json:"success"`
	ChallengeTS string   `json:"challenge_ts"`
	Hostname    string   `json:"hostname"`
	ErrorCodes  []string `json:"error-codes,omitempty"`
}

func verifyHCaptcha(hCaptchaResponse string) (bool, error) {
	resp, err := resty.New().SetTimeout(10 * time.Second).R().
		SetFormData(map[string]string{
			"secret":   viper.GetString("captcha.secret_key"),
			"response": hCaptchaResponse,
		}).
		Post("https://hcaptcha.com/siteverify")
	if err != nil {
		return false, err
	}

	var result HCaptchaResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return false, err
	}

	return result.Success, nil
}

// captchaPassed verifies the hCaptcha token when captcha.enabled is set. On
// failure it writes the error response and returns false.
func captchaPassed(c *fiber.Ctx) bool {
	if !viper.GetBool("captcha.enabled") {
		return true
	}

	status := fiber.StatusBadRequest
	passed := false
	if token := c.FormValue("h-captcha-response"); token != "" {
		valid, err := verifyHCaptcha(token)
		if err != nil {
			status = fiber.StatusBadGateway
		}
		passed = err == nil && valid
	}

	if !passed {
		ShowToastError(c, "CAPTCHA verification failed")
		c.Status(status).SendString("CAPTCHA verification failed")
	}
	return passed
}

// jwtKey reads the signing secret on every call. It must not be captured in a
// package-level variable: package variables are initialised before main()
// loads the config file, which previously left the key empty.
func jwtKey() []byte {
	return []byte(viper.GetString("app.secret"))
}

// jwtLifetime bounds how long a login stays valid. Overridable with
// app.session_hours (default 12).
func jwtLifetime() time.Duration {
	if hours := viper.GetInt("app.session_hours"); hours > 0 {
		return time.Duration(hours) * time.Hour
	}
	return 12 * time.Hour
}

func GenerateJWT(userID, sessionVersion uint) (string, error) {
	key := jwtKey()
	if len(key) == 0 {
		return "", errors.New("app.secret is not configured")
	}

	claims := jwt.MapClaims{
		"user_id": userID,
		"sv":      sessionVersion,
		"exp":     time.Now().Add(jwtLifetime()).Unix(),
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
}

// parseJWT validates the token signature, algorithm and expiry and returns the
// user ID and session version it was issued for.
func parseJWT(tokenString string) (userID, sessionVersion uint, err error) {
	key := jwtKey()
	if tokenString == "" || len(key) == 0 {
		return 0, 0, errors.New("no token")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return key, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return 0, 0, errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, 0, errors.New("invalid claims")
	}

	id, ok := claims["user_id"].(float64)
	if !ok || id <= 0 {
		return 0, 0, errors.New("invalid user_id claim")
	}

	var sv uint
	if raw, ok := claims["sv"].(float64); ok && raw >= 0 {
		sv = uint(raw)
	}

	return uint(id), sv, nil
}

func FormatValidationError(err error) string {
	var errMessages []string

	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		for _, e := range validationErrors {
			errMessages = append(errMessages, formatSingleError(e))
		}
	}

	return strings.Join(errMessages, ", ")
}

func formatSingleError(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return e.Field() + " is required"
	case "min":
		return e.Field() + " must be at least " + e.Param() + " characters long"
	case "max":
		return e.Field() + " must be less than " + e.Param() + " characters long"
	case "alphanum":
		return e.Field() + " must be alphanumeric"
	default:
		return e.Field() + " is invalid"
	}
}

func SetJWTTokenCookie(c *fiber.Ctx, tokenString string) {
	cookie := new(fiber.Cookie)
	cookie.Name = "jwt"
	cookie.Value = tokenString
	cookie.HTTPOnly = true
	cookie.Secure = utils.SecureCookies()
	cookie.SameSite = "Lax"
	cookie.Path = "/"
	cookie.Expires = time.Now().Add(jwtLifetime())
	c.Cookie(cookie)
}

func Login(db *gorm.DB, store *session.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {

		if !captchaPassed(c) {
			return nil
		}

		if loginBlocked(c.IP()) {
			ShowToastError(c, "Too many login attempts, try again later")
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "Too many login attempts"})
		}

		type loginRequest struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}

		var req loginRequest
		if err := c.BodyParser(&req); err != nil {
			ShowToastError(c, "Invalid request body")
			return c.SendStatus(fiber.StatusBadRequest)
		}

		var user model.User
		if err := db.Where("username = ?", req.Username).First(&user).Error; err != nil {
			// Same cost as a real password check, so unknown usernames do
			// not fail visibly faster than wrong passwords.
			_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(req.Password))
			recordLoginFailure(c.IP())
			ShowToastError(c, "Invalid login credentials")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid login credentials"})
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
			recordLoginFailure(c.IP())
			ShowToastError(c, "Invalid login credentials - Password does not match")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid login credentials"})
		}

		resetLoginAttempts(c.IP())

		sess, err := store.Get(c)
		if err != nil {
			ShowToastError(c, "Failed to initiate session")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to initiate session"})
		}

		sess.Set("user_id", user.ID)

		if err := sess.Save(); err != nil {
			ShowToastError(c, "Failed to initiate session")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to initiate session"})
		}

		tokenString, err := GenerateJWT(user.ID, user.SessionVersion)
		if err != nil {
			ShowToastError(c, "Failed to generate token")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate token"})
		}

		SetJWTTokenCookie(c, tokenString)

		c.Locals("user", user)
		c.Locals("isLoggedin", true)
		c.Locals("isAdmin", user.RoleID == model.RoleAdmin)

		c.Set("HX-Redirect", "/")
		c.Status(fiber.StatusOK).SendString("Logged in successfully" + user.Username)
		return nil
	}
}

// Logout destroys the server session, revokes all JWTs issued to the user by
// bumping their session version, and clears the login cookie.
func Logout(c *fiber.Ctx, db *gorm.DB) error {
	if sess, ok := c.Locals("session").(*session.Session); ok && sess != nil {
		sess.Destroy()
	}

	if user, ok := CurrentUser(c); ok {
		db.Model(&model.User{}).Where("id = ?", user.ID).
			Update("session_version", gorm.Expr("session_version + 1"))
	}

	cookie := new(fiber.Cookie)
	cookie.Name = "jwt"
	cookie.Value = ""
	cookie.Expires = time.Now().Add(-1 * time.Hour)
	cookie.HTTPOnly = true
	cookie.Path = "/"

	c.Cookie(cookie)
	c.Locals("user", nil)
	c.Locals("isLoggedin", false)
	c.Locals("isAdmin", false)
	c.Locals("session", nil)

	c.Set("HX-Redirect", "/")

	c.Status(fiber.StatusOK).SendString("Logged out successfully")

	return nil
}

// registrationEnabled reports whether public sign-ups are turned on in
// Settings. Logins and existing accounts are unaffected.
func registrationEnabled(db *gorm.DB) bool {
	return SiteSettings(db)["RegistrationEnabled"] == "true"
}

func Register(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {

		if !registrationEnabled(db) {
			ShowToastError(c, "Registration is disabled")
			return c.Status(fiber.StatusForbidden).SendString("Registration is disabled")
		}

		if !captchaPassed(c) {
			return nil
		}

		var user model.User

		if err := c.BodyParser(&user); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
		}

		user.RoleID = model.RoleUser

		validate := validator.New()
		if err := validate.Struct(&user); err != nil {
			ShowToastError(c, "Validation failed: "+FormatValidationError(err))
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Validation failed", "details": err.Error()})
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
		if err != nil {
			ShowToastError(c, "Failed to hash password")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to hash password"})
		}
		user.Password = string(hashedPassword)

		if err := db.Where("username = ?", user.Username).First(&model.User{}).Error; err == nil {
			ShowToastError(c, "Username already exists")
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Username already exists"})
		}

		if err := db.Create(&user).Error; err != nil {
			ShowToastError(c, "Registration failed")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Registration failed"})
		}

		tokenString, err := GenerateJWT(user.ID, user.SessionVersion)
		if err != nil {
			ShowToastError(c, "Error generating token")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Error generating token"})
		}

		SetJWTTokenCookie(c, tokenString)

		c.Set("HX-Redirect", "/")
		return c.Status(fiber.StatusOK).SendString("Registered successfully")
	}
}

func AuthStatusMiddleware(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("isLoggedin", false)
		c.Locals("isAdmin", false)

		userID, sessionVersion, err := parseJWT(c.Cookies("jwt"))
		if err != nil {
			return c.Next()
		}

		var user model.User
		if err := db.First(&user, userID).Error; err != nil {
			return c.Next()
		}

		// Tokens issued before the latest logout (or any session-version
		// bump) are rejected even if their signature is still valid.
		if user.SessionVersion != sessionVersion {
			return c.Next()
		}

		c.Locals("isLoggedin", true)
		c.Locals("user", user)
		c.Locals("isAdmin", user.RoleID == model.RoleAdmin)

		return c.Next()
	}
}

func HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

// ChangePassword updates the logged-in user's password after verifying the
// current one. Other sessions are revoked via the session version; a fresh
// token is issued for this session so the user stays logged in here.
func ChangePassword(c *fiber.Ctx, db *gorm.DB) error {
	user, ok := CurrentUser(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).SendString("Not logged in")
	}

	current := c.FormValue("current_password")
	next := c.FormValue("new_password")
	confirm := c.FormValue("confirm_password")

	if next != confirm {
		ShowToastError(c, "New passwords do not match")
		return c.Status(fiber.StatusBadRequest).SendString("New passwords do not match")
	}
	if len(next) < 6 {
		ShowToastError(c, "New password must be at least 6 characters")
		return c.Status(fiber.StatusBadRequest).SendString("New password must be at least 6 characters")
	}

	var dbUser model.User
	if err := db.First(&dbUser, user.ID).Error; err != nil {
		ShowToastError(c, "User not found")
		return c.Status(fiber.StatusNotFound).SendString("User not found")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(dbUser.Password), []byte(current)); err != nil {
		ShowToastError(c, "Current password is incorrect")
		return c.Status(fiber.StatusUnauthorized).SendString("Current password is incorrect")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		ShowToastError(c, "Could not update password")
		return c.Status(fiber.StatusInternalServerError).SendString("Could not update password")
	}

	newVersion := dbUser.SessionVersion + 1
	if err := db.Model(&model.User{}).Where("id = ?", dbUser.ID).
		Updates(map[string]interface{}{"password": string(hash), "session_version": newVersion}).Error; err != nil {
		ShowToastError(c, "Could not update password")
		return c.Status(fiber.StatusInternalServerError).SendString("Could not update password")
	}

	tokenString, err := GenerateJWT(dbUser.ID, newVersion)
	if err != nil {
		ShowToastError(c, "Password changed, please log in again")
		return c.Status(fiber.StatusOK).SendString("Password changed, please log in again")
	}
	SetJWTTokenCookie(c, tokenString)

	ShowToast(c, "Password changed successfully")
	return c.Status(fiber.StatusOK).SendString("Password changed successfully")
}

// IsTrue reads a boolean local. Missing or non-bool values count as false, so
// a request that never went through AuthStatusMiddleware is never trusted.
func IsTrue(c *fiber.Ctx, key string) bool {
	v, _ := c.Locals(key).(bool)
	return v
}

// CurrentUser returns the logged-in user, if any.
func CurrentUser(c *fiber.Ctx) (model.User, bool) {
	user, ok := c.Locals("user").(model.User)
	return user, ok && IsTrue(c, "isLoggedin")
}

// denyAccess rejects a request. HTMX requests get an HX-Redirect header, plain
// page loads a redirect, and everything else a bare status code.
func denyAccess(c *fiber.Ctx, status int, redirectTo string) error {
	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", redirectTo)
		return c.SendStatus(status)
	}
	if c.Method() == fiber.MethodGet {
		return c.Redirect(redirectTo)
	}
	return c.SendStatus(status)
}

// IsLoggedIn only lets authenticated users through.
func IsLoggedIn(c *fiber.Ctx) error {
	if !IsTrue(c, "isLoggedin") {
		return denyAccess(c, fiber.StatusUnauthorized, "/login")
	}
	return c.Next()
}

// IsAdmin only lets authenticated administrators through. It does not rely on
// IsLoggedIn having run first.
func IsAdmin(c *fiber.Ctx) error {
	if !IsTrue(c, "isLoggedin") {
		return denyAccess(c, fiber.StatusUnauthorized, "/login")
	}
	if !IsTrue(c, "isAdmin") {
		return denyAccess(c, fiber.StatusForbidden, "/")
	}
	return c.Next()
}
