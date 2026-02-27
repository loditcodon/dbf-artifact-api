package oracle

import (
	"dbfartifactapi/models"
)

// OraclePrivilegeData holds all privilege data collected from Oracle database.
// Contains system privileges, object privileges, and password file privileges.
// Used to create policies after dbfAgentAPI background job completion.
type OraclePrivilegeData struct {
	SysPrivs    []OracleSysPriv    // From DBA_SYS_PRIVS / CDB_SYS_PRIVS
	TabPrivs    []OracleTabPriv    // From DBA_TAB_PRIVS
	PwFileUsers []OraclePwFileUser // From V$PWFILE_USERS
	RolePrivs   []OracleRolePriv   // From DBA_ROLE_PRIVS
	CdbSysPrivs []OracleCdbSysPriv // From CDB_SYS_PRIVS (CDB only)
}

// OracleSysPriv represents a system privilege grant from DBA_SYS_PRIVS.
// System privileges control database-wide operations (CREATE SESSION, SELECT ANY TABLE, etc.).
type OracleSysPriv struct {
	Grantee     string // User or role receiving the privilege
	Privilege   string // System privilege name (CREATE SESSION, DBA, etc.)
	AdminOption string // YES if grantee can grant to others
	Common      string // YES for CDB-level grant, NO for local
	Inherited   string // YES if inherited from CDB
}

// OracleTabPriv represents an object privilege grant from DBA_TAB_PRIVS.
// Object privileges control access to specific database objects.
type OracleTabPriv struct {
	Grantee   string // User or role receiving the privilege
	Owner     string // Schema owner of the object
	TableName string // Object name (table, view, procedure, etc.)
	Grantor   string // User who granted the privilege
	Privilege string // Object privilege (SELECT, INSERT, UPDATE, DELETE, EXECUTE, etc.)
	Grantable string // YES if grantee can grant to others
	Hierarchy string // YES for hierarchy option
	Common    string // YES for CDB-level grant
	Type      string // Object type (TABLE, VIEW, SEQUENCE, PROCEDURE, etc.)
	Inherited string // YES if inherited from CDB
}

// OraclePwFileUser represents password file user privileges from V$PWFILE_USERS.
// Password file users have special administrative privileges for database startup/shutdown.
type OraclePwFileUser struct {
	Username  string // Database username
	Sysdba    string // TRUE if has SYSDBA privilege
	Sysoper   string // TRUE if has SYSOPER privilege
	Sysasm    string // TRUE if has SYSASM privilege (ASM)
	Sysbackup string // TRUE if has SYSBACKUP privilege
	Sysdg     string // TRUE if has SYSDG privilege (Data Guard)
	Syskm     string // TRUE if has SYSKM privilege (Key Management)
}

// OracleRolePriv represents role grants from DBA_ROLE_PRIVS.
// Tracks which roles are granted to users.
type OracleRolePriv struct {
	Grantee     string // User receiving the role
	GrantedRole string // Role name being granted
	AdminOption string // YES if grantee can grant role to others
	DelegateOpt string // YES if grantee can delegate role
	DefaultRole string // YES if role is enabled by default
	Common      string // YES for CDB-level grant
	Inherited   string // YES if inherited from CDB
}

// OracleCdbSysPriv represents CDB-wide system privileges from CDB_SYS_PRIVS.
// Only available when connected to CDB (Container Database).
// Includes CON_ID to identify which container the privilege applies to.
type OracleCdbSysPriv struct {
	Grantee     string // User or role receiving the privilege
	Privilege   string // System privilege name
	AdminOption string // YES if grantee can grant to others
	Common      string // YES for common privilege
	Inherited   string // YES if inherited
	ConID       int    // Container ID (1=CDB$ROOT, 3+=PDB)
}

// OraclePrivilegeSessionJobContext contains context data for Oracle privilege session job completion.
// Passed to completion handler when dbfAgentAPI background job finishes.
type OraclePrivilegeSessionJobContext struct {
	CntMgtID      uint                 `json:"cnt_mgt_id"`
	CMT           *models.CntMgt       `json:"cmt"`
	EndpointID    uint                 `json:"endpoint_id"`
	ConnType      OracleConnectionType `json:"conn_type"`
	DbActorMgts   []models.DBActorMgt  `json:"db_actor_mgts"`
	DbMgts        []models.DBMgt       `json:"db_mgts"`
	SessionID     string               `json:"session_id"`
	PrivilegeFile string               `json:"privilege_file"`
}
