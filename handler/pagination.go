package handlers

import (
	"math"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// queryPage returns the 1-based "page" query or form value, defaulting to 1.
func queryPage(c *fiber.Ctx) int {
	page, err := strconv.Atoi(c.Query("page", c.FormValue("page", "1")))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

// pageCount returns how many pages of pageSize items are needed for count
// items.
func pageCount(count int64, pageSize int) int {
	return int(math.Ceil(float64(count) / float64(pageSize)))
}
