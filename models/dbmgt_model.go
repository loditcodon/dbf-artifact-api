package models

// DBMgt represents a managed database instance configuration.
// Each record tracks a specific database on a remote server connection.
// Used by security policy engine to enforce access control rules per database.
type DBMgt struct {
	ID     uint   `gorm:"primaryKey;column:id" json:"id"`
	CntID  uint   `gorm:"column:cnt_id" json:"cntmgt" validate:"required"`     // Foreign key to CntMgt (connection)
	DbName string `gorm:"column:dbname;unique" json:"dbname" validate:"required"` // Unique database name per connection
	DbType string `gorm:"column:dbtype" json:"dbtype" validate:"required"`    // Database type (mysql, postgres, etc)
	Status string `gorm:"column:status" json:"status"`                        // enabled/disabled for policy enforcement
}

// TableName specifies the static table name for GORM.
// Required to override GORM's default pluralization behavior.
func (DBMgt) TableName() string {
	return "dbmgt"
}
