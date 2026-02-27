# DBF Artifact API - System Architecture

**Version:** 1.0
**Last Updated:** 2026-02-27
**Status:** Active Development

---

## Architecture Overview

The DBF Artifact API is a layered, service-oriented system for managing database access policies across heterogeneous database environments. It integrates with the dbfAgentAPI platform for remote policy execution and provides privilege discovery through in-memory database sessions.

### Architecture Layers

```
┌─────────────────────────────────────────────────────┐
│ HTTP Clients (cURL, SDKs, Web UI)                   │
└────────────────┬────────────────────────────────────┘
                 │
┌─────────────────────────────────────────────────────┐
│ Controllers Layer (Gin REST Handlers)               │
│ - Request validation                                │
│ - Response formatting                               │
│ - HTTP status codes                                 │
└────────────────┬────────────────────────────────────┘
                 │
┌─────────────────────────────────────────────────────┐
│ Services Layer (Business Logic)                     │
│ - Policy execution orchestration                    │
│ - Privilege discovery                               │
│ - Job monitoring                                    │
│ - Transaction coordination                          │
└────────────────┬────────────────────────────────────┘
                 │
┌─────────────────────────────────────────────────────┐
│ Repository Layer (Data Access)                      │
│ - CRUD operations                                   │
│ - Transaction management                            │
│ - Query abstraction                                 │
└────────────────┬────────────────────────────────────┘
                 │
┌─────────────────────────────────────────────────────┐
│ Models Layer (Domain Entities)                      │
│ - GORM entity definitions                           │
│ - Relationship mappings                             │
└────────────────┬────────────────────────────────────┘
                 │
┌─────────────────────────────────────────────────────┐
│ MySQL Database                                      │
└─────────────────────────────────────────────────────┘
```

---

## Component Architecture

### Core Components

```
┌─────────────────────────────────────────────────────────────────┐
│ API Endpoints (16 controllers)                                  │
├─────────────────────────────────────────────────────────────────┤
│ • DBMgt (connections)      • DBObjectMgt (objects)              │
│ • DBActorMgt (users)       • DBPolicy (policies)                │
│ • Group Management         • Job Status Monitoring              │
│ • Backup/Upload/Download   • PDB Management                     │
│ • Policy Compliance        • Session Management                 │
│ • Connection Testing                                            │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ Service Layer (33 services)                                     │
├─────────────────────────────────────────────────────────────────┤
│ Policy Services           Job Management (`services/job/`)  │
│ ├─ DBPolicy              ├─ JobMonitor                │
│ ├─ DBMgt                 ├─ PolicyCompletion          │
│ ├─ DBActorMgt            ├─ BulkCompletion            │
│ ├─ DBObjectMgt           ├─ ObjectCompletion          │
│ └─ Group Management      └─ Other Handlers            │
│                                                       │
│ Privilege Discovery (`services/privilege/`)            │
│ ├─ privilege/ (shared types, registry, session)        │
│ ├─ privilege/mysql/ (MySQL handler+session)            │
│ ├─ OraclePrivilegeSession  Infrastructure             │
│ └─ Oracle Handlers          ├─ AgentAPI               │
│                              ├─ Backup/Upload/Download│
│                              └─ Connection Testing    │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ Repository Layer (15 repositories)                              │
├─────────────────────────────────────────────────────────────────┤
│ • BaseRepository           • DBTypeRepository                   │
│ • CntMgtRepository         • DBActorRepository                  │
│ • DBMgtRepository          • DBObjectRepository                 │
│ • DBActorMgtRepository     • DBGroupMgtRepository               │
│ • DBObjectMgtRepository    • DBActorGroupsRepository            │
│ • DBPolicyRepository       • DBPolicyGroupsRepository           │
│ • EndpointRepository       • Others                             │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ GORM Models (18 entities)                                       │
├─────────────────────────────────────────────────────────────────┤
│ CntMgt, DBMgt, DBActorMgt, DBObjectMgt, DBPolicy                │
│ DBPolicyDefault, Endpoints, DBGroupMgt, DBGroupListPolicies    │
│ DBActorGroups, DBPolicyGroups, Backup, Upload, Download, etc.  │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ MySQL Database (20+ tables)                                     │
└─────────────────────────────────────────────────────────────────┘
```

---

## Data Flow Diagrams

### Policy Creation Flow

