package handlers

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
)

// setToast sends an HX-Trigger header that views/main.html turns into a toast.
func setToast(c *fiber.Ctx, event, message string) error {
	messageBytes, _ := json.Marshal(map[string]string{event: message})
	c.Set("HX-Trigger", string(messageBytes))
	c.Status(fiber.StatusOK)

	return nil
}

// ShowToast shows an informational toast.
func ShowToast(c *fiber.Ctx, message string) error {
	return setToast(c, "showToast", message)
}

// ShowToastError shows an error toast.
func ShowToastError(c *fiber.Ctx, message string) error {
	return setToast(c, "ShowToastError", message)
}
