package oracle

import (
	"fmt"
	"strings"

	"dbfartifactapi/pkg/logger"
)

// GetOraclePrivilegeColumnNames returns column names for Oracle privilege tables.
// Column order must match SELECT query order to prevent data corruption during parsing.
func GetOraclePrivilegeColumnNames(tableName string) ([]string, error) {
	columnMap := map[string][]string{
		"dba_sys_privs": {
			"GRANTEE", "PRIVILEGE", "ADMIN_OPTION", "COMMON", "INHERITED",
		},
		"dba_tab_privs": {
			"GRANTEE", "OWNER", "TABLE_NAME", "GRANTOR", "PRIVILEGE",
			"GRANTABLE", "HIERARCHY", "COMMON", "TYPE", "INHERITED",
		},
		"dba_role_privs": {
			"GRANTEE", "GRANTED_ROLE", "ADMIN_OPTION", "DELEGATE_OPTION",
			"DEFAULT_ROLE", "COMMON", "INHERITED",
		},
		"v$pwfile_users": {
			"USERNAME", "SYSDBA", "SYSOPER", "SYSASM", "SYSBACKUP", "SYSDG", "SYSKM",
		},
		"cdb_sys_privs": {
			"GRANTEE", "PRIVILEGE", "ADMIN_OPTION", "COMMON", "INHERITED", "CON_ID",
		},
	}

	columns, ok := columnMap[tableName]
	if !ok {
		return nil, fmt.Errorf("unknown Oracle privilege table: %s", tableName)
	}
	return columns, nil
}

