package models

// DBObjectMgt represents a specific database object instance with SQL parameters.
type DBObjectMgt struct {
	ID uint `json:"id,omitempty"`
	// CntMgt     uint   `json:"cntmgt" validate:"required"`
	DBMgt      uint   `json:"dbmgt" gorm:"column:dbid"`
	ObjectName string `json:"objectName,omitempty" gorm:"column:objectname"`
	ObjectId   uint   `json:"objectTypeId,omitempty" gorm:"column:dbobject_id"`
	// Host        string `json:"host,omitempty"`
	Description string `json:"description,omitempty" gorm:"column:description"`
	Status      string `gorm:"column:status" json:"status"`
	SqlParam    string `json:"sqlParam,omitempty" gorm:"column:sql_param"`
}

// TableName returns the database table name for DBObjectMgt model.
func (DBObjectMgt) TableName() string {
	return "dbobjectmgt"
}
