package dto

// DBActorMgtCreate represents the request body for creating a database actor.
// For Oracle: ip_address is optional (Oracle uses username only, no host concept).
// For MySQL: ip_address is required (MySQL uses user@host format).
type DBActorMgtCreate struct {
	CntMgt      uint   `json:"cntmgt" validate:"required"`
	DBUser      string `json:"dbuser" validate:"required"`
	Password    string `json:"dbuserpaswd"`
	IPAddress   string `json:"ip_address"`
	DBClient    string `json:"db_client"`
	OSUser      string `json:"osuser"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// DBActorMgtUpdate represents the updateable fields for database actor management records.
type DBActorMgtUpdate struct {
	DBClient    string `json:"db_client,omitempty"`
	DBUser      string `json:"dbuser,omitempty"`
	Password    string `json:"dbuserpaswd,omitempty"`
	Description string `json:"description,omitempty"`
	IPAddress   string `json:"ip_address,omitempty"`
	OSUser      string `json:"osuser,omitempty"`
	Status      string `json:"status,omitempty"`
}
