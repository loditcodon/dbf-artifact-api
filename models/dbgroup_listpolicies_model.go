package models

import "time"

// DBGroupListPolicies represents the dbgroup_listpolicies table.
// Contains policy definitions that can be applied to database groups.
type DBGroupListPolicies struct {
	ID                      uint      `gorm:"primaryKey;column:id" json:"id"`
	DatabaseTypeID          *uint     `gorm:"column:database_type_id" json:"database_type_id,omitempty"`
	CategoryID              *uint     `gorm:"column:category_id" json:"category_id,omitempty"`
	Name                    string    `gorm:"column:name" json:"name" validate:"required"`
	Code                    string    `gorm:"column:code" json:"code" validate:"required"`
	Description             *string   `gorm:"column:description" json:"description,omitempty"`
	RiskLevel               string    `gorm:"column:risk_level;type:enum('LOW','MEDIUM','HIGH','CRITICAL')" json:"risk_level"`
	IsDDL                   bool      `gorm:"column:is_ddl" json:"is_ddl"`
	IsDML                   bool      `gorm:"column:is_dml" json:"is_dml"`
	IsSystem                bool      `gorm:"column:is_system" json:"is_system"`
	DBPolicyDefaultID       *string   `gorm:"column:dbpolicydefault_id" json:"dbpolicydefault_id,omitempty"`
	ContainedPolicydefaults *string   `gorm:"column:contained_policydefaults" json:"contained_policydefaults,omitempty"`
	Metadata                *string   `gorm:"column:metadata" json:"metadata,omitempty"`
	IsActive                bool      `gorm:"column:is_active" json:"is_active"`
	CreatedAt               time.Time `gorm:"column:created_at" json:"created_at"`
}

// TableName returns the database table name for DBGroupListPolicies model.
func (DBGroupListPolicies) TableName() string {
	return "dbgroup_listpolicies"
}
