package handlers

import (
	"log"
	"sync"

	"github.com/gofiber/template/html/v2"
)

// Template engine handle, used only for the rare "re-read templates from disk"
// operations. The Fiber html engine freezes its template set after the first
// load, so a theme created on disk would otherwise need a process restart.
var engineRef struct {
	sync.RWMutex
	engine *html.Engine
}

// SetTemplateEngine records the engine so themes can be re-registered without
// a restart. Safe to call with nil.
func SetTemplateEngine(e *html.Engine) {
	engineRef.Lock()
	engineRef.engine = e
	engineRef.Unlock()
}

// ReloadTemplates makes the engine re-read the views directory on the next
// render.
//
// It clears Engine.Loaded, which is the only supported lever: Load() short
// circuits on it, so /clear-cache could never pick up an edited template
// before. Reload is also toggled off, because leaving ShouldReload on would
// re-parse the whole template tree on every single request.
func ReloadTemplates() {
	engineRef.RLock()
	e := engineRef.engine
	engineRef.RUnlock()
	if e == nil {
		return
	}
	e.Reload(false)
	e.Loaded = false
	log.Println("templates: marked for reload from disk")
}
