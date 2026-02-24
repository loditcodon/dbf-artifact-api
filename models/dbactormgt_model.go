package models

// DBActorMgt represents database user credentials and access control configuration.
// Maps to the dbactormgt table storing user authentication details for database connections.
type DBActorMgt struct {
	ID          uint   `gorm:"primaryKey;column:id" json:"id"`
	CntID       uint   `gorm:"column:cntid" json:"cntmgt"`
	DBUser      string `gorm:"column:dbuser" json:"dbuser" validate:"required"`
	IPAddress   string `gorm:"column:ip_address" json:"ip_address" validate:"required"`
	DBClient    string `gorm:"column:db_client" json:"db_client"`
	OSUser      string `gorm:"column:osuser" json:"osuser"`
	Password    string `gorm:"column:password" json:"dbuserpaswd"`
	Description string `gorm:"column:description" json:"description"`
	Status      string `gorm:"column:status" json:"status"`
}

//
// DBActorMgt represents the dbactormgt table.
// Stores database user credentials and connection details for access control.
// type DBActorMgt struct {
// 	ID          uint    `gorm:"primaryKey;column:id" json:"id"`
// 	CntID       *uint   `gorm:"column:cntid" json:"cntid,omitempty"`
// 	DBUser      string  `gorm:"column:dbuser" json:"dbuser" validate:"required"`
// 	IPAddress   string  `gorm:"column:ip_address" json:"ip_address" validate:"required"`
// 	DBClient    *string `gorm:"column:db_client" json:"db_client,omitempty"`
// 	OSUser      *string `gorm:"column:osuser" json:"osuser,omitempty"`
// 	Password    *string `gorm:"column:password" json:"password,omitempty"`
// 	Description *string `gorm:"column:description" json:"description,omitempty"`
// 	Status      *string `gorm:"column:status" json:"status,omitempty"`
// }

// TableName returns the database table name for DBActorMgt model.
func (DBActorMgt) TableName() string {
	return "dbactormgt"
}
