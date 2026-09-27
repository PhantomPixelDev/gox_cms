package utils

import (
	"bytes"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
	"sync"
)

// parseGuard hides templates that do not parse from the template engine.
//
// Why this exists: the engine is created over the whole views/ tree, and its
// load step parses every .html file it finds. That means one malformed template
// anywhere under views/ makes the entire load fail, and every page on the site
// returns 500 — including the admin page whose job is to explain that a theme
// is incomplete. A theme being written is therefore able to take the site down,
// which is the opposite of what a filesystem theme system is for.
//
// The guard wraps the filesystem: a file that fails to parse is reported as
// absent, so the engine's walk skips it and the rest of the site keeps working.
// The broken theme still shows up in the admin UI, listed and disabled, with the
// parse error attached.
//
// A file is parsed twice, once here and once by the engine. That is deliberate:
// reimplementing the engine's association logic to parse each file only once
// would duplicate exactly the cross-template resolution that html/template
// gets right, and getting it subtly wrong would break working templates. The
// cost is a few hundred microseconds on load, which happens at startup and on
// an explicit reload, never per request.
//
// parseVerdicts memoises the result per path, so a reload after a fix does not
// re-parse the whole tree, and a template is parsed once per load even though
// it is checked once and parsed once by the engine.
var parseVerdicts sync.Map // path string -> error (nil means "parses")

// ResetParseGuard forgets the cached verdicts. Call it after a reload so an
// edited template is re-checked.
func ResetParseGuard() {
	parseVerdicts.Range(func(k, _ any) bool {
		parseVerdicts.Delete(k)
		return true
	})
}

// ParseGuardFS wraps an http.FileSystem for the template engine, hiding .html
// files that fail to parse on their own.
func ParseGuardFS(base http.FileSystem, funcs template.FuncMap) http.FileSystem {
	return &guardedFS{base: base, funcs: funcs}
}

type guardedFS struct {
	base  http.FileSystem
	funcs template.FuncMap
}

// Open returns the file, or an empty stand-in when it does not parse.
//
// Returning fs.ErrNotExist instead looks like the obvious way to hide a file
// and does not work: the engine's loader walks the tree, then opens each file
// it found, and treats that open error as fatal for the whole load. An empty
// template is accepted silently, which is what is wanted here. The name is
// unchanged, so the theme still looks complete to the engine; the theme is
// separately reported as broken in the admin UI and cannot be activated there.
func (g *guardedFS) Open(name string) (http.File, error) {
	if g.isTemplate(name) && !g.parses(name) {
		return &emptyFile{name: name, base: g.base}, nil
	}
	return g.base.Open(name)
}

// isTemplate reports whether a path is one the engine would try to parse.
func (g *guardedFS) isTemplate(name string) bool {
	return strings.HasSuffix(name, ".html")
}

// parses reports whether the file at name is valid Go template syntax.
//
// It opens its own handle rather than reusing the caller's: the caller needs
// that handle unread and still open, and closing it here to reopen is exactly
// how this guard ended up handing the engine a closed file.
//
// Cross-template references are not resolved here: a template that calls
// {{template "x"}} is checked by the engine when it parses, against the set it
// has built. A lone {{template}} to a missing name is a parse error for the
// engine, and the engine is what reports it.
func (g *guardedFS) parses(name string) bool {
	if v, ok := parseVerdicts.Load(name); ok {
		if err, isErr := v.(error); isErr && err != nil {
			return false
		}
		return true
	}

	r, err := g.base.Open(name)
	if err != nil {
		// Not readable is the engine's problem to report, not a verdict.
		return true
	}
	defer r.Close()

	if st, err := r.Stat(); err == nil && st.IsDir() {
		return true
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		return true
	}

	verdict := checkTemplate(name, buf.Bytes(), g.funcs)
	parseVerdicts.Store(name, verdict)
	if verdict != nil {
		log.Printf("views: hiding unparseable template %s: %v", path.Clean(name), verdict)
	}
	return verdict == nil
}

// checkTemplate parses one file against a throwaway set.
//
// A stub for "embed" is registered because a layout's {{embed}} is filled in by
// the engine when it renders, not by the template itself; without the stub every
// layout would be reported as broken.
func checkTemplate(name string, body []byte, funcs template.FuncMap) error {
	stub := template.FuncMap{
		"embed": func() string { return "" },
	}
	merged := make(template.FuncMap, len(funcs)+len(stub))
	for k, v := range funcs {
		merged[k] = v
	}
	for k, v := range stub {
		merged[k] = v
	}

	// A per-file set, named as the engine would name it, so a {{define}} in one
	// file behaves the same here as it does there.
	base := strings.TrimSuffix(name, ".html")
	if _, err := template.New(base).Funcs(merged).Parse(string(body)); err != nil {
		return err
	}
	return nil
}

// emptyFile stands in for a template that does not parse. It reports the real
// file's metadata, so the engine logs the right path and size, but reads as zero
// bytes, which parses as an empty template.
type emptyFile struct {
	name string
	base http.FileSystem
}

func (e *emptyFile) Close() error { return nil }

func (e *emptyFile) Read([]byte) (int, error) { return 0, io.EOF }

func (e *emptyFile) Seek(int64, int) (int64, error) { return 0, nil }

func (e *emptyFile) Readdir(int) ([]fs.FileInfo, error) { return nil, nil }

func (e *emptyFile) Stat() (fs.FileInfo, error) {
	if real, err := e.base.Open(e.name); err == nil {
		defer real.Close()
		if st, err := real.Stat(); err == nil {
			return st, nil
		}
	}
	return nil, fs.ErrNotExist
}
