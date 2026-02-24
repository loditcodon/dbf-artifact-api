package models

// DBObject represents a database object type definition (table, view, procedure, etc.).
type DBObject struct {
	ID           uint   `gorm:"primaryKey;column:id" json:"id"`
	ObjectType   string `gorm:"column:objecttype" json:"objecttype"`
	DBType       string `gorm:"column:dbtype" json:"dbtype"`
	SQLGet       string `gorm:"column:sql_get" json:"sql_get"`
	SQLCreate    string `gorm:"column:sql_create" json:"sql_create"`
	SQLUpdate    string `gorm:"column:sql_update" json:"sql_update"`
	SQLDelete    string `gorm:"column:sql_delete" json:"sql_delete"`
	Description  string `gorm:"column:description" json:"description"`
	SqlInputType uint   `gorm:"column:sql_input_type" json:"sql_input_type"`
}

// TableName returns the database table name for DBObject model.
func (DBObject) TableName() string {
	return "dbobject"
}
