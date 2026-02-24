package models

import "time"

// DBActorGroups represents the dbactor_groups table.
// Links database actors to groups with validity periods for access control.
type DBActorGroups struct {
	ID         uint       `gorm:"primaryKey;column:id" json:"id"`
	ActorID    uint       `gorm:"column:actor_id" json:"actor_id" validate:"required"`
	GroupID    uint       `gorm:"column:group_id" json:"group_id" validate:"required"`
	ValidFrom  time.Time  `gorm:"column:valid_from" json:"valid_from"`
	ValidUntil *time.Time `gorm:"column:valid_until" json:"valid_until,omitempty"`
	IsActive   bool       `gorm:"column:is_active" json:"is_active"`
}

// TableName returns the database table name for DBActorGroups model.
func (DBActorGroups) TableName() string {
	return "dbactor_groups"
}
