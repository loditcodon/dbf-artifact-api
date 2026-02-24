package models

import "time"

// DBPolicyGroups represents the dbpolicy_groups table.
// Links specific policies to database groups with validity periods.
type DBPolicyGroups struct {
	ID                    uint       `gorm:"primaryKey;column:id" json:"id"`
	DBGroupListPoliciesID uint       `gorm:"column:dbgroup_listpolicies_id" json:"dbgroup_listpolicies_id" validate:"required"`
	GroupID               uint       `gorm:"column:group_id" json:"group_id" validate:"required"`
	ValidFrom             time.Time  `gorm:"column:valid_from" json:"valid_from"`
	ValidUntil            *time.Time `gorm:"column:valid_until" json:"valid_until,omitempty"`
	IsActive              bool       `gorm:"column:is_active" json:"is_active"`
}

// TableName returns the database table name for DBPolicyGroups model.
func (DBPolicyGroups) TableName() string {
	return "dbpolicy_groups"
}
