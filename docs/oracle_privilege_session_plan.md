# Oracle Privilege Session - Implementation Documentation

## Overview

Oracle privilege data collection support for DBF Artifact API. Uses the **same go-mysql-server approach as MySQL** - creates Oracle privilege tables in go-mysql-server and executes DBPolicyDefault queries against them.

### Key Architecture Decisions

1. **Same go-mysql-server approach**: Oracle privilege tables are created in go-mysql-server (DBA_SYS_PRIVS, DBA_TAB_PRIVS, etc.)
2. **Two-tier architecture**: CDB (Container Database) and PDB (Pluggable Database)
3. **Three-pass execution**: Same as MySQL - Pass 1 (super), Pass 2 (action-wide), Pass 3 (object-specific)
4. **Different group assignment**: Oracle superPrivGroupID = 1000 (MySQL uses group 1)

## Key Differences from MySQL

| Aspect | MySQL | Oracle |
|--------|-------|--------|
| Connection Types | Single database | CDB (ParentConnectionID=null) / PDB (ParentConnectionID≠null) |
| Privilege Tables | mysql.user, mysql.db, etc. | DBA_SYS_PRIVS, DBA_TAB_PRIVS, V_PWFILE_USERS, DBA_ROLE_PRIVS, CDB_SYS_PRIVS |
| In-memory Server | go-mysql-server with "mysql" database | go-mysql-server with "oracle" database |
| Object Type Wildcard | N/A | `${dbobject.objecttype}` = '*' (CDB) or 'PDB' (PDB) |
| Super Privilege Group | Group ID = 1 | Group ID = 1000 |
| Database Type ID | 1 | 3 |
| Table Name Compatibility | Direct use | V$* views → V_* ($ not allowed in go-mysql-server) |

## Connection Type Detection

```go
// File: services/oracle_connection_helper.go

type OracleConnectionType int

const (
    OracleConnectionCDB OracleConnectionType = iota  // ParentConnectionID = null
    OracleConnectionPDB                               // ParentConnectionID != null
)

// GetOracleConnectionType determines if connection is CDB or PDB
func GetOracleConnectionType(cmt *models.CntMgt) OracleConnectionType

// GetObjectTypeWildcard returns wildcard value for object_id=-1
// CDB → "*", PDB → "PDB"
func GetObjectTypeWildcard(connType OracleConnectionType) string
```

## Oracle Privilege Tables (go-mysql-server)

### Tables Created in privilege_session.go

```go
// File: services/privilege_session.go - createOraclePrivilegeTables()

// DBA_SYS_PRIVS - System privileges
GRANTEE, PRIVILEGE, ADMIN_OPTION, COMMON, INHERITED

// DBA_TAB_PRIVS - Object privileges
GRANTEE, OWNER, TABLE_NAME, GRANTOR, PRIVILEGE, GRANTABLE, HIERARCHY, COMMON, TYPE, INHERITED

// DBA_ROLE_PRIVS - Role grants
GRANTEE, GRANTED_ROLE, ADMIN_OPTION, DELEGATE_OPTION, DEFAULT_ROLE, COMMON, INHERITED

// V_PWFILE_USERS - Password file users (V$PWFILE_USERS renamed)
USERNAME, SYSDBA, SYSOPER, SYSASM, SYSBACKUP, SYSDG, SYSKM

// CDB_SYS_PRIVS - CDB-wide system privileges (CDB only)
GRANTEE, PRIVILEGE, ADMIN_OPTION, COMMON, INHERITED, CON_ID

// DBA_TS_QUOTAS - Tablespace quotas for users
TABLESPACE_NAME, USERNAME, BYTES, MAX_BYTES, BLOCKS, MAX_BLOCKS, DROPPED
```

### V$ View Replacement

Oracle dynamic performance views with `$` in name are automatically replaced by `replaceOracleDollarViews()`:

```go
// Common replacements:
V$PWFILE_USERS → V_PWFILE_USERS
V$SESSION      → V_SESSION
V$PARAMETER    → V_PARAMETER
V$DATABASE     → V_DATABASE
V$INSTANCE     → V_INSTANCE
GV$SESSION     → GV_SESSION
// ... and more
```

## Implementation Files

### Core Files

| File | Purpose |
|------|---------|
| `services/oracle_connection_helper.go` | CDB/PDB detection, GetObjectTypeWildcard() |
| `services/oracle_privilege_session.go` | Type definitions: OraclePrivilegeData, OracleSysPriv, etc. |
| `services/oracle_privilege_queries.go` | buildOraclePrivilegeDataQueries(), writeOraclePrivilegeQueryFile() |
| `services/oracle_privilege_session_handler.go` | Main handler: createOraclePoliciesWithPrivilegeData(), 3-pass execution |
| `services/privilege_session.go` | NewOraclePrivilegeSession(), LoadOraclePrivilegeDataFromResults(), ExecuteOracleTemplate() |
| `services/dbpolicy_service.go` | GetByCntMgtWithOraclePrivilegeSession() |

