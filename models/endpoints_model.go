package models

// Endpoint map với bảng endpoints
type Endpoint struct {
	ID       uint   `gorm:"primaryKey;column:id" json:"id"`
	ClientID string `gorm:"column:client_id" json:"client_id"`
	OsType   string `gorm:"column:os_type" json:"os_type"`
}

// TableName chỉ định tên bảng tĩnh
func (Endpoint) TableName() string {
	return "endpoints"
}
