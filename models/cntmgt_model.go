package models

// CntMgt represents a database server connection configuration.
// Stores credentials and connection details for remote database servers.
// Agent field references Endpoint ID for VeloArtifact command execution.
// ParentConnectionID links Oracle PDB connections to their parent CDB container.
type CntMgt struct {
	ID                 uint   `gorm:"primaryKey;column:id" json:"id"`
	CntName            string `gorm:"column:cntname" json:"cntname"`                           // Connection display name
	CntType            string `gorm:"column:cnttype" json:"cnttype"`                           // Database type (mysql, postgres, mssql, oracle)
	IP                 string `gorm:"column:ip" json:"ip"`                                     // Database server IP address
	Port               int    `gorm:"column:port" json:"port"`                                 // Database server port
	ConfigFilePath     string `gorm:"column:config_file_path" json:"config_file_path"`         // Path to database config file
	Username           string `gorm:"column:username" json:"username"`                         // Database authentication username
	Password           string `gorm:"column:password" json:"password"`                         // Database authentication password (consider encryption)
	UserIP             string `gorm:"column:user_ip" json:"user_ip"`                           // Allowed user IP for connections
	Agent              int    `gorm:"column:agent" json:"agent"`                               // Foreign key to Endpoint for VeloArtifact execution
	Status             string `gorm:"column:status;default:disabled" json:"status"`            // enabled/disabled for policy operations
	Profile            string `gorm:"column:profile" json:"profile"`                           // Connection profile name
	ParentConnectionID *uint  `gorm:"column:parent_connection_id" json:"parent_connection_id"` // Parent CDB connection ID for Oracle PDBs
	ServiceName        string `gorm:"column:service_name" json:"service_name"`                 // Oracle service name for connection
	Description        string `gorm:"column:description" json:"description"`                   // Human-readable description
}

// TableName specifies the static table name for GORM.
// Required to override GORM's default pluralization behavior.
func (CntMgt) TableName() string {
	return "cntmgt"
}
