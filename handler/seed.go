package handlers

import (
	"io"
	"log"
	"os"
	"path/filepath"

	"goxcms/model"

	"gorm.io/gorm"
)

// seedImages are bundled under static/seed and copied into the uploads dir on
// first start so the file manager and demo posts have real media.
var seedImages = []string{"seed-1.jpg", "seed-2.jpg", "seed-3.jpg"}

// SeedDemoContent fills a fresh install with demo categories, tags, posts,
// comments, a primary menu, an about page and sample media. Every section is
// gated on its own table being empty, so it never touches existing content.
func SeedDemoContent(db *gorm.DB) {
	cleanupLegacyShop(db)
	seedMedia(db)
	seedPosts(db)
	seedMenu(db)
	seedPages(db)
}

// cleanupLegacyShop drops the tables and plugin row left behind by the
// removed ShopPlugin. It only affects pre-removal databases; on fresh
// installs every statement is a no-op.
func cleanupLegacyShop(db *gorm.DB) {
	db.Exec("DROP TABLE IF EXISTS products")
	db.Exec("DROP TABLE IF EXISTS product_categories")
	db.Exec("DELETE FROM plugins WHERE name = ?", "ShopPlugin")
}

func seedAuthor(db *gorm.DB) (model.User, bool) {
	var user model.User
	if err := db.Where("role_id = ?", model.RoleAdmin).First(&user).Error; err != nil {
		if err := db.First(&user).Error; err != nil {
			return user, false
		}
	}
	return user, true
}

func seedMedia(db *gorm.DB) {
	var count int64
	db.Model(&model.File{}).Count(&count)
	if count > 0 {
		return
	}

	if err := os.MkdirAll(UploadDir, 0o755); err != nil {
		log.Printf("seed: creating upload dir: %v", err)
		return
	}

	for _, name := range seedImages {
		src, err := os.Open(filepath.Join("static", "seed", name))
		if err != nil {
			continue
		}
		dst, err := os.Create(filepath.Join(UploadDir, name))
		if err != nil {
			src.Close()
			continue
		}
		_, copyErr := io.Copy(dst, src)
		src.Close()
		dst.Close()
		if copyErr != nil {
			continue
		}
		db.Create(&model.File{Name: name, Extension: ".jpg", Path: "/static/uploads/" + name})
	}
	log.Println("seed: demo media ready")
}

func seedPosts(db *gorm.DB) {
	var count int64
	db.Model(&model.Post{}).Count(&count)
	if count > 0 {
		return
	}

	author, ok := seedAuthor(db)
	if !ok {
		return
	}

	categories := []model.Category{
		{Name: "Getting Started", Slug: "getting-started"},
		{Name: "Tutorials", Slug: "tutorials"},
		{Name: "News", Slug: "news"},
	}
	db.Create(&categories)

	tags := []model.Tag{
		{Name: "cms", Slug: "cms"},
		{Name: "golang", Slug: "golang"},
		{Name: "htmx", Slug: "htmx"},
		{Name: "tutorial", Slug: "tutorial"},
	}
	db.Create(&tags)

	bySlug := func(items []model.Category, slugs ...string) []model.Category {
		var out []model.Category
		for _, c := range items {
			for _, s := range slugs {
				if c.Slug == s {
					out = append(out, c)
				}
			}
		}
		return out
	}
	tagBySlug := func(slugs ...string) []model.Tag {
		var out []model.Tag
		for _, t := range tags {
			for _, s := range slugs {
				if t.Slug == s {
					out = append(out, t)
				}
			}
		}
		return out
	}

	posts := []model.Post{
		{
			Title:      "Welcome to GoX CMS",
			Slug:       "welcome-to-gox-cms",
			Content:    SanitizeRichHTML(`<p>This is your first post. Everything on this site — posts, categories, tags, comments, menus, pages and media — is managed from the <a href="/admin">admin panel</a>.</p><p>Try editing this post, adding a tag, or switching the theme under <strong>Admin → Settings</strong>.</p>`),
			ImageURL:   "/static/uploads/seed-1.jpg",
			UserID:     author.ID,
			Published:  true,
			Categories: bySlug(categories, "getting-started"),
			Tags:       tagBySlug("cms", "golang"),
		},
		{
			Title:      "Writing Posts with Rich Text",
			Slug:       "writing-posts-with-rich-text",
			Content:    SanitizeRichHTML(`<p>Posts are written with the Quill rich-text editor. Formatting like <strong>bold</strong>, <em>italic</em>, lists and links is preserved, while dangerous markup such as scripts is stripped automatically on save.</p><ul><li>Headings, quotes and code blocks</li><li>Images from the file manager</li><li>Tags and categories for discovery</li></ul>`),
			ImageURL:   "/static/uploads/seed-2.jpg",
			UserID:     author.ID,
			Published:  true,
			Categories: bySlug(categories, "tutorials"),
			Tags:       tagBySlug("tutorial", "htmx"),
		},
		{
			Title:      "Fresh Look, Same Speed",
			Slug:       "fresh-look-same-speed",
			Content:    SanitizeRichHTML(`<p>The site now ships with the Flatly theme, a cleaned-up homepage and a refreshed admin panel with an overview dashboard. Under the hood: sanitized HTML, shorter sessions with logout revocation, login throttling and nightly backups.</p>`),
			ImageURL:   "/static/uploads/seed-3.jpg",
			UserID:     author.ID,
			Published:  true,
			Categories: bySlug(categories, "news"),
			Tags:       tagBySlug("cms"),
		},
	}
	if err := db.Create(&posts).Error; err != nil {
		log.Printf("seed: creating posts: %v", err)
		return
	}

	db.Create(&model.Comment{Content: "Looks great! Excited to try the new admin panel.", UserID: author.ID, PostID: posts[0].ID, Status: "approved"})
	db.Create(&model.Comment{Content: "The Quill editor walkthrough was exactly what I needed.", UserID: author.ID, PostID: posts[1].ID, Status: "approved"})
	log.Println("seed: demo posts, comments ready")
}

func seedMenu(db *gorm.DB) {
	var count int64
	db.Model(&model.Menu{}).Count(&count)
	if count > 0 {
		return
	}

	menu := model.Menu{Title: "Primary", Slug: "primary", Primary: true, Position: 1}
	if err := db.Create(&menu).Error; err != nil {
		log.Printf("seed: creating menu: %v", err)
		return
	}
	items := []model.MenuItem{
		{Title: "Home", Link: "/", MenuID: &menu.ID, Position: 1},
		{Title: "Blog", Link: "/blog", MenuID: &menu.ID, Position: 2},
		{Title: "About", Link: "/about", MenuID: &menu.ID, Position: 3},
	}
	db.Create(&items)
	log.Println("seed: primary menu ready")
}

func seedPages(db *gorm.DB) {
	var count int64
	db.Model(&model.CustomPage{}).Count(&count)
	if count > 0 {
		return
	}

	db.Create(&model.CustomPage{
		Title:     "About",
		Slug:      "about",
		Template:  "page",
		Published: true,
		Content:   SanitizeRichHTML(`<p>GoX CMS is a fast content management system built with Go, Fiber and HTMX. This is a demo custom page — edit it under <strong>Admin → Custom Pages</strong>, or create your own. Pages go live the moment you save them.</p>`),
	})
	log.Println("seed: about page ready")
}