```
User Request (POST /api/queries/dbpolicy)
    │
    ├─→ Controller validates request
    │
    ├─→ Service creates policy
    │   ├─→ Repository creates DBPolicy record
    │   ├─→ Repository creates group assignments
    │   └─→ Transaction commits
    │
    ├─→ (Optional) Execute via dbfAgentAPI
    │   ├─→ Service builds hex-encoded JSON payload
    │   ├─→ Execute dbfAgentAPI with --background option
    │   ├─→ Return job_id to client
    │   │
    │   └─→ Job Monitor polls status
    │       ├─→ Every 10 seconds: checkstatus <job_id>
    │       ├─→ On completion: invoke callback
    │       │   ├─→ Download results from agent
    │       │   ├─→ Update policy records
    │       │   └─→ Create group assignments
    │       └─→ Notify client via webhook (optional)
    │
    └─→ Response to client
        ├─→ Synchronous: Policy created
        └─→ Async: Job queued, poll for status
```

### Privilege Discovery Flow (MySQL)

```
User Request (POST /api/queries/dbpolicy/get-by-cntmgt)
    │
    ├─→ Controller routes to DBPolicy service
    │
    ├─→ Service detects database type = MySQL
    │
    ├─→ Privilege Session Handler:
    │   ├─→ Create in-memory go-mysql-server
    │   ├─→ Connect to remote MySQL database
    │   │
    │   ├─→ Pass 1: Load SUPER privileges
    │   │   ├─→ Query: SELECT * FROM mysql.user WHERE Super_priv='Y'
    │   │   ├─→ Create policies for SUPER actors
    │   │   └─→ Assign to wildcard objects
    │   │
    │   ├─→ Pass 2: Load action-wide privileges
    │   │   ├─→ Query: mysql.db for database-level grants
    │   │   ├─→ Query: mysql.tables_priv for table-level grants
    │   │   └─→ Create policies for each actor+object combo
    │   │
    │   └─→ Pass 3: Load object-specific privileges
    │       ├─→ Query: mysql.columns_priv for column-level grants
    │       └─→ Create fine-grained policies
    │
    ├─→ Service auto-creates DBPolicy records
    │   └─→ Repository bulk insert within transaction
    │
    ├─→ Service auto-assigns actors to groups
    │   └─→ Repository creates DBActorGroups records
    │
    └─→ Response: {"created_policies": 150, "assigned_actors": 42}
```

### Privilege Discovery Flow (Oracle)

```
User Request (POST /api/queries/dbpolicy/get-by-cntmgt)
    │
    ├─→ Controller routes to DBPolicy service
    │
    ├─→ Service detects database type = Oracle
    │
    ├─→ Oracle Connection Helper:
    │   ├─→ Detect: CDB (Container DB) or PDB (Pluggable DB)
    │   │   └─→ Query: v$database.name patterns
    │   │
    │   └─→ Set connection scope (CDB vs PDB)
    │       └─→ If PDB: SELECT from pdb.sys.dba_sys_privs (isolated)
    │
    ├─→ Oracle Privilege Session Handler:
    │   ├─→ Create in-memory Oracle server
    │   │
    │   ├─→ Pass 1: Load system privileges
    │   │   ├─→ Query: sys.dba_sys_privs WHERE privilege IN (...)
    │   │   ├─→ Include role privileges via sys.role_sys_privs
    │   │   └─→ Create policies for each actor
    │   │
    │   ├─→ Pass 2: Load table privileges
    │   │   ├─→ Query: sys.dba_tab_privs
    │   │   └─→ Create table-level policies
    │   │
    │   └─→ Pass 3: Load column privileges
    │       ├─→ Query: sys.dba_col_privs
    │       └─→ Create column-level policies
    │
    ├─→ Service auto-creates DBPolicy records
    │
    └─→ Response: {"created_policies": 320, "assigned_actors": 85}
```

### Job Monitoring Flow

```
Background Job Submitted (job_id: "abc123")
    │
    ├─→ Service registers job with JobMonitor
    │   └─→ JobMonitor stores: job_id, type, callback function
    │
    ├─→ JobMonitor spawns polling goroutine
    │   │
    │   └─→ Every 10 seconds:
    │       ├─→ Execute: dbfAgentAPI --json cmd <agentID> 'dbfsqlexecute checkstatus abc123'
    │       ├─→ Parse response: {"status": "running"|"completed"|"failed", ...}
    │       │
    │       ├─→ If status == "running": continue polling
    │       ├─→ If status == "completed":
    │       │   ├─→ Invoke completion callback with results
    │       │   ├─→ Callback processes job results (download files, update DB)
    │       │   └─→ Remove job from monitor
    │       │
    │       └─→ If status == "failed": error handling
    │           ├─→ Retry up to AGENT_MAX_RETRIES times
    │           ├─→ Log error with context
    │           └─→ Notify client (if configured)
    │
    └─→ Client can query status: GET /api/jobs/abc123
        └─→ Returns: {"status": "completed", "result": {...}}
```

