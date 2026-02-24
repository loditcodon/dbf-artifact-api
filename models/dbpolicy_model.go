package models

// DBPolicy represents a database firewall policy record.
// Maps policy rules to specific databases, actors, and objects with enforcement status.
type DBPolicy struct {
	ID              uint   `gorm:"primaryKey;column:id" json:"id,omitempty"`
	CntMgt          uint   `gorm:"column:cnt_id" json:"cntmgt,omitempty"`
	DBPolicyDefault uint   `gorm:"column:dbpolicydefault_id" json:"dbpolicydefault,omitempty"`
	DBMgt           int    `gorm:"column:dbmgt_id" json:"dbmgt,omitempty"`
	DBActorMgt      uint   `gorm:"column:actor_id" json:"dbactormgt,omitempty"`
	DBObjectMgt     int    `gorm:"column:object_id" json:"dbobjectmgt,omitempty"`
	Status          string `gorm:"column:status" json:"status"`
	Description     string `gorm:"column:description" json:"description,omitempty"`
}

// TableName returns the database table name for DBPolicy model.
func (DBPolicy) TableName() string {
	return "dbpolicy"
}
