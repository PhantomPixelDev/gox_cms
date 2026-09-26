package handlers

import (
	"github.com/microcosm-cc/bluemonday"
)

// richHTMLPolicy allowlists the formatting produced by the Quill editor and
// strips everything else (scripts, event handlers, iframes, ...). Content is
// sanitized on save because post and custom-page bodies are rendered as raw
// HTML.
var richHTMLPolicy = bluemonday.UGCPolicy()

// SanitizeRichHTML strips dangerous markup from editor HTML while keeping
// safe formatting (links, lists, images, ...).
func SanitizeRichHTML(input string) string {
	return richHTMLPolicy.Sanitize(input)
}
