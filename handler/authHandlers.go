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
	client := resty.New()
	secret := viper.GetString("captcha.secret_key")
	resp, err := client.R().
		SetFormData(map[string]string{
			"secret":   secret,
			"response": hCaptchaResponse,
		}).
		Post("https://hcaptcha.com/siteverify")

	if err != nil {
		/// show toast error here
		return false, err
	}

	var result HCaptchaResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return false, err
	}

	return result.Success, nil
}

// jwtKey reads the signing secret on every call. It must not be captured in a
// package-level variable: package variables are initialised before main()
// loads the config file, which previously left the key empty.
func jwtKey() []byte {
	return []byte(viper.GetString("app.secret"))
}

const jwtLifetime = 72 * time.Hour

func GenerateJWT(userID uint) (string, error) {
	key := jwtKey()
	if len(key) == 0 {
		return "", errors.New("app.secret is not configured")
	}

	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(jwtLifetime).Unix(),
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
}

// parseJWT validates the token signature, algorithm and expiry and returns the
// user ID it was issued for.
func parseJWT(tokenString string) (uint, error) {
	key := jwtKey()
	if tokenString == "" || len(key) == 0 {
		return 0, errors.New("no token")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return key, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return 0, errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, errors.New("invalid claims")
	}

	userID, ok := claims["user_id"].(float64)
	if !ok || userID <= 0 {
		return 0, errors.New("invalid user_id claim")
	}

	return uint(userID), nil
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
	cookie.Expires = time.Now().Add(jwtLifetime)
	c.Cookie(cookie)
}

func Login(db *gorm.DB, store *session.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {

		capcha_enabled := viper.GetBool("captcha.enabled")
		if capcha_enabled {

			hCaptchaResponse := c.FormValue("h-captcha-response")

			if hCaptchaResponse == "" {
				ShowToastError(c, "CAPTCHA verification failed")
				return c.Status(fiber.StatusBadRequest).SendString("CAPTCHA verification failed")
			}

			valid, err := verifyHCaptcha(hCaptchaResponse)
			if err != nil {
				ShowToastError(c, "CAPTCHA verification failed")
				return c.Status(fiber.StatusInternalServerError).SendString("CAPTCHA verification failed")
			}

			if !valid {
				ShowToastError(c, "CAPTCHA verification failed")
				return c.Status(fiber.StatusBadRequest).SendString("CAPTCHA verification failed")
			}
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
			ShowToastError(c, "Invalid login credentials")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid login credentials"})
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
			ShowToastError(c, "Invalid login credentials - Password does not match")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid login credentials"})
		}

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

		tokenString, err := GenerateJWT(user.ID)
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

func Logout(c *fiber.Ctx) error {
	sess := c.Locals("session").(*session.Session)
	sess.Destroy()

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

func Register(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {

		hCaptchaResponse := c.FormValue("h-captcha-response")

		if hCaptchaResponse == "" {
			ShowToastError(c, "CAPTCHA verification failed")
			return c.Status(fiber.StatusBadRequest).SendString("CAPTCHA verification failed")
		}

		valid, err := verifyHCaptcha(hCaptchaResponse)
		if err != nil {
			ShowToastError(c, "CAPTCHA verification failed")
			return c.Status(fiber.StatusInternalServerError).SendString("CAPTCHA verification failed")
		}

		if !valid {
			ShowToastError(c, "CAPTCHA verification failed")
			return c.Status(fiber.StatusBadRequest).SendString("CAPTCHA verification failed")
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

		tokenString, err := GenerateJWT(user.ID)
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

		userID, err := parseJWT(c.Cookies("jwt"))
		if err != nil {
			return c.Next()
		}

		var user model.User
		if err := db.First(&user, userID).Error; err != nil {
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