## Three-Pass Execution Strategy

### Pass 1: Super Privileges
- Creates policies with `actor_id=-1, object_id=-1, dbmgt_id=-1`
- Actors marked in `superPrivActors` - skip Pass 2 and 3
- Assigned to **group 1000** (Oracle-specific)

### Pass 2: Action-Wide Privileges
- Uses `DBGroupListPolicies` with `database_type_id=3`
- Creates policies with `object_id=-1, dbmgt_id=-1`
- Actions marked in `grantedActions` - skip Pass 3 for same action

### Pass 3: Object-Specific Privileges
- **General queries** (`objectId = -1`): `processOracleSQLTemplatesForSession()`
- **Specific queries** (`objectId != -1`): `processOracleSpecificSQLTemplatesForSession()`
- Uses `oracleQueryBuildCache` for efficient DBObjectMgt lookups

## Query Building Functions

### processOracleSQLTemplatesForSession (objectId = -1)

```go
// File: services/oracle_privilege_session_handler.go

// Builds general queries from SqlGet template
// Returns objectId = -1 (wildcard)
func processOracleSQLTemplatesForSession(
    tx *gorm.DB,
    policies []models.DBPolicyDefault,
    dbMgts []models.DBMgt,
    actors []models.DBActorMgt,
    cmt *models.CntMgt,
    service *dbPolicyService,
    connType OracleConnectionType,
) (map[string]policyInput, error)
```

Variable substitutions:
- `${dbactormgt.dbuser}` → actor.DBUser
- `${dbactormgt.ip_address}` → actor.IPAddress
- `${dbobject.objecttype}` → "*" (CDB) or "PDB" (PDB)
- `${dbmgt.dbname}` → dbMgt.DbName
- `V$*` views → `V_*` (automatic replacement)

### processOracleSpecificSQLTemplatesForSession (objectId != -1)

```go
// File: services/oracle_privilege_session_handler.go

// Builds specific queries from SqlGetSpecific template
// Uses oracleQueryBuildCache for DBObjectMgt lookups
// Returns objectId = object.ID (actual ID)
func processOracleSpecificSQLTemplatesForSession(
    tx *gorm.DB,
    policies []models.DBPolicyDefault,
    dbMgts []models.DBMgt,
    actors []models.DBActorMgt,
    cmt *models.CntMgt,
    service *dbPolicyService,
    connType OracleConnectionType,
    cache *oracleQueryBuildCache,
) (map[string]policyInput, error)
```

Additional substitution:
- `${dbobjectmgt.objectname}` → object.ObjectName

### oracleQueryBuildCache

```go
// File: services/oracle_privilege_session_handler.go

type oracleQueryBuildCache struct {
    objectsByKey     map[string][]models.DBObjectMgt // Key: "objectId:dbMgtId"
    allActorsByCntID map[uint][]models.DBActorMgt    // Key: cnt_id
}

// Caches DBObjectMgt records to avoid N+1 queries
func newOracleQueryBuildCache(tx *gorm.DB, cntMgtID uint, dbMgts []models.DBMgt, policies []models.DBPolicyDefault) *oracleQueryBuildCache
```

## Group Assignment

### assignOracleActorsToGroups

```go
// File: services/oracle_privilege_session_handler.go

// Oracle uses superPrivGroupID = 1000 (different from MySQL's group 1)
const superPrivGroupID = uint(1000)

func assignOracleActorsToGroups(
    tx *gorm.DB,
    cntMgtID uint,
    cmt *models.CntMgt,
    allowedResults *allowedPolicyResults,
    superPrivActors *superPrivilegeActors,
) error
```

Logic:
1. Super privilege actors (Pass 1) → direct assign to **group 1000**
2. Other actors → match with `dbgroup_listpolicies` (database_type_id=3) and `dbpolicy_groups`
3. Find groups where actor satisfies ALL required listpolicies
4. Create `DBActorGroups` records

## Data Flow

