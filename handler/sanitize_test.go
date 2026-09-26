package handlers

import (
	"strings"
	"testing"
)

func TestSanitizeRichHTML(t *testing.T) {
	in := `<script>alert(1)</script><p>Hello <a href="https://example.com" onclick="evil()">link</a></p><img src="x" onerror="evil()">`
	got := SanitizeRichHTML(in)

	for _, bad := range []string{"<script", "onclick", "onerror"} {
		if strings.Contains(got, bad) {
			t.Errorf("sanitized output still contains %q: %q", bad, got)
		}
	}
	for _, good := range []string{"<p>Hello", `href="https://example.com"`} {
		if !strings.Contains(got, good) {
			t.Errorf("sanitized output lost safe markup %q: %q", good, got)
		}
	}
}
