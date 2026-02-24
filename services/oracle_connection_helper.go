package services

import (
	"dbfartifactapi/models"
)

// OracleConnectionType represents CDB (Container Database) or PDB (Pluggable Database) connection.
// CDB connections manage the entire container, while PDB connections operate within a specific pluggable database.
type OracleConnectionType int

const (
	// OracleConnectionCDB represents a Container Database connection (parent container).
	// Identified by: CntType='oracle', ServiceName!=null, ParentConnectionID=null
	OracleConnectionCDB OracleConnectionType = iota

	// OracleConnectionPDB represents a Pluggable Database connection (child container).
	// Identified by: CntType='oracle', ServiceName!=null, ParentConnectionID!=null
	OracleConnectionPDB
)

// String returns human-readable connection type name for logging.
func (t OracleConnectionType) String() string {
	switch t {
	case OracleConnectionCDB:
		return "CDB"
	case OracleConnectionPDB:
		return "PDB"
	default:
		return "UNKNOWN"
	}
}

// GetOracleConnectionType determines if Oracle connection is CDB or PDB.
// CDB: Container Database with no parent connection.
// PDB: Pluggable Database with parent CDB connection reference.
// Returns OracleConnectionCDB if ParentConnectionID is nil or zero.
func GetOracleConnectionType(cmt *models.CntMgt) OracleConnectionType {
	if cmt.ParentConnectionID == nil || *cmt.ParentConnectionID == 0 {
		return OracleConnectionCDB
	}
	return OracleConnectionPDB
}

// IsOracleCDB checks if connection is a Container Database.
// CDB connections have no parent and manage all PDBs within the container.
func IsOracleCDB(cmt *models.CntMgt) bool {
	return GetOracleConnectionType(cmt) == OracleConnectionCDB
}

// IsOraclePDB checks if connection is a Pluggable Database.
// PDB connections have a parent CDB reference via ParentConnectionID.
func IsOraclePDB(cmt *models.CntMgt) bool {
	return GetOracleConnectionType(cmt) == OracleConnectionPDB
}

// GetObjectTypeWildcard returns the appropriate wildcard value for Oracle object_id=-1 substitution.
// For CDB: returns "*" (all objects across all PDBs).
// For PDB: returns "PDB" (objects within the specific PDB scope).
// Used when processing dbpolicydefault sql_getdata with ${dbobject.objecttype} variable.
func GetObjectTypeWildcard(connType OracleConnectionType) string {
	if connType == OracleConnectionCDB {
		return "*"
	}
	return "PDB"
}

// GetOraclePrivilegeScope returns the scope name for logging and policy descriptions.
// CDB scope includes all PDBs, PDB scope is limited to the specific pluggable database.
func GetOraclePrivilegeScope(connType OracleConnectionType) string {
	if connType == OracleConnectionCDB {
		return "CDB-wide"
	}
	return "PDB-local"
}
