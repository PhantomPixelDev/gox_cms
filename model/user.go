package model

import (
	"time"

	"github.com/go-playground/validator/v10"
)

// User struct with validation tags using go-playground validator
type User struct {
	ID        uint       `json:"id" gorm:"primaryKey"`
	Username  string     `form:"username" json:"username" validate:"required,alphanum,min=2,max=30"`
	Password  string     `json:"-" validate:"required,min=6"`
	RoleID    uint       `json:"role_id" validate:"required"`
	FirstName string     `form:"first_name" json:"first_name" validate:"required,min=2,max=30"`
	LastName  string     `form:"last_name" json:"last_name" validate:"required,min=2,max=30"`
	Email     *string    `form:"email" json:"email" validate:"omitempty,email"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `gorm:"index"`
}

// Role struct
type Role struct {
	ID   uint   `json:"id" gorm:"primaryKey"`
	Name string `json:"name" validate:"required"`
}

// Validation function
func ValidateStruct(v *validator.Validate, s interface{}) error {
	return v.Struct(s)
}