```
1. GetByCntMgt() routes to GetByCntMgtWithOraclePrivilegeSession()
   ↓
2. Build Oracle privilege queries (buildOraclePrivilegeDataQueries)
   ↓
3. Write queries to JSON file (writeOraclePrivilegeQueryFile)
   ↓
4. Execute via dbfAgentAPI background job
   ↓
5. Job completion → CreateOraclePrivilegeSessionCompletionHandler()
   ↓
6. Parse privilege data (parseOraclePrivilegeResults)
   ↓
7. Create in-memory Oracle session (NewOraclePrivilegeSession)
   ↓
8. Load data into go-mysql-server (LoadOraclePrivilegeDataFromResults)
   ↓
9. Three-pass execution:
   - Pass 1: executeOracleSuperPrivilegeQueries()
   - Pass 2: executeOracleActionWideQueries()
   - Pass 3: executeOracleObjectSpecificQueries()
   ↓
10. Create DBPolicy records (createOraclePolicyRecord)
    ↓
11. Assign actors to groups (assignOracleActorsToGroups)
    ↓
12. Export DBF policy rules (utils.ExportDBFPolicy)
```

## Type Definitions

### OraclePrivilegeData

```go
// File: services/oracle_privilege_session.go

type OraclePrivilegeData struct {
    SysPrivs    []OracleSysPriv    // From DBA_SYS_PRIVS
    TabPrivs    []OracleTabPriv    // From DBA_TAB_PRIVS
    PwFileUsers []OraclePwFileUser // From V$PWFILE_USERS
    RolePrivs   []OracleRolePriv   // From DBA_ROLE_PRIVS
    CdbSysPrivs []OracleCdbSysPriv // From CDB_SYS_PRIVS (CDB only)
}
```

### OraclePrivilegeSessionJobContext

```go
// File: services/oracle_privilege_session.go

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
```

## Policy Template Classification

```go
// File: services/oracle_privilege_session_handler.go

func classifyOraclePolicyTemplates(allPolicies map[uint]models.DBPolicyDefault) *policyClassification

// Classification rules:
// - Super privileges: policy ID = 1001 (Oracle-specific, MySQL uses 1)
// - Action-wide: policy IDs in DBGroupListPolicies (database_type_id=3)
// - Object-specific: all other Oracle policies

// Oracle policy detection:
func isOraclePolicyTemplate(policy models.DBPolicyDefault) bool
// Checks if SqlGet contains: DBA_SYS_PRIVS, DBA_TAB_PRIVS, DBA_ROLE_PRIVS, V$PWFILE_USERS, V_PWFILE_USERS, CDB_SYS_PRIVS
```

## Key Constants

```go
// Oracle database type ID
const oracleDatabaseTypeID = uint(3)

// Super privilege group ID for Oracle
const superPrivGroupID = uint(1000)  // MySQL uses 1

// Connection types
OracleConnectionCDB = 0  // ParentConnectionID = null
OracleConnectionPDB = 1  // ParentConnectionID != null
```

## Error Handling

- All database operations wrapped in transactions
- Rollback on failure, commit on success
- Individual query failures logged but don't stop processing
- Idempotency check prevents duplicate job processing (`processedOraclePrivilegeJobs`)
- Panic recovery in goroutines to prevent crash propagation

## Concurrency and Performance

### Parallel Query Execution

Oracle uses the same concurrency model as MySQL for query execution:

```go
// Configurable via config.GetPrivilegeQueryConcurrency()
// Uses semaphore pattern to limit concurrent queries
// Default auto-detects based on CPU cores (min 4, max 50)

// Pattern used in all three passes:
maxConcurrent := config.GetPrivilegeQueryConcurrency()
semaphore := make(chan struct{}, maxConcurrent)
resultsChan := make(chan queryResult, len(queries))

for uniqueKey, policyData := range queries {
    go func(key string, input policyInput) {
        defer func() {
            if r := recover(); r != nil {
                // Panic recovery
            }
        }()

        semaphore <- struct{}{}
        defer func() { <-semaphore }()

        // Execute query
        results, err := session.ExecuteOracleTemplate(input.finalSQL, nil)
        resultsChan <- queryResult{...}
    }(uniqueKey, policyData)
}

// Collect results, then process serially for database operations
```

### Query Build Cache

Pre-loads all actors and objects data in batches to eliminate N+1 queries during query building:

```go
type oracleQueryBuildCache struct {
    objectsByKey     map[string][]models.DBObjectMgt // Key: "objectId:dbMgtId"
    allActorsByCntID map[uint][]models.DBActorMgt    // Key: cnt_id
}
```

## Logging

Query logging enabled via `config.Cfg.EnableMySQLPrivilegeQueryLogging`:
- Log file: `oracle_privilege_queries_{sessionID}_{timestamp}.log`
- Location: `config.Cfg.DBFWebTempDir`
- Format: `[timestamp] [PASS-N-TYPE] [uniqueKey]\n{query}\n`
