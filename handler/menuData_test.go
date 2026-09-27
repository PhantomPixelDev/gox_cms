package handlers

import "testing"

// menuTitle returns the top-level titles, for readable assertions.
func menuTitles(items []MenuNode) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Title
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestMarkMenuActive(t *testing.T) {
	items := []MenuNode{
		{Title: "Home", Link: "/"},
		{Title: "Blog", Link: "/blog"},
		{Title: "More", HasChildren: true, Children: []MenuNode{
			{Title: "About", Link: "/about"},
		}},
	}

	cases := []struct {
		path      string
		wantTop   []string
		wantChild string
	}{
		// Exact match on a top-level link.
		{path: "/blog", wantTop: []string{"Blog"}},
		// A trailing slash must not lose the highlight: /blog/ and /blog are
		// the same page to a visitor, and previously only one matched.
		{path: "/blog/", wantTop: []string{"Blog"}},
		// The root normalises to "/".
		{path: "/", wantTop: []string{"Home"}},
		// A child match highlights the child and its parent, so a theme that
		// only styles .active on top-level items still looks right.
		{path: "/about", wantTop: []string{"More"}, wantChild: "About"},
		// No match leaves everything inactive rather than mis-highlighting.
		{path: "/nope", wantTop: nil},
	}

	for _, tc := range cases {
		// Deep copy, exactly as BuildMenuData does, because the caller owns a
		// per-request copy and the test must not see mutation leaking between
		// cases.
		marked := deepCopyMenu(items)
		markMenuActive(marked, tc.path)

		var got []string
		child := ""
		for _, it := range marked {
			if it.Active {
				got = append(got, it.Title)
			}
			for _, ch := range it.Children {
				if ch.Active {
					child = ch.Title
				}
			}
		}
		if !equalStrings(got, tc.wantTop) {
			t.Errorf("markMenuActive(%q) active top-level = %v, want %v", tc.path, got, tc.wantTop)
		}
		if child != tc.wantChild {
			t.Errorf("markMenuActive(%q) active child = %q, want %q", tc.path, child, tc.wantChild)
		}
	}
}

// The cached tree is shared between requests, so marking must never write back
// into it. A regression here would show one visitor's page as the next
// visitor's active menu item. The child case is the one a shallow copy misses,
// which is why it is the one that matters.
func TestBuildMenuDataDoesNotMutateCachedTree(t *testing.T) {
	shared := []MenuNode{
		{Title: "Blog", Link: "/blog"},
		{Title: "More", HasChildren: true, Children: []MenuNode{{Title: "About", Link: "/about"}}},
	}

	marked := deepCopyMenu(shared)
	markMenuActive(marked, "/about")

	if !marked[1].Active || !marked[1].Children[0].Active {
		t.Fatal("test setup: expected the copy to be marked")
	}
	if shared[0].Active || shared[1].Active || shared[1].Children[0].Active {
		t.Error("markMenuActive wrote back into the shared tree")
	}

	// A second request for an unrelated path must not inherit the first
	// request's highlight.
	second := deepCopyMenu(shared)
	markMenuActive(second, "/blog")
	if !second[0].Active {
		t.Error("second request did not mark its own path")
	}
	if second[1].Active || second[1].Children[0].Active {
		t.Error("second request inherited the first request's highlight")
	}
}

func TestNormalisePath(t *testing.T) {
	cases := map[string]string{
		"":            "/",
		"/":           "/",
		"/blog":       "/blog",
		"/blog/":      "/blog",
		"/blog///":    "/blog",
		"/a/b":        "/a/b",
		"/a/b/":       "/a/b",
		"/search?q=1": "/search?q=1",
	}
	for in, want := range cases {
		if got := normalisePath(in); got != want {
			t.Errorf("normalisePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMenuTitlesHelperSanity(t *testing.T) {
	items := []MenuNode{{Title: "A"}, {Title: "B"}, {Title: "C"}}
	got := menuTitles(items)
	if !equalStrings(got, []string{"A", "B", "C"}) {
		t.Errorf("menuTitles = %v", got)
	}
}
