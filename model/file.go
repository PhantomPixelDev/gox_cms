package model

import (
	"time"

	"gorm.io/gorm"
)

type File struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"index:idx_name"`
	Extension string    `json:"extension"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func AddFile(file *File, db *gorm.DB) error {
	err := db.Create(file).Error
	return err
}