---

## Database Schema

### Entity Relationship Diagram

```
┌──────────────┐         ┌─────────────┐
│  endpoints   │←───────→│   cntmgt    │
│──────────────│  Agent  │─────────────│
│ client_id    │         │ type        │
│ agent_ip     │         │ host        │
│ agent_port   │         │ port        │
└──────────────┘         │ user        │
                         │ password    │
                         └──────┬──────┘
                                │
                    ┌───────────┼───────────┐
                    │           │           │
            ┌───────▼──────┐    │      ┌────▼─────────┐
            │   dbmgt      │    │      │ parent_id    │
            │──────────┐   │    │      │ (self-ref)   │
            │ cnt_id   │   │    │      │ Oracle CDB   │
            │ db_name  │   │    │      └──────────────┘
            │ db_type  │   │    │
            └────┬─────┘   │    │
                 │         │    │
    ┌────────────┼─────────┼────┼──────────────┐
    │            │         │    │              │
┌───▼────────┐  ┌▼──────────▼──┐  ┌──────────▼──────┐
│ dbobjectmgt│  │   dbactormgt │  │   dbpolicy      │
│────────────│  │──────────────│  │─────────────────│
│ db_mgt_id  │  │ cnt_id       │  │ cnt_mgt_id      │
│ object_name│  │ actor_name   │  │ db_mgt_id       │
│ object_type│  │ actor_type   │  │ db_actor_mgt_id │
└────────────┘  └┬─────────────┘  │ db_object_mgt   │
                 │                 │ sql_allow       │
         ┌───────▼──────────┐      │ sql_deny        │
         │ dbactor_groups   │      │ db_policy_def_id│
         │──────────────────│      └────────┬────────┘
         │ actor_id         │               │
         │ group_id         │               │
         │ created_at       │    ┌──────────┴──────────────┐
         └──────────────────┘    │                         │
                                 │                         │
         ┌───────────────────────▼──────┐  ┌───────────────▼────┐
         │  dbgroupmgt (hierarchical)   │  │  dbpolicydefault   │
         │───────────────────────────────│  │────────────────────│
         │ parent_group_id (self-ref)    │  │ policy_name        │
         │ group_name                    │  │ sql_get (hex)      │
         └──────────┬────────────────────┘  └────────────────────┘
                    │
         ┌──────────▼──────────────────┐
         │  dbpolicy_groups            │
         │──────────────────────────────│
         │ policy_id                    │
         │ group_id                     │
         │ db_group_list_policies_id    │
         │ created_at                   │
         └──────────────────────────────┘
```

### Key Tables

| Table | Purpose | Primary Key | Relationships |
|-------|---------|-------------|---------------|
| endpoints | Agent endpoints | client_id | Referenced by cntmgt.agent |
| cntmgt | Database connections | id | Parent to dbmgt, dbactormgt, dbpolicy |
| dbmgt | Database instances | id | Parent to dbobjectmgt, dbpolicy |
| dbactormgt | Database actors/users | id | Parent to dbpolicy, dbactor_groups |
| dbobjectmgt | Database objects | id | Referenced by dbpolicy |
| dbpolicy | Policy enforcement | id | Foreign keys to cntmgt, dbmgt, dbactormgt, dbobjectmgt |
| dbpolicydefault | Policy templates | id | Referenced by dbpolicy |
| dbgroupmgt | Hierarchical groups | id | Parent to dbactor_groups, dbpolicy_groups |
| dbgroup_listpolicies | Policy list definitions | id | Referenced by dbpolicy_groups |
| dbactor_groups | Actor-to-group mapping | id | Foreign keys to dbactormgt, dbgroupmgt |
| dbpolicy_groups | Policy-to-group mapping | id | Foreign keys to dbpolicy, dbgroupmgt |

---

## Service Architecture

### DBPolicy Service (Core)

**Responsibilities:**
- Create, update, delete policies
- Privilege discovery (MySQL/Oracle)
- Bulk policy operations
- Group assignment automation

