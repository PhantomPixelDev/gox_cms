package handlers

import (
	"goxcms/model"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// postsPerPage is the page size for the blog list and its taxonomies.
const postsPerPage = 10

// pageWindow builds the pagination block that every list template reads.
//
// It used to be assembled inline, slightly differently, in each of the blog,
// category and tag handlers. Three copies meant a theme had to cope with
// whichever keys a given page happened to set, and a key that was missing from
// one page rendered as "0" in the template rather than failing loudly, so the
// drift showed up as a paginator that was subtly wrong on one page only.
//
// CurrentPage   the page being shown, 1-based
// TotalPagesInt how many pages there are, as a number
// TotalPages    the same as a []int, for templates that range over it
// PrevPage      previous page number, clamped at 1
// NextPage      next page number, clamped at TotalPagesInt
// HasPrev/HasNext whether those are real links
func pageWindow(pageNumber, totalPages int) fiber.Map {
	// pageCount returns 0 for an empty result set. A paginator that says
	// "page 1 of 0" is worse than one that says "page 1 of 1", and a
	// NextPage of 0 would link to page 0.
	if totalPages < 1 {
		totalPages = 1
	}
	if pageNumber < 1 {
		pageNumber = 1
	}
	if pageNumber > totalPages {
		pageNumber = totalPages
	}
	return fiber.Map{
		"CurrentPage":   pageNumber,
		"TotalPagesInt": totalPages,
		"TotalPages":    pageRange(1, totalPages),
		"PrevPage":      maxInt(pageNumber-1, 1),
		"NextPage":      minInt(pageNumber+1, totalPages),
		"HasPrev":       pageNumber > 1,
		"HasNext":       pageNumber < totalPages,
	}
}

// pageRange builds [from, to] inclusive.
//
// A template that wants a windowed paginator ("1 … 4 5 6 … 20") should derive
// it from TotalPagesInt rather than ranging over this list, which is what the
// existing paginators did and which produces a 20-link row on page 1 of a
// 20-page blog.
func pageRange(from, to int) []int {
	if to < from {
		return []int{}
	}
	out := make([]int, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, i)
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// listData merges the pagination window with caller-specific keys and adds the
// flags every public template expects. Every list page and fragment goes
// through here, so the contract is defined once.
func listData(c *fiber.Ctx, title string, pageNumber, totalPages int, extra fiber.Map) fiber.Map {
	data := listDataNoCtx(title, pageNumber, totalPages, extra)
	data["IsAdmin"] = c.Locals("isAdmin")
	data["IsLoggedIn"] = c.Locals("isLoggedin")
	data["Settings"] = c.Locals("Settings")
	return data
}

// listDataNoCtx is listData for callers with no *fiber.Ctx, namely the fragment
// data assembly. themeContext adds the per-request flags at render time.
func listDataNoCtx(title string, pageNumber, totalPages int, extra fiber.Map) fiber.Map {
	data := pageWindow(pageNumber, totalPages)
	for k, v := range extra {
		data[k] = v
	}
	data["Title"] = title
	return data
}

// countPublished returns how many published posts there are.
func countPublished(db *gorm.DB) int64 {
	var n int64
	db.Model(&model.Post{}).Where("published = ?", true).Count(&n)
	return n
}
