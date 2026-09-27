# Vendored third-party assets

Copies of the libraries the app serves itself instead of pulling from a CDN at
runtime. Vendoring removes a render-blocking third-party origin, lets the
Content-Security-Policy drop its CDN allowlist, and makes the exact version
part of the repository rather than a URL that can change underneath us.

| File | Upstream | Version | Integrity (SRI) |
|---|---|---|---|
| `htmx.min.js` | `https://unpkg.com/htmx.org@1.9.10/dist/htmx.min.js` | 1.9.10 | `sha384-D1Kt99CQMDuVetoL1lrYwg5t+9QdHe7NLX/SoJYkXDFfX37iInKRy5xSi8nO7UC` |

## Updating a library

1. Download the new version over the existing file.
2. Update the version in the table above and every `<script>`/`<link>` tag that
   references it.
3. If the CDN form is kept anywhere, update the `integrity` hash to match, or
   the browser will refuse the script.
4. Run `go test ./...` and load the affected pages; htmx in particular is used
   by every admin interaction.

## Still loaded from a CDN

These have not been vendored yet and are the remaining reason `script-src` and
`style-src` in `handler/security.go` allow list `cdn.jsdelivr.net`,
`unpkg.com`, `ajax.googleapis.com` and `cdnjs.cloudflare.com`:

- `bootstrap.bundle.min.js` (5.3.2) — admin panel and the `default` theme
- `bootstrap-icons` (1.11.3) — icon font
- `quill.js` + `quill.snow.css` (2.0.0-beta.0) — rich text editor
- `selectize.min.js` (0.15.2) and `jquery.min.js` (3.6.0) — tag/category pickers

Vendoring them is the remaining half of the "vendor everything" step; the theme
system does not depend on it, because a site theme only needs `htmx.min.js`
and `site.js`, both of which are first-party.
