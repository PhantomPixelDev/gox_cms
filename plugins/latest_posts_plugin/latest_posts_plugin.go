package latest_posts_plugin

import (
	"fmt"
	"goxcms/model"
	"html/template"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
	"gorm.io/gorm"
)

type LatestPostsPlugin struct{}

const (
	PluginName = "LatestPostsPlugin"
	Author     = "Ashba22"
	Version    = "1.0"
	Enabled    = false
)

func (p *LatestPostsPlugin) Setup(app *fiber.App, db *gorm.DB, engine *html.Engine) error {
	fmt.Println("LatestPosts Plugin setup")
	app.Get("/latest_posts_plugin", func(c *fiber.Ctx) error {
		/// if plugin is not enabled return 404
		if !p.Enabled(db) {
			return c.Status(404).SendString("Plugin not enabled")
		}
		posts := []model.Post{}
		postLimit := 5
		postQuery := db.Where("published = ?", true).Order("created_at desc").Limit(postLimit).Find(&posts)
		if postQuery.Error != nil {
			return c.Status(500).SendString("Error fetching latest posts")
		}

		tmpl := template.Must(template.New("latest_posts").Parse(`
			<p class="mb-4">Latest Posts PLUGIN</p>
			<div class="d-flex flex-wrap justify-content-center">
				{{range .}}
				<div class="m-2 bg-body rounded shadow">
					<a href="/blog/post/{{.Slug}}" class="text-decoration-none d-block">
						<img src="{{.ImageURL}}" class="w-100" style="max-height: 200px; object-fit: cover;">
						<div class="p-3">
							<h3 style="font-size: 1.2rem; font-weight: bold;" class="text-secondary">
								<a href="/blog/post/{{.Slug}}" class="text-decoration-none">{{.Title}}</a>
							</h3>
						</div>
					</a>
				</div>
				{{end}}
			</div>
		`))

		err := tmpl.Execute(c.Response().BodyWriter(), posts)
		if err != nil {
			return c.Status(500).SendString("Error rendering latest posts")
		}

		return nil
	})

	return nil
}

func (p *LatestPostsPlugin) Teardown() error {
	fmt.Println("LatestPostsPlugin teardown")
	return nil
}

func (p *LatestPostsPlugin) Name() string {
	return PluginName
}

func (p *LatestPostsPlugin) Author() string {
	return Author
}
func (p *LatestPostsPlugin) DefaultSettings() map[string]string {
	return map[string]string{}
}

func (p *LatestPostsPlugin) Settings(db *gorm.DB) map[string]string {
	plugin := &model.Plugin{}
	db.Where("name = ?", PluginName).First(plugin)
	return map[string]string{
		"Enabled": fmt.Sprintf("%t", plugin.Enabled),
	}
}

func (p *LatestPostsPlugin) Version() string {
	return Version
}

func (p *LatestPostsPlugin) Enabled(db *gorm.DB) bool {
	/// get status from database
	plugin := &model.Plugin{}
	db.Where("name = ?", PluginName).First(plugin)
	fmt.Println(PluginName, "enabled status:", plugin.Enabled)
	return plugin.Enabled
}
