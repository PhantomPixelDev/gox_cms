package handlers

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
)

func ShowToastError(c *fiber.Ctx, message string) error {
	messageMap := map[string]string{"showToast": message}
	messageBytes, _ := json.Marshal(messageMap)
	c.Set("HX-Trigger", string(messageBytes))
	c.Status(fiber.StatusOK)

	return nil
}

func ShowToast(c *fiber.Ctx, message string) error {
	messageMap := map[string]string{"showToast": message}
	messageBytes, _ := json.Marshal(messageMap)
	c.Set("HX-Trigger", string(messageBytes))
	c.Status(fiber.StatusOK)

	return nil
}
