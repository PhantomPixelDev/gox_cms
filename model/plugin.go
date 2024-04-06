package model

import (
	"time"
)

type Plugin struct {
	ID        uint       `json:"id" gorm:"primaryKey"`
	Name      string     `json:"name" validate:"required"`
	Author    string     `json:"author" validate:"required"`
	Version   string     `json:"version" validate:"required"`
	Enabled   bool       `json:"enabled" default:"false"`
	Settings  string     `json:"settings"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `gorm:"index"`
}
