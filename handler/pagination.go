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
	return clampPage(c.Query("page", c.FormValue("page", "1")))
}

// clampPage parses and bounds a page number.
//
// The public list routes used to read c.Params("page") themselves and only
// checked for < 1, so /blog/99999999 produced an offset of 999999980: an
// unauthenticated request that forces SQLite to scan the whole posts table,
// plus a second unindexed COUNT(*). Every paginated route now goes through
// here.
func clampPage(raw string) int {
	page, err := strconv.Atoi(strings.TrimSpace(raw))
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

// parseIDParam reads a numeric id from a form or query value.
//
// It exists because GORM treats a *string* argument to First/Find as raw SQL
// when it is not a bare integer: db.First(&x, c.Query("id")) with
// id="1 OR 1=1" becomes the WHERE clause verbatim. Worse, an *empty* id makes
// GORM emit no condition at all, so the query silently matches the first row
// and the handler mutates or deletes it. Both are now rejected.
func parseIDParam(raw string) (uint, bool) {
	n, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil || n == 0 {
		return 0, false
	}
	return uint(n), true
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
