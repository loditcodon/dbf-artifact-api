package models

// DBType represents a database type configuration with CRUD SQL templates.
type DBType struct {
	ID        uint   `gorm:"primaryKey;column:id" json:"id"`
	Name      string `gorm:"column:name" json:"dbname"`
	SqlGet    string `gorm:"column:sql_get" json:"sql_get"`
	SqlUpdate string `gorm:"column:sql_update" json:"sql_update"`
	SqlCreate string `gorm:"column:sql_create" json:"sql_create"`
	SqlDelete string `gorm:"column:sql_delete" json:"sql_delete"`
}

// TableName returns the database table name for DBType model.
func (DBType) TableName() string {
	return "dbtype"
}
