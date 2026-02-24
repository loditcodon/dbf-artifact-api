package models

// DBActor map với bảng dbactor
type DBActor struct {
	ID        uint   `gorm:"primaryKey;column:id" json:"id"`
	Actortype string `gorm:"column:actortype" json:"actortype"`
	DBType    string `gorm:"column:dbtype" json:"dbtype"`
	SQLGet    string `gorm:"column:sql_get" json:"sql_get"`
	SQLCreate string `gorm:"column:sql_create" json:"sql_create"`
	SQLUpdate string `gorm:"column:sql_update" json:"sql_update"`
	SQLDelete string `gorm:"column:sql_delete" json:"sql_delete"`
}

// TableName returns the database table name for DBActor model.
func (DBActor) TableName() string {
	return "dbactor"
}
