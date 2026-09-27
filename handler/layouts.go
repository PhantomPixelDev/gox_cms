package handlers

// Layout names used with c.Render. Fiber's html engine has no default layout,
// so every full-page render names one explicitly and HTMX fragment endpoints
// name none.
//
// PublicLayout wraps site pages with the navbar and footer.
// AdminLayout wraps the admin panel and its editors with a compact topbar and
// breadcrumb, so the public chrome (which has no link back to /admin) no
// longer sits on top of admin screens.
const (
	PublicLayout = "main"
	AdminLayout  = "layout-admin"
)
