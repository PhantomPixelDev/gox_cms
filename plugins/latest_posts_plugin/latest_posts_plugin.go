package latest_posts_plugin

import (
	"fmt"
	"goxcms/model"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type LatestPostsPlugin struct{}

func (p *LatestPostsPlugin) Setup(app *fiber.App, db *gorm.DB) error {
	fmt.Println("LatestPosts Plugin setup")
	app.Get("/latest_posts_plugin", func(c *fiber.Ctx) error {
		posts := []model.Post{}
		postLimit := 5
		postQuery := db.Where("published = ?", true).Order("created_at desc").Limit(postLimit).Find(&posts)
		if postQuery.Error != nil {
			return c.Status(500).SendString("Error fetching latest posts")
		}

		htmlResponse := "<h3>Latest Posts Plugin</h3>"
		for _, post := range posts {
			htmlResponse += fmt.Sprintf("<h4><a href=\"%s\">%s</a></h4>", "/blog/post/"+post.Slug, post.Title)
		}

		return c.SendString(htmlResponse)
	})

	return nil
}

func (p *LatestPostsPlugin) Teardown() error {
	fmt.Println("LatestPostsPlugin teardown")
	return nil
}

func (p *LatestPostsPlugin) Name() string {
	return "LatestPostsPlugin"
}
