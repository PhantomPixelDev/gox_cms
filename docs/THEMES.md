# Writing a theme

A theme is a directory of templates and a stylesheet. There is no build step, no
package manifest to register, and no code to change: create the directory, add
the files, and the theme appears in **Admin Dashboard → Settings → Template
set** on the next page load.

```
views/site/<name>/
  layout.html
  index.html
  search.html
  404.html
  blog/blog.html
  blog/blog_post.html
  blog/blog_category.html
  blog/blog_tag.html
  page/page.html
  page/page_sidebar.html
  page/page_fullwidth.html
  theme.json          (optional)

static/themes/<name>/
  style.css
  theme.js            (optional)
```

A theme is only selectable once all eleven templates exist and parse. Until then
it is listed, disabled, and annotated with which file is missing or which
template failed to parse — so a half-built theme tells you what is wrong with it
instead of silently breaking the site.

`theme.json` gives the theme a display name, version, author and description,
shown in the selector:

```json
{
  "name": "Simple",
  "version": "1.0.0",
  "author": "You",
  "description": "Plain HTML and CSS. No framework, no build step.",
  "framework": "none"
}
```

A malformed `theme.json` is a warning, not a rejection: a bad description should
not make a theme unusable.

## What a theme is allowed to assume

Two first-party scripts are always available. `partials/theme-head.html` loads
both, and using it saves you from getting the wiring wrong in a way that is
silent and expensive:

| Script | What it does |
|---|---|
| `/static/vendor/htmx.min.js` | htmx, vendored. Any `hx-*` attribute works. |
| `/static/js/site.js` | Injects the `X-Csrf-Token` header on every htmx request, shows toasts, applies the saved light/dark preference. |

`site.js` is not optional in practice. Without it every unsafe htmx call fails
with 403 "session expired", because the CSRF token is a session-bound header,
not a hidden form field.

`partials/theme-head.html` and `partials/theme-foot.html` also handle the meta
tags, the favicon, the theme stylesheet, and the toast elements that
`HX-Trigger` toasts need. `theme-foot.html` is not decoration: `site.js` looks
up `#toast` and `#toast-error` by id, so omit them and every toast in the app is
silently dropped.

The dark-mode toggle uses `data-theme-toggle`; `site.js` binds it and persists
the choice.

## Data every public page receives

Set on every render, in every view, by `handler.themeContext`:

| Key | Type | Notes |
|---|---|---|
| `.Settings` | `map[string]string` | Site name, tagline, footer text, SEO description, language, favicon, captcha flags, container class, and so on. |
| `.Theme.Name` | string | The active theme's directory name. |
| `.Theme.Assets` | string | `/static/themes/<name>`. Use it for your own assets. |
| `.Theme.Label` | string | Display name from `theme.json`, or the directory name. |
| `.IsLoggedIn` | bool | |
| `.IsAdmin` | bool | |
| `.Menu` | `handler.MenuData` | The navigation tree. See below. |

## The navigation

`.Menu` is data, not markup. The navbar used to be a Go string with
hard-coded Bootstrap classes, which locked every theme to Bootstrap markup. Now
each theme draws it:

```html
<ul>
  {{ range .Menu.Items }}
    {{ if .HasChildren }}
      <li>
        <span>{{ .Title }}</span>
        <ul>
          {{ range .Children }}
            <li><a href="{{ .Link }}">{{ .Title }}</a></li>
          {{ end }}
        </ul>
      </li>
    {{ else }}
      <li>
        <a href="{{ .Link }}" {{ if .Active }}aria-current="page"{{ end }}>{{ .Title }}</a>
      </li>
    {{ end }}
  {{ end }}
</ul>
```

`MenuData` has `Items []MenuNode`, `CurrentPath`, `IsAdmin`, `IsLoggedIn` and
`RegistrationOpen`. A `MenuNode` has `ID`, `Title`, `Link`, `Active`,
`HasChildren`, `Position` and `Children`. Titles and links are raw; escaping is
`html/template`'s job, so do not add `| safe` to them.

A submenu parent has no link of its own, so match on the children. A child match
also sets the parent `Active`, so a theme that only styles `.active` still looks
right. The tree is cached for a minute and invalidated by every menu change, so
an edit shows on the next page load.

`partials/menu.html` is the Bootstrap-flavoured default if you do not want to
write your own. A theme that draws its own nav can skip it entirely.

## Fragments

`hx-get` on a normal page URL injects a second complete `<html>` document into
the current one, which is why a framework-free theme could not do partial
updates at all. Fragments are the supported way: they render the same template
with **no layout**, so the response is exactly the markup to swap in.

| Endpoint | Template |
|---|---|
| `/frag/menu` | `partials/menu` |
| `/frag/blog?page=N` | `blog/blog` |
| `/frag/blog/category/:slug?page=N` | `blog/blog_category` |
| `/frag/blog/tag/:slug?page=N` | `blog/blog_tag` |
| `/frag/post/:slug` | `blog/blog_post` |
| `/frag/comments/:slug` | `partials/comments` |
| `/frag/page/:slug` | the page's own template |
| `/frag/search?q=...` | `search` |

Three rules that matter:

- A fragment **must not redirect**. `hx-get` follows a 302 and swaps the
  target's response — a whole page — into the panel. Out-of-range page numbers
  render an empty list instead.
- A fragment **must not return a whole document**, including when it fails. A
  missing slug answers 404 with the theme's `404` view rendered *without* the
  layout, so a swap in never splices in a second `<html>`. The status is still
  404, so your code and your monitoring can see it.
