# MySQL Privilege Session Logic Documentation

## Overview

The MySQL Privilege Session system is designed to efficiently discover and create database policies by analyzing user privileges stored in MySQL system tables. This document explains the complete flow from data collection to policy creation.

## Architecture Components

### 1. Core Files

- **[dbpolicy_service.go](../services/dbpolicy_service.go)**: Entry point for policy generation via `GetByCntMgt()`
- **[privilege_session.go](../services/privilege_session.go)**: In-memory MySQL server using `go-mysql-server` library
- **[privilege_session_handler.go](../services/privilege_session_handler.go)**: Job completion handler and policy creation logic

### 2. Data Flow

```
GetByCntMgt()
    ↓
Build privilege data queries (buildPrivilegeDataQueries)
    ↓
Write queries to JSON file (writePrivilegeQueryFile)
    ↓
Execute via dbfAgentAPI background job
    ↓
Job Monitor polls for completion
    ↓
CreatePrivilegeSessionCompletionHandler()
    ↓
Create in-memory MySQL server (NewPrivilegeSession)
    ↓
Load privilege data into session (loadPrivilegeDataFromResults)
    ↓
Three-pass policy execution:
    - PASS-1: Super privileges (ID=1)
    - PASS-2: Action-wide privileges
    - PASS-3: Object-specific privileges
    ↓
Assign actors to groups (assignActorsToGroups)
    ↓
Export DBF policy rules (exportDBFPolicy)
```

## Detailed Process

### Step 1: GetByCntMgt Entry Point

