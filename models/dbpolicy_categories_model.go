package models

import "time"

// DBPolicyCategories represents the dbpolicy_categories table.
// Defines categories for organizing database policies by functional area.
type DBPolicyCategories struct {
	ID            uint      `gorm:"primaryKey;column:id" json:"id"`
	Name          string    `gorm:"column:name" json:"name" validate:"required"`
	Code          string    `gorm:"column:code;unique" json:"code" validate:"required"`
	Description   *string   `gorm:"column:description" json:"description,omitempty"`
	PriorityLevel int       `gorm:"column:priority_level" json:"priority_level"`
	IsActive      bool      `gorm:"column:is_active" json:"is_active"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
}

// TableName returns the database table name for DBPolicyCategories model.
func (DBPolicyCategories) TableName() string {
	return "dbpolicy_categories"
}
