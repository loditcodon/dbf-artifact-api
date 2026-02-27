package services

import (
	"dbfartifactapi/models"
	"dbfartifactapi/services/privilege/oracle"
)

// OracleConnectionType is an alias for oracle.OracleConnectionType.
// Kept for backward compatibility with callers in services package.
type OracleConnectionType = oracle.OracleConnectionType

// Re-export Oracle connection type constants for backward compatibility.
const (
	OracleConnectionCDB = oracle.OracleConnectionCDB
	OracleConnectionPDB = oracle.OracleConnectionPDB
)

// GetOracleConnectionType delegates to oracle.GetOracleConnectionType.
func GetOracleConnectionType(cmt *models.CntMgt) OracleConnectionType {
	return oracle.GetOracleConnectionType(cmt)
}

// IsOracleCDB delegates to oracle.IsOracleCDB.
func IsOracleCDB(cmt *models.CntMgt) bool {
	return oracle.IsOracleCDB(cmt)
}

// IsOraclePDB delegates to oracle.IsOraclePDB.
func IsOraclePDB(cmt *models.CntMgt) bool {
	return oracle.IsOraclePDB(cmt)
}

// GetObjectTypeWildcard delegates to oracle.GetObjectTypeWildcard.
func GetObjectTypeWildcard(connType OracleConnectionType) string {
	return oracle.GetObjectTypeWildcard(connType)
}

// GetOraclePrivilegeScope delegates to oracle.GetOraclePrivilegeScope.
func GetOraclePrivilegeScope(connType OracleConnectionType) string {
	return oracle.GetOraclePrivilegeScope(connType)
}