- The swap target and the pager have to move together. The fragment re-renders
  the whole list template, so the element that receives the swap must contain
  both the list and the paginator. The built-in themes wrap them in
  `<div id="post-list">` and use `hx-swap="outerHTML"`.

Page numbers in `href` must be **path segments**, not query parameters. The
routes are `/blog/:page?` and `/blog/category/:slug/:page?`, so
`/blog/category/guides?page=2` is ignored and returns page 1. The `hx-get` URL
may use `?page=` because that is a fragment.

## Pagination

Every list view receives the same keys, computed in `handler.pageWindow`:

| Key | Type | Notes |
|---|---|---|
| `.Posts` | `[]model.Post` | |
| `.CurrentPage` | int | 1-based |
| `.TotalPagesInt` | int | At least 1, even for an empty result set. |
| `.TotalPages` | `[]int` | `1..TotalPagesInt`. Provided for compatibility. |
| `.PageWindow` | `[]int` | A short run around the current page. Prefer this. |
| `.ShowFirstGap` / `.ShowLastGap` | bool | Whether to render an ellipsis. |
| `.PrevPage` / `.NextPage` | int | Clamped, so they are always valid links. |
| `.HasPrev` / `.HasNext` | bool | |

Range over `.PageWindow`, not `.TotalPages`. Ranging over every page put one
`<li>` per page on a long archive, for a control that shows five numbers.

The built-in paginators look like this:

```html
{{ if .ShowFirstGap }}<span>&hellip;</span>{{ end }}
{{ range .PageWindow }}
  <a href="/blog/{{ . }}" {{ if eq . $.CurrentPage }}aria-current="page"{{ end }}>{{ . }}</a>
{{ end }}
{{ if .ShowLastGap }}<span>&hellip;</span>{{ end }}
```

## List pages

| Template | Extra keys |
|---|---|
| `blog/blog` | — |
| `blog/blog_category` | `.Slug`, `.CategoryName` |
| `blog/blog_tag` | `.Slug`, `.TagName` |

`blog/blog_post` and `page/page*` read `.Title` and `.Content` directly rather
than a wrapper object.

## Search

`.Query`, `.TooShort`, `.Posts`, `.Pages`. The two slices are always present,
so a theme can range over them without a nil check. `.TooShort` is true when
the query is under two characters; the handler deliberately does not query the
database in that case, because a leading-wildcard `LIKE` on an unindexed content
column is a full table scan.

## Comments

`partials/comments.html` is **shared**, not per-theme, and styled from your
stylesheet. That is deliberate: Go templates cannot compute a template name, so
`{{template (printf "site/%s/blog/comments" .Theme.Name) .}}` is a parse error
and a per-theme copy could not be included from your post template. A template
that fails to parse also takes down every other theme, since they share one
template set.

The class names are the contract: `.comment-list`, `.comment`, `.comment-head`,
`.comment-author`, `.comment-date`, `.comment-body`, `.comment-empty`.

Include it from your post template:

```html
{{ template "partials/comments" . }}
```

and point your comment form at the fragment:

```html
<form hx-post="/add-comment" hx-swap="outerHTML" hx-target="#comments">
```

The comment *form* stays in your post template, so you control its markup.

## Template helpers

Available in every template. `utils.TemplateFuncMap` is the full list.

| Helper | Notes |
|---|---|
| `truncate s n` | Shorten a string. |
| `escape` / `unescape` | HTML-escape, and reverse it. |
| `add` / `sub` | Arithmetic. |
| `sequence from to` | `1..to`. |
| `window current total span` | A `[]int` of page numbers around `current`. The list views already receive `.PageWindow`, so you rarely need it. |
| `year`, `timestamp` | |
| `embed` | Filled in by the engine with the view being rendered. Only meaningful in a layout. |

## Two constraints worth knowing

**Named HTML entities are not valid in XHTML.** `&hellip;`, `&middot;` and
friends are not defined in XML without a DTD, and browsers do not load the
XHTML DTD, so they are a fatal parse error in the `xhtml` theme. Use numeric
references (`&#8230;`) or plain characters. The `xhtml` theme is the test case:
`TestXHTMLThemeOutputIsWellFormedXML` renders every public page and parses the
response with a strict XML parser.

**A bare attribute is invalid XML.** `data-theme-toggle` must be
`data-theme-toggle="true"` in an XHTML theme. HTML allows valueless attributes;
XML does not.

## Testing a theme

```sh
gofmt -w .
go vet ./...
go test ./...
```

The relevant tests, all of which a new theme has to pass:

- `TestXHTMLThemeOutputIsWellFormedXML` — strict XML parse of every page.
- `TestThemeOnDiskIsUsableWithoutRestart` — a complete theme works end to end,
  with no rebuild and no restart.
- `TestIncompleteThemeIsRejected` — a theme with a missing file and a parse
  error is listed, disabled, explained, and refused on activation.
- `TestThemePaginationUsesPathSegmentsNotQueryParams` — pagination links.
- `TestPaginatorIsWindowed` / `TestPaginatorHtmxTargetsFragments` — the pager.
- `TestMenuPartialEscapesAdminContent` — navigation content is escaped.

A theme that is being written cannot take the site down: `utils.ParseGuardFS`
hands the engine an empty template in place of one that does not parse, so the
rest of the tree loads and the broken theme is reported in the admin UI instead.
