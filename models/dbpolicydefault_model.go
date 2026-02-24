package models

// DBPolicyDefault represents a policy template with SQL queries for allow/deny rules.
type DBPolicyDefault struct {
	ID             uint   `gorm:"primaryKey;column:id" json:"id"`
	ActorType      string `gorm:"column:actortype" json:"actorType"`
	ActionId       int    `gorm:"column:action_id" json:"actionId"`
	ObjectId       int    `gorm:"column:object_id" json:"objectId"`
	SqlGet         string `gorm:"column:sql_getdata" json:"sqlGet"`
	SqlGetSpecific string `gorm:"column:sql_getdata_specific" json:"sqlGetSpecific"`
	SqlGetAllow    string `gorm:"column:sql_getdata_allow" json:"sqlGetAllow"`
	SqlGetDeny     string `gorm:"column:sql_getdata_deny" json:"sqlGetDeny"`
	SqlUpdateAllow string `gorm:"column:sql_updatedata_allow" json:"sqlUpdateAllow"`
	SqlUpdateDeny  string `gorm:"column:sql_updatedata_deny" json:"sqlUpdateDeny"`
}

// TableName returns the database table name for DBPolicyDefault model.
func (DBPolicyDefault) TableName() string {
	return "dbpolicydefault"
}