// EscapeOracleSQL escapes single quotes in Oracle SQL strings.
// Oracle uses doubled single quotes for escaping: O'Brien -> O''Brien
func EscapeOracleSQL(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// BuildOraclePrivilegeDataQueries builds queries to fetch privilege data from Oracle system tables.
// Queries adapt based on connection type: CDB queries include CDB_SYS_PRIVS, PDB queries use DBA views.
// Returns map of query keys to SQL statements for dbfAgentAPI execution.
func BuildOraclePrivilegeDataQueries(
	actors []ActorInfo,
	connType OracleConnectionType,
) (map[string][]string, error) {
	if len(actors) == 0 {
		return nil, fmt.Errorf("no actors provided for Oracle privilege query building")
	}

	// Build actor filter for WHERE clauses
	actorNames := make([]string, 0, len(actors))
	for _, actor := range actors {
		actorNames = append(actorNames, fmt.Sprintf("'%s'", EscapeOracleSQL(actor.DBUser)))
	}
	actorFilter := strings.Join(actorNames, ", ")

	queries := make(map[string][]string)

	// DBA_SYS_PRIVS - System privileges granted to users/roles
	queries["dba_sys_privs"] = []string{
		fmt.Sprintf(`SELECT GRANTEE, PRIVILEGE, ADMIN_OPTION,
			NVL(COMMON, 'NO') AS COMMON,
			NVL(INHERITED, 'NO') AS INHERITED
			FROM DBA_SYS_PRIVS
			WHERE GRANTEE IN (%s)`, actorFilter),
	}

	// DBA_TAB_PRIVS - Object privileges on tables, views, procedures, etc.
	queries["dba_tab_privs"] = []string{
		fmt.Sprintf(`SELECT GRANTEE, OWNER, TABLE_NAME, GRANTOR, PRIVILEGE,
			GRANTABLE, NVL(HIERARCHY, 'NO') AS HIERARCHY,
			NVL(COMMON, 'NO') AS COMMON,
			NVL(TYPE, 'TABLE') AS TYPE,
			NVL(INHERITED, 'NO') AS INHERITED
			FROM DBA_TAB_PRIVS
			WHERE GRANTEE IN (%s)`, actorFilter),
	}

	// DBA_ROLE_PRIVS - Roles granted to users
	queries["dba_role_privs"] = []string{
		fmt.Sprintf(`SELECT GRANTEE, GRANTED_ROLE, ADMIN_OPTION,
			NVL(DELEGATE_OPTION, 'NO') AS DELEGATE_OPTION,
			DEFAULT_ROLE,
			NVL(COMMON, 'NO') AS COMMON,
			NVL(INHERITED, 'NO') AS INHERITED
			FROM DBA_ROLE_PRIVS
			WHERE GRANTEE IN (%s)`, actorFilter),
	}

	// V$PWFILE_USERS - Password file administrative privileges
	// These are special privileges for database startup/shutdown
	queries["v$pwfile_users"] = []string{
		fmt.Sprintf(`SELECT USERNAME,
			NVL(SYSDBA, 'FALSE') AS SYSDBA,
			NVL(SYSOPER, 'FALSE') AS SYSOPER,
			NVL(SYSASM, 'FALSE') AS SYSASM,
			NVL(SYSBACKUP, 'FALSE') AS SYSBACKUP,
			NVL(SYSDG, 'FALSE') AS SYSDG,
			NVL(SYSKM, 'FALSE') AS SYSKM
			FROM V$PWFILE_USERS
			WHERE USERNAME IN (%s)`, actorFilter),
	}

	// CDB_SYS_PRIVS - CDB-wide privileges (only for CDB connections)
	// Shows privileges across all containers in the CDB
	if connType == OracleConnectionCDB {
		queries["cdb_sys_privs"] = []string{
			fmt.Sprintf(`SELECT GRANTEE, PRIVILEGE, ADMIN_OPTION,
				NVL(COMMON, 'NO') AS COMMON,
				NVL(INHERITED, 'NO') AS INHERITED,
				CON_ID
				FROM CDB_SYS_PRIVS
				WHERE GRANTEE IN (%s)`, actorFilter),
		}
	}

	logger.Infof("Built %d Oracle privilege queries for %d actors (conn_type=%s)",
		len(queries), len(actors), connType)

	return queries, nil
}

// ActorInfo provides actor data needed for building Oracle privilege queries.
// Decouples from models.DBActorMgt to avoid circular imports.
type ActorInfo struct {
	DBUser string
}

// DatabaseInfo provides database data needed for building Oracle object queries.
// Decouples from models.DBMgt to avoid circular imports.
type DatabaseInfo struct {
	DbName string
}

// BuildOracleObjectQueries builds queries to fetch database objects for policy matching.
// Objects are retrieved from ALL_* views based on the object types defined in DBPolicyDefault.
func BuildOracleObjectQueries(
	databases []DatabaseInfo,
	connType OracleConnectionType,
) (map[string][]string, error) {
	if len(databases) == 0 {
		return nil, fmt.Errorf("no databases provided for Oracle object query building")
	}

	queries := make(map[string][]string)

	// Build schema filter from databases
	schemaNames := make([]string, 0, len(databases))
	for _, db := range databases {
		schemaNames = append(schemaNames, fmt.Sprintf("'%s'", EscapeOracleSQL(db.DbName)))
	}
	schemaFilter := strings.Join(schemaNames, ", ")

	// ALL_TABLES - User-accessible tables
	queries["all_tables"] = []string{
		fmt.Sprintf(`SELECT OWNER, TABLE_NAME, TABLESPACE_NAME, STATUS
			FROM ALL_TABLES
			WHERE OWNER IN (%s)`, schemaFilter),
	}

	// ALL_VIEWS - User-accessible views
	queries["all_views"] = []string{
		fmt.Sprintf(`SELECT OWNER, VIEW_NAME, TEXT_LENGTH
			FROM ALL_VIEWS
			WHERE OWNER IN (%s)`, schemaFilter),
	}

	// ALL_PROCEDURES - Procedures, functions, packages
	queries["all_procedures"] = []string{
		fmt.Sprintf(`SELECT OWNER, OBJECT_NAME, PROCEDURE_NAME, OBJECT_TYPE
			FROM ALL_PROCEDURES
			WHERE OWNER IN (%s) AND OBJECT_TYPE IN ('PROCEDURE', 'FUNCTION', 'PACKAGE')`, schemaFilter),
	}

	// ALL_SEQUENCES - Sequences
	queries["all_sequences"] = []string{
		fmt.Sprintf(`SELECT SEQUENCE_OWNER, SEQUENCE_NAME
			FROM ALL_SEQUENCES
			WHERE SEQUENCE_OWNER IN (%s)`, schemaFilter),
	}

	// ALL_INDEXES - Indexes
	queries["all_indexes"] = []string{
		fmt.Sprintf(`SELECT OWNER, INDEX_NAME, TABLE_OWNER, TABLE_NAME, INDEX_TYPE
			FROM ALL_INDEXES
			WHERE OWNER IN (%s)`, schemaFilter),
	}

	// ALL_TRIGGERS - Triggers
	queries["all_triggers"] = []string{
		fmt.Sprintf(`SELECT OWNER, TRIGGER_NAME, TRIGGER_TYPE, TABLE_OWNER, TABLE_NAME
			FROM ALL_TRIGGERS
			WHERE OWNER IN (%s)`, schemaFilter),
	}

	// DBA_USERS - Database users (for ObjectId 12 equivalent)
	queries["dba_users"] = []string{
		`SELECT USERNAME, USER_ID, ACCOUNT_STATUS, DEFAULT_TABLESPACE, PROFILE
			FROM DBA_USERS
			WHERE ORACLE_MAINTAINED = 'N'`,
	}

	// DBA_ROLES - Database roles
	queries["dba_roles"] = []string{
		`SELECT ROLE, ROLE_ID, AUTHENTICATION_TYPE
			FROM DBA_ROLES
			WHERE ORACLE_MAINTAINED = 'N'`,
	}

	// DBA_PDBS - Pluggable databases (CDB only)
	if connType == OracleConnectionCDB {
		queries["dba_pdbs"] = []string{
			`SELECT PDB_ID, PDB_NAME, STATUS, CON_ID
				FROM DBA_PDBS
				WHERE STATUS = 'NORMAL'`,
		}
	}

	// V$DATABASE - Database information
	queries["v$database"] = []string{
		`SELECT DBID, NAME, CREATED, OPEN_MODE, DATABASE_ROLE, CDB
			FROM V$DATABASE`,
	}

	// V$INSTANCE - Instance information
	queries["v$instance"] = []string{
		`SELECT INSTANCE_NUMBER, INSTANCE_NAME, HOST_NAME, VERSION, STATUS
			FROM V$INSTANCE`,
	}

	logger.Infof("Built %d Oracle object queries for %d schemas (conn_type=%s)",
		len(queries), len(databases), connType)

	return queries, nil
}
