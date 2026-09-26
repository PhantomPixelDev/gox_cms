package model

import (
	"time"
)

type Post struct {
	ID         uint       `json:"id" gorm:"primaryKey"`
	Title      string     `json:"title"`
	Content    string     `json:"content"`
	UserID     uint       `json:"user_id"`
	Categories []Category `json:"categories" gorm:"many2many:post_categories;"`
	Tags       []Tag      `json:"tags" gorm:"many2many:post_tags;"`
	Slug       string     `json:"slug" gorm:"uniqueIndex"`
	ImageURL   string     `json:"image_url"`
	Published  bool       `json:"published" gorm:"default:false;index"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type Category struct {
	ID         uint   `json:"id" gorm:"primaryKey"`
	Name       string `json:"name"`
	Slug       string `json:"slug" gorm:"uniqueIndex"`
	Posts      []Post `json:"posts" gorm:"many2many:post_categories;"`
	PostsCount int    `json:"posts_count" gorm:"-"`
}

type Tag struct {
	ID         uint   `json:"id" gorm:"primaryKey"`
	Name       string `json:"name"`
	Slug       string `json:"slug" gorm:"uniqueIndex"`
	Posts      []Post `json:"posts" gorm:"many2many:post_tags;"`
	PostsCount int    `json:"posts_count" gorm:"-"`
}

type Comment struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Content   string    `json:"content"`
	UserID    uint      `json:"user_id" gorm:"index"`
	User      User      `json:"user" gorm:"foreignKey:UserID"`
	PostID    uint      `json:"post_id" gorm:"index:idx_comments_post_status"`
	Status    string    `json:"status" gorm:"default:pending;index:idx_comments_post_status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
