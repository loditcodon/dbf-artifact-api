package models

import "time"

// DBGroupMgt represents the dbgroupmgt table.
// Manages database access groups with hierarchical structure support.
type DBGroupMgt struct {
	ID             uint      `gorm:"primaryKey;column:id" json:"id"`
	Name           string    `gorm:"column:name" json:"name" validate:"required"`
	Code           string    `gorm:"column:code;unique" json:"code" validate:"required"`
	Description    *string   `gorm:"column:description" json:"description,omitempty"`
	DatabaseTypeID *uint     `gorm:"column:database_type_id" json:"database_type_id,omitempty"`
	GroupType      string    `gorm:"column:group_type;type:enum('SYSTEM','CUSTOM')" json:"group_type"`
	ParentGroupID  *uint     `gorm:"column:parent_group_id" json:"parent_group_id,omitempty"`
	IsTemplate     bool      `gorm:"column:is_template" json:"is_template"`
	Metadata       *string   `gorm:"column:metadata" json:"metadata,omitempty"`
	IsActive       bool      `gorm:"column:is_active" json:"is_active"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updated_at"`
}

// TableName returns the database table name for DBGroupMgt model.
func (DBGroupMgt) TableName() string {
	return "dbgroupmgt"
}