**Key Methods:**
- `GetByCntMgt(cntID)` - Discover privileges for connection (most complex)
- `Create(policy)` - Create single policy
- `BulkUpdatePoliciesByActor(actor, policies)` - Bulk operations
- `Update(policy)`, `Delete(id)` - Standard CRUD

### Agent API Service (`services/agent/`)

**Responsibilities:**
- Build hex-encoded JSON payloads
- Execute commands via dbfAgentAPI
- Retry logic with exponential backoff
- Response parsing

**Key Methods:**
- `executeSqlAgentAPI()` - Execute SQL with background job
- `executeAgentAPISimpleCommand()` - Simple commands (checkstatus)
- `downloadFileAgentAPI()` - Download files from agent
- `executeConnectionTestAgentAPI()` - Test database connectivity

### Job Monitor Service (`services/job/`, Singleton)

**Responsibilities:**
- Register background jobs
- Poll job status every 10 seconds
- Invoke completion callbacks
- Job cleanup

**Key Methods:**
- `RegisterJob(jobID, jobInfo)` - Register with callback
- `GetJobStatus(jobID)` - Query current status
- `CancelJob(jobID)` - Cancel polling
- `Stop()` - Graceful shutdown

### Privilege Discovery Services (`services/privilege/`)

**Shared Package (`services/privilege/`):**
- `types.go` — Shared types: `PrivilegeSessionJobContext`, `QueryResult`, `PolicyEvaluator` interface, registry function types
- `registry.go` — Registry pattern to break circular dependency between `services/` and `privilege/mysql/`. Functions registered via `init()` in `services/dbpolicy_service.go`
- `session.go` — `PrivilegeSession` struct (in-memory go-mysql-server), `GetFreePort()`, `ExecuteInDatabase()`, `ExecuteTemplate()`

**Dependency Graph:**
```
services/privilege      (no imports of services/ or privilege/mysql/)
services/privilege/mysql (imports privilege, NOT services/)
services/                (imports privilege AND privilege/mysql, registers via init())
```

**MySQL Privilege Session (`services/privilege/mysql/`):**
- `session.go` — `NewMySQLPrivilegeSession()`, creates in-memory go-mysql-server with MySQL privilege tables
- `handler.go` — `CreatePrivilegeSessionCompletionHandler()`, three-pass policy engine (SUPER → action-wide → object-specific)

**Oracle Privilege Session (still in `services/`):**
1. Create in-memory Oracle server
2. Detect CDB vs PDB
3. Execute Oracle-specific queries
4. Handle role inheritance

### Completion Handlers

**Policy Completion Handler:**
- Download policy results from agent
- Parse policy records
- Create DBPolicy entries
- Assign to groups

**Bulk Completion Handler:**
- Process bulk update results
- Atomic consistency check
- Update multiple policies

**Object Completion Handler:**
- Download discovered objects
- Create DBObjectMgt records
- Update database asset inventory

**File Operations Completion Handlers** (`services/fileops/`):
- Backup, download, upload completion handlers

### Session & Connection Test Services (`services/session/`)

**Session Service:**
- Kill active database sessions via agent API

**Connection Test Service:**
- Test database connectivity via agent API
- Parse connection test results
- Update connection status in database

---

## Integration Points

### dbfAgentAPI Integration

**Command Format:**
```bash
dbfAgentAPI --json cmd <agentID> '<binary> <action> <hex_json> <options>'
```

**Examples:**
```bash
# Background job execution
dbfAgentAPI --json cmd agent-1 '/etc/v2/dbf/bin/dbfsqlexecute download <hex> --background'

# Job status check
dbfAgentAPI --json cmd agent-1 '/etc/v2/dbf/bin/dbfsqlexecute checkstatus <job_id>'

# File download
dbfAgentAPI getfile agent-1 /remote/path /local/path
```

**Response Format:**
```json
{
  "status": "success",
  "output": "{...}",
  "exit_code": 0
}
```

### Go-MySQL-Server Integration

**In-memory MySQL for privilege discovery:**
```go
// Create server
server := createMySQLServer()

// Load privilege data from remote MySQL
privileges := fetchRemotePrivileges(remoteDB)

// Execute queries against in-memory server
results := server.Execute(sqlQuery)

// Parse results into policy format
policies := parsePrivileges(results)
```

### GORM Integration

**Repository pattern with transactions:**
```go
tx := db.Begin()
repo.WithTx(tx).Create(entity)
tx.Commit()
```

---

