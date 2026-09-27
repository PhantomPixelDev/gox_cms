package handlers

import "testing"

func TestPageWindow(t *testing.T) {
	cases := []struct {
		name                     string
		page, total              int
		wantCurrent              int
		wantTotal                int
		wantPrev, wantNext       int
		wantHasPrev, wantHasNext bool
	}{
		{
			name: "middle page", page: 3, total: 10,
			wantCurrent: 3, wantTotal: 10, wantPrev: 2, wantNext: 4,
			wantHasPrev: true, wantHasNext: true,
		},
		{
			name: "first page has no previous", page: 1, total: 5,
			wantCurrent: 1, wantTotal: 5, wantPrev: 1, wantNext: 2,
			wantHasPrev: false, wantHasNext: true,
		},
		{
			name: "last page has no next", page: 5, total: 5,
			wantCurrent: 5, wantTotal: 5, wantPrev: 4, wantNext: 5,
			wantHasPrev: true, wantHasNext: false,
		},
		{
			// pageCount returns 0 for an empty result set. Reporting "page 1 of
			// 0" and linking NextPage to 0 is what a naive implementation does,
			// and it is the state of a brand-new install with no posts.
			name: "empty result set still reads as one page", page: 1, total: 0,
			wantCurrent: 1, wantTotal: 1, wantPrev: 1, wantNext: 1,
			wantHasPrev: false, wantHasNext: false,
		},
		{
			// An out-of-range request must not leak its own number into the
			// window, or the paginator links back to a page that redirects.
			name: "out-of-range page is clamped", page: 99, total: 3,
			wantCurrent: 3, wantTotal: 3, wantPrev: 2, wantNext: 3,
			wantHasPrev: true, wantHasNext: false,
		},
		{
			name: "zero and negative are clamped", page: 0, total: 4,
			wantCurrent: 1, wantTotal: 4, wantPrev: 1, wantNext: 2,
			wantHasPrev: false, wantHasNext: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pageWindow(tc.page, tc.total)
			checks := []struct {
				key  string
				want any
			}{
				{"CurrentPage", tc.wantCurrent},
				{"TotalPagesInt", tc.wantTotal},
				{"PrevPage", tc.wantPrev},
				{"NextPage", tc.wantNext},
				{"HasPrev", tc.wantHasPrev},
				{"HasNext", tc.wantHasNext},
			}
			for _, ck := range checks {
				if got[ck.key] != ck.want {
					t.Errorf("pageWindow(%d, %d)[%s] = %v, want %v",
						tc.page, tc.total, ck.key, got[ck.key], ck.want)
				}
			}

			// Every page of the blog carries a complete window: the list and
			// the count can never disagree.
			wantLen := tc.wantTotal
			list, ok := got["TotalPages"].([]int)
			if !ok {
				t.Fatalf("TotalPages is %T, want []int", got["TotalPages"])
			}
			if len(list) != wantLen {
				t.Errorf("TotalPages has %d entries, want %d", len(list), wantLen)
			}
			for i, n := range list {
				if n != i+1 {
					t.Errorf("TotalPages[%d] = %d, want %d", i, n, i+1)
				}
			}
		})
	}
}

func TestPageRange(t *testing.T) {
	cases := []struct {
		from, to int
		want     []int
	}{
		{1, 3, []int{1, 2, 3}},
		{1, 1, []int{1}},
		{4, 2, []int{}},
		{0, 0, []int{0}},
	}
	for _, tc := range cases {
		got := pageRange(tc.from, tc.to)
		if len(got) != len(tc.want) {
			t.Errorf("pageRange(%d, %d) = %v, want %v", tc.from, tc.to, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("pageRange(%d, %d) = %v, want %v", tc.from, tc.to, got, tc.want)
				break
			}
		}
	}
}

func TestMinMaxInt(t *testing.T) {
	if minInt(3, 5) != 3 || minInt(5, 3) != 3 {
		t.Error("minInt is wrong")
	}
	if maxInt(3, 5) != 5 || maxInt(5, 3) != 5 {
		t.Error("maxInt is wrong")
	}
}

// listData must not let a caller's extra keys overwrite the pagination
// contract, and must always set the flags a public template dereferences.
func TestListDataKeepsContract(t *testing.T) {
	data := listDataNoCtx("Blog", 2, 3, fiberMap("Posts", "x"))
	if data["Title"] != "Blog" {
		t.Errorf("Title = %v, want Blog", data["Title"])
	}
	if data["CurrentPage"] != 2 {
		t.Errorf("CurrentPage = %v, want 2", data["CurrentPage"])
	}
	if data["Posts"] != "x" {
		t.Errorf("extra key lost: %v", data["Posts"])
	}
}

func fiberMap(kv ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		if k, ok := kv[i].(string); ok {
			m[k] = kv[i+1]
		}
	}
	return m
}
