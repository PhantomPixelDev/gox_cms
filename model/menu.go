package model

import (
	"time"
)

// Menu represents the structure for a menu.
type Menu struct {
	ID         uint        `json:"id" gorm:"primaryKey"`
	Title      string      `json:"title"`
	MenuItems  []*MenuItem `json:"menu_items" gorm:"foreignKey:MenuID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	SubMenus   []*Menu     `json:"sub_menus" gorm:"foreignKey:ParentID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	ItemsCount int         `json:"items_count" gorm:"-"` // Ignored field
	ParentID   *uint       `json:"parent_id"`            // Pointer to allow null (zero value)
	Slug       string      `json:"slug"`
	Primary    bool        `json:"primary" gorm:"column:is_primary"`
	Position   int         `json:"position" gorm:"index:idx_menu_position,sort:asc"` // Position field for ordering menus
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

// MenuItem represents the structure for an item in a menu.
type MenuItem struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Title     string    `json:"title"`
	Link      string    `json:"link"`
	MenuID    *uint     `json:"menu_id"`                                          // Pointer to allow null (zero value)
	Position  int       `json:"position" gorm:"index:idx_item_position,sort:asc"` // Position field for ordering items within a menu
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MenuRepository defines the interface for menu repository operations.
type MenuRepository interface {
	FindAll() ([]*Menu, error)
	FindByID(id uint) (*Menu, error)
	FindBySlug(slug string) (*Menu, error)
	Create(menu *Menu) (*Menu, error)
	Update(menu *Menu) (*Menu, error)
	Delete(id uint) error
	FindByParentID(parentID uint) ([]*Menu, error) // Method to find sub-menus
}
