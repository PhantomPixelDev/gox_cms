package handlers

import (
	"math"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// maxPageNumber bounds offsets: page 1000 at 20 per page is already more
// rows than any admin table should scan through.
const maxPageNumber = 1000

// queryPage returns the 1-based "page" query or form value, defaulting to 1.
func queryPage(c *fiber.Ctx) int {
	page, err := strconv.Atoi(c.Query("page", c.FormValue("page", "1")))
	if err != nil || page < 1 {
		return 1
	}
	if page > maxPageNumber {
		return maxPageNumber
	}
	return page
}

// pageCount returns how many pages of pageSize items are needed for count
// items.
func pageCount(count int64, pageSize int) int {
	return int(math.Ceil(float64(count) / float64(pageSize)))
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// escapeLike escapes the LIKE wildcards in user input so admin searches match
// literally instead of treating % and _ as wildcards.
func escapeLike(s string) string {
	return likeEscaper.Replace(s)
}

// likePattern wraps escaped user input for a LIKE query. Use with an
// explicit ESCAPE clause: Where("col LIKE ? ESCAPE '\\'", likePattern(q)).
func likePattern(s string) string {
	return "%" + escapeLike(s) + "%"
}

// isDupKeyError reports whether err is a unique-constraint violation on any
// supported driver (sqlite, postgres, mysql).
func isDupKeyError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "Duplicate entry")
}