## Security Architecture

### SQL Injection Prevention

1. **Hex-encoded templates** - Store templates as hex strings
2. **Parameterized queries** - GORM handles all SQL generation
3. **Input validation** - Validate at service boundaries
4. **Actor/object bounds** - Validate IDs are in expected ranges

### Integer Overflow Protection

- Custom converters in `utils/conversion.go`
- Bounds checking for policy parameters
- Safe casting from user input

### Authentication & Authorization

- **Note:** No built-in auth (assumed handled upstream)
- Logging includes audit trail for all changes
- Structured logs track: user, operation, timestamp, result

### Credential Management

- All secrets via environment variables
- Never logged or exposed in responses
- Database credentials loaded at startup only

---

## Performance Characteristics

### Scalability Metrics

| Operation | Typical Size | Time | Bottleneck |
|-----------|--------------|------|-----------|
| Policy creation | 1 policy | < 100ms | DB insert |
| Bulk policy create | 1000 policies | < 5s | Transaction commit |
| Privilege discovery | 10k privileges | < 30s | Remote DB load |
| Job polling | 100 jobs | < 100ms per job | Agent response |
| Group assignment | 10k assignments | < 10s | DB transaction |

### Resource Usage

- **Memory:** ~500MB baseline + 1MB per concurrent job
- **CPU:** Minimal (mostly I/O bound)
- **Disk:** Log rotation at 100MB per file
- **Database connections:** 50+ pooled connections

### Bottlenecks

1. **dbfAgentAPI response time** - Largest latency (100s+ for large jobs)
2. **Remote database load time** - Privilege discovery dependent on remote DB size
3. **Transaction commit** - Bulk operations block on final commit
4. **Network bandwidth** - File upload/download operations

---

## Deployment Architecture

### Single-Instance Deployment

```
┌────────────────────────────────────────┐
│  Linux Server (Go 1.24.1)              │
├────────────────────────────────────────┤
│  DBF Artifact API (port 8081)          │
│  ├─ Controllers                        │
│  ├─ Services                           │
│  └─ Repository Layer                   │
├────────────────────────────────────────┤
│  Job Monitor Service (background)      │
│  └─ Polling every 10s                  │
├────────────────────────────────────────┤
│  Logger (lumberjack rotation)          │
└────────────────────────────────────────┘
         ↓              ↓              ↓
    MySQL DB       dbfAgentAPI      Log Files
    (remote)       (remote)         (local)
```

### Environment Variables

**Database:** DB_HOST, DB_PORT, DB_USER, DB_PASS, DB_NAME
**Server:** PORT (default 8081)
**Logging:** LOG_FILE, LOG_LEVEL, LOG_MAX_SIZE, LOG_MAX_BACKUPS, LOG_COMPRESS
**Paths:** DBFWEB_TEMP_DIR, AGENT_API_PATH
**Timeouts:** AGENT_EXECUTION_TIMEOUT, AGENT_MAX_RETRIES
**Concurrency:** PRIVILEGE_LOAD_CONCURRENCY, PRIVILEGE_QUERY_CONCURRENCY

---

## Error Handling Strategy

### Error Categories

1. **Validation Errors** (400 HTTP)
   - Invalid request structure
   - Missing required fields
   - Out-of-bounds values

2. **Not Found Errors** (404 HTTP)
   - Database record not found
   - Connection doesn't exist

3. **Conflict Errors** (409 HTTP)
   - Policy already exists
   - Duplicate group assignment

4. **Server Errors** (500 HTTP)
   - Database connection failure
   - dbfAgentAPI unavailable
   - Unexpected exception

### Error Recovery

- **Transient failures:** Retry with exponential backoff
- **Database errors:** Rollback transaction, return error
- **Agent failures:** Retry AGENT_MAX_RETRIES times
- **Job failures:** Log error, notify client, mark as failed

---

## Testing Architecture

### Unit Tests
- Pure business logic (no external dependencies)
- Mock repositories using mockery
- Coverage: validation, error handling, algorithms

### Integration Tests
- Real database (test MySQL instance)
- Real dbfAgentAPI calls
- Full request/response cycle
- Marked with `//go:build integration`

### Test Patterns

```bash
# Run unit tests
go test ./...

# Run integration tests only
go test -tags=integration ./...

# Run with coverage
go test -cover ./...
```

---

**Last Updated:** 2026-02-27
**Architecture Owner:** DBF Architecture Team
**Next Review:** 2026-05-24
