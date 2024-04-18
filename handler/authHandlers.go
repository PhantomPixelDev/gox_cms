package handlers

import (
	"fmt"
	"goxcms/model"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/google/uuid"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var jwtSecretKey = []byte(viper.GetString("app.secret"))

func GenerateJWT(userID uint) (string, error) {
	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)
	claims["user_id"] = userID
	claims["exp"] = time.Now().Add(time.Hour * 72).Unix()

	tokenString, err := token.SignedString(jwtSecretKey)
	return tokenString, err
}

// Function to validate CSRF token
func ValidateCSRFToken(c *fiber.Ctx) error {
	csrfToken := c.Locals("csrf").(string)
	submittedToken := c.FormValue("csrf")
	if csrfToken != submittedToken {
		return fiber.NewError(fiber.StatusForbidden, "CSRF token mismatch")
	}
	return nil
}

func ValidateJWT(c *fiber.Ctx) error {
	cookie := c.Cookies("jwt")

	token, err := jwt.Parse(cookie, func(token *jwt.Token) (interface{}, error) {
		return jwtSecretKey, nil
	})

	if err != nil || !token.Valid {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}

	return c.Next()
}

func ValidateAdmin(c *fiber.Ctx, db *gorm.DB) error {
	cookie := c.Cookies("jwt")

	token, err := jwt.Parse(cookie, func(token *jwt.Token) (interface{}, error) {
		return jwtSecretKey, nil
	})

	if err != nil || !token.Valid {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}

	claims := token.Claims.(jwt.MapClaims)
	userID := claims["user_id"].(float64)

	var user model.User
	db.First(&user, userID)

	if user.RoleID != 2 {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}

	c.Locals("isAdmin", true)

	return c.Next()
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
	cookie.SameSite = "Lax"
	cookie.Path = "/"
	c.Cookie(cookie)

}

func Login(db *gorm.DB, store *session.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
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

		/// csrf token generation
		csrfToken := GenerateCSRFToken()

		cookie := new(fiber.Cookie)
		cookie.Name = "csrf"
		cookie.Value = csrfToken
		cookie.HTTPOnly = true
		cookie.SameSite = "Lax"
		cookie.Path = "/"
		c.Cookie(cookie)

		c.Locals("user", user)
		c.Locals("isLoggedin", true)
		c.Locals("isAdmin", user.RoleID == uint(2))
		c.Locals("csrf", csrfToken)

		c.Set("HX-Redirect", "/")
		c.Status(fiber.StatusOK).SendString("Logged in successfully" + user.Username)
		return nil
	}
}

func GenerateCSRFToken() string {
	uuid := uuid.New()
	return uuid.String()
}

func Logout(c *fiber.Ctx) error {
	sess := c.Locals("session").(*session.Session)
	sess.Destroy()

	cookie := new(fiber.Cookie)
	cookie.Name = "jwt"
	cookie.Value = ""
	cookie.Expires = time.Now().Add(-1 * time.Hour)
	cookie.HTTPOnly = true

	/// remove the csrf cookie as well
	csrfCookie := new(fiber.Cookie)
	csrfCookie.Name = "csrf"
	csrfCookie.Value = ""
	csrfCookie.Expires = time.Now().Add(-1 * time.Hour)
	csrfCookie.HTTPOnly = true

	c.Cookie(csrfCookie)
	c.Cookie(cookie)
	c.Locals("user", nil)
	c.Locals("isLoggedin", false)
	c.Locals("isAdmin", false)
	c.Locals("session", nil)
	c.Locals("csrf", nil)
	// remove the csrf cookie

	c.Set("HX-Redirect", "/")

	c.Status(fiber.StatusOK).SendString("Logged out successfully")

	return nil
}

func Register(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var user model.User

		if err := c.BodyParser(&user); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
		}

		user.RoleID = uint(1)

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
		tokenString := c.Cookies("jwt")
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return jwtSecretKey, nil

		})

		if err != nil || !token.Valid {
			c.Locals("isLoggedin", false)
			return c.Next()
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.Locals("isLoggedin", false)
			return c.Next()
		}

		userID, ok := claims["user_id"].(float64)
		if !ok {
			c.Locals("isLoggedin", false)
			return c.Next()
		}

		var user model.User
		if err := db.First(&user, uint(userID)).Error; err != nil {
			c.Locals("isLoggedin", false)
			return c.Next()
		}

		c.Locals("isLoggedin", true)
		c.Locals("user", user)

		c.Locals("isAdmin", user.RoleID == uint(2))

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

func IsAdmin(c *fiber.Ctx) error {
	if c.Locals("isAdmin") == false {
		return c.Status(fiber.StatusUnauthorized).Redirect("/login")
	}
	return c.Next()
}

func IsLoggedIn(c *fiber.Ctx) error {
	if c.Locals("isLoggedin") == false {
		return c.Status(fiber.StatusUnauthorized).Redirect("/login")
	}
	return c.Next()
}
