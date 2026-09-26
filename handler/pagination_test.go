package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestPageCount(t *testing.T) {
	cases := []struct {
		count    int64
		pageSize int
		want     int
	}{
		{0, 10, 0},
		{1, 10, 1},
		{10, 10, 1},
		{11, 10, 2},
		{25, 10, 3},
	}
	for _, tc := range cases {
		if got := pageCount(tc.count, tc.pageSize); got != tc.want {
			t.Errorf("pageCount(%d, %d) = %d, want %d", tc.count, tc.pageSize, got, tc.want)
		}
	}
}

func TestEscapeLike(t *testing.T) {
	if got := escapeLike(`100%_x\y`); got != `100\%\_x\\y` {
		t.Errorf("escapeLike = %q", got)
	}
	if got := likePattern("a%b"); got != "%a\\%b%" {
		t.Errorf("likePattern = %q", got)
	}
	if got := likePattern("plain"); got != "%plain%" {
		t.Errorf("likePattern = %q", got)
	}
}

func TestQueryPage(t *testing.T) {
	cases := map[string]int{
		"/":         1,
		"/?page=3":  3,
		"/?page=0":  1,
		"/?page=-2": 1,
		"/?page=x":  1,
	}

	for target, want := range cases {
		app := fiber.New()
		var got int
		app.Get("/", func(c *fiber.Ctx) error {
			got = queryPage(c)
			return nil
		})
		if _, err := app.Test(httptest.NewRequest("GET", target, nil)); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("queryPage(%q) = %d, want %d", target, got, want)
		}
	}
}