**File**: [dbpolicy_service.go:96-98](../services/dbpolicy_service.go#L96-L98)

```go
func (s *dbPolicyService) GetByCntMgt(ctx context.Context, id uint) (string, error) {
    return s.GetByCntMgtWithPrivilegeSession(ctx, id)
}
```

### Step 2: Build Privilege Data Queries

**File**: [dbpolicy_service.go:664-744](../services/dbpolicy_service.go#L664-L744)

The system builds queries to fetch privilege data from these MySQL system tables:
- `mysql.user` - Global user privileges
- `mysql.db` - Database-level privileges
- `mysql.tables_priv` - Table-level privileges
- `mysql.procs_priv` - Stored procedure privileges
- `mysql.role_edges` - Role assignments
- `mysql.global_grants` - Dynamic privileges (MySQL 8.0+)
- `mysql.proxies_priv` - Proxy user privileges
- `information_schema.USER_PRIVILEGES` - User privilege information
- `information_schema.SCHEMA_PRIVILEGES` - Schema privilege information
- `information_schema.TABLE_PRIVILEGES` - Table privilege information

Queries are filtered by:
- **Actors**: `(User, Host) IN (('user1', 'host1'), ('user2', 'host2'))`
- **Databases**: `Db IN ('db1', 'db2', 'db3')`

### Step 3: Execute via dbfAgentAPI Background Job

**File**: [dbpolicy_service.go:551-624](../services/dbpolicy_service.go#L551-L624)

1. Write queries to JSON file in `DBFWEB_TEMP_DIR`
2. Create query parameters with `--background` option
3. Execute via `executeSqlAgentAPI()` with action `download`
4. Register job with `JobMonitorService` for polling

### Step 4: Job Completion Handler

**File**: [privilege_session_handler.go:528-547](../services/privilege_session_handler.go#L528-L547)

When job completes:
1. Extract context data from job info
2. Parse privilege data from result file
3. Call `createPoliciesWithPrivilegeData()`

### Step 5: Create In-Memory MySQL Server

**File**: [privilege_session.go:35-114](../services/privilege_session.go#L35-L114)

The `PrivilegeSession` uses `go-mysql-server` library to:
1. Create in-memory `mysql` and `information_schema` databases
2. Create privilege tables with TEXT columns for flexibility
3. Start temporary MySQL server on a free port
4. Load privilege data from query results

**Key Tables Created** (with TEXT type for all columns):
- `mysql.user` - 31 columns
- `mysql.db` - 22 columns
- `mysql.tables_priv` - 8 columns
- `mysql.procs_priv` - 8 columns
- `mysql.role_edges` - 5 columns
- `mysql.global_grants` - 4 columns
- `mysql.proxies_priv` - 7 columns
- `mysql.infoschema_user_privileges` - 4 columns
- `mysql.infoschema_schema_privileges` - 5 columns
- `mysql.infoschema_table_privileges` - 6 columns

### Step 6: Three-Pass Policy Execution

**File**: [privilege_session_handler.go:716-882](../services/privilege_session_handler.go#L716-L882)

#### PASS-1: Super Privileges

- **Policy ID**: 1 (hardcoded)
- **Scope**: All actions on all objects for all databases
- **Result**: `actor_id=<id>, object_id=-1, dbmgt_id=-1`
- **Special**: Actors with super privileges skip PASS-2 and PASS-3

#### PASS-2: Action-Wide Privileges

- **Source**: DBGroupListPolicies table
- **Scope**: Specific action on all objects for all databases
- **Result**: `actor_id=<id>, object_id=-1, dbmgt_id=-1`
- **Cache**: Records granted actions to prevent redundant PASS-3 policies

#### PASS-3: Object-Specific Privileges

- **Scope**: Specific action on specific objects/databases
- **Skip conditions**:
  - Actor has super privileges (from PASS-1)
  - Action already granted (from PASS-2)
- **Result**: `actor_id=<id>, object_id=<id>, dbmgt_id=<id>`

### Step 7: Query Building for Sessions

**File**: [privilege_session_handler.go:1680-1934](../services/privilege_session_handler.go#L1680-L1934)

#### General SQL Templates (`processGeneralSQLTemplatesForSession`)

- Uses `SqlGet` field from `DBPolicyDefault`
- Variables substituted:
  - `${dbmgt.dbname}` → Database name
  - `${dbactormgt.dbuser}` → User name
  - `${dbactormgt.ip_address}` → Host/IP address
  - `${dbobjectmgt.objectname}` → Object name (set to `*` for wildcard)

#### Specific SQL Templates (`processSpecificSQLTemplatesForSession`)

- Uses `SqlGetSpecific` field from `DBPolicyDefault`
- Special handling for MySQL ObjectId:
  - **ObjectId 12**: User objects - substitute with all actors
  - **ObjectId 15**: Schema objects - skipped for MySQL

### Step 8: Policy Allowed Check

**File**: [dbpolicy_service.go:650-662](../services/dbpolicy_service.go#L650-L662)

```go
func (s *dbPolicyService) isPolicyAllowed(output, resAllow, resDeny string) bool {
    if output == resDeny {
        return false
    }
    if output == resAllow {
        return true
    }
    // Special case: "NOT NULL" means any non-null value is allowed
    if resAllow == "NOT NULL" && output != "NULL" {
        return true
    }
    return false
}
```

The query result is compared against:
- `SqlGetAllow` - Expected value for policy to be allowed
- `SqlGetDeny` - Value that denies the policy

### Step 9: Actor to Group Assignment

**File**: [privilege_session_handler.go:291-526](../services/privilege_session_handler.go#L291-L526)

Two-level strict exact-match strategy:
1. **Super privilege actors** → Auto-assign to group 1
2. **Other actors**:
   - Level 1: Actor must satisfy ALL policies in a listpolicy
   - Level 2: Actor must satisfy ALL listpolicies required by a group
   - Select lowest group_id among matched groups

### Step 10: Export DBF Policy Rules

**File**: [privilege_session_handler.go:873-879](../services/privilege_session_handler.go#L873-L879)

After policies are created, call `utils.ExportDBFPolicy()` to build rule files.

## Concurrency and Performance

### Parallel Query Execution

- Configurable concurrency via `config.GetPrivilegeQueryConcurrency()`
- Uses semaphore pattern to limit concurrent queries
- Results collected serially for database operations

### Query Build Cache

**File**: [privilege_session_handler.go:1596-1674](../services/privilege_session_handler.go#L1596-L1674)

Pre-loads all actors and objects data in batches to eliminate N+1 queries during query building.

## Key Data Structures

### policyInput

```go
type policyInput struct {
    policydf models.DBPolicyDefault
    actorId  uint
    objectId int
    dbmgtId  int    // -1 for all databases
    finalSQL string // Processed SQL with variables substituted
}
```

### PrivilegeSessionJobContext

```go
type PrivilegeSessionJobContext struct {
    CntMgtID      uint
    DbMgts        []models.DBMgt
    DbActorMgts   []models.DBActorMgt
    CMT           *models.CntMgt
    EndpointID    uint
    SessionID     string
    PrivilegeFile string
}
```

### Policy Classification

```go
type policyClassification struct {
    superPrivileges     []models.DBPolicyDefault // PASS-1: ID=1
    actionWidePrivs     []models.DBPolicyDefault // PASS-2: From DBGroupListPolicies
    objectSpecificPrivs []models.DBPolicyDefault // PASS-3: All others
}
```

## Query Rewriting

**File**: [privilege_session_handler.go:1412-1432](../services/privilege_session_handler.go#L1412-L1432)

Since `go-mysql-server` doesn't allow INSERT into `information_schema`, queries are rewritten:
- `information_schema.USER_PRIVILEGES` → `mysql.infoschema_user_privileges`
- `information_schema.SCHEMA_PRIVILEGES` → `mysql.infoschema_schema_privileges`
- `information_schema.TABLE_PRIVILEGES` → `mysql.infoschema_table_privileges`

## Error Handling

- Idempotency check prevents duplicate job processing
- Graceful handling of missing tables/columns
- Transaction rollback on failure
- Structured logging throughout

## Dependencies

- `github.com/dolthub/go-mysql-server` - In-memory MySQL server
- dbfAgentAPI - Remote command execution on agents
- GORM - Database ORM for policy persistence
