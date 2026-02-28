# DBF Artifact API - Codebase Summary

**Project:** DBF Artifact API (Database Firewall/Security Policy Management System)
**Language:** Go 1.24.1
**Total LOC:** 24,054 (94 files, excluding mocks and docs)
**Architecture:** Layered (Controllers → Services → Repository → Models → Database)

## Directory Structure

```
dbfartifactapi_151/
├── main.go (124 LOC)              - Entry point, route registration, graceful shutdown
├── config/                         - Configuration and database setup
│   ├── config.go (200 LOC)        - AppConfig struct, 30+ env var fields
│   └── database.go (91 LOC)       - GORM MySQL connection, singleton pattern
├── controllers/ (3,075 LOC, 16 files) - Gin REST endpoint handlers
│   ├── *_controller.go            - Entity CRUD endpoints (dbmgt, dbactormgt, etc)
│   ├── group_management_controller.go - Group CRUD + assignment (~800 LOC)
│   ├── backup_controller.go       - POST /backup background jobs
│   ├── job_status_controller.go   - GET /api/jobs/* monitoring
│   └── swagger_*.go               - Swagger model definitions
├── services/ (17,464 LOC, 33 files) - Business logic + dbfAgentAPI orchestration
│   ├── agent/agent_api_service.go (563 LOC) - Core dbfAgentAPI integration (sub-package)
│   ├── entity/ (sub-package)        - DBMgt, DBActorMgt, DBObjectMgt CRUD + object completion handler
│   ├── policy/ (sub-package)         - DBPolicy CRUD + privilege discovery + completion handlers (Phase 8)
│   ├── pdb/ (sub-package)            - PDB management services (Phase 8)
│   ├── group/ (sub-package)          - Group management CRUD + assignments (Phase 9)
│   ├── compliance/ (sub-package)     - Policy compliance monitoring + completion handlers (Phase 10)
│   ├── fileops/ (sub-package)        - Backup, download, upload services + completion handlers
│   ├── session/ (sub-package)        - Session kill + connection test services
│   ├── job/ (sub-package)            - Job monitor service + job types
│   ├── privilege/ (sub-package)      - Shared privilege types, registry, session base
│   │   ├── mysql/ (sub-package)     - MySQL in-memory privilege discovery
│   │   └── oracle/ (sub-package)    - Oracle in-memory privilege discovery
│   └── dto/                          - Unchanged
├── models/ (390 LOC, 18 files) - GORM domain entities
│   ├── cntmgt_model.go           - Connection management (MySQL/Oracle/PG/MSSQL)
│   ├── dbmgt_model.go            - Database instances
│   ├── dbactormgt_model.go       - Database actors (users)
│   ├── dbobjectmgt_model.go      - Database objects
│   ├── dbpolicy_model.go         - Policy enforcement records
│   ├── dbpolicydefault_model.go  - Policy templates (hex-encoded SQL)
│   ├── dbgroupmgt_model.go       - Hierarchical groups (self-ref FK)
│   └── *_model.go               - Other domain entities
├── repository/ (1,445 LOC, 15 files) - GORM data access layer
│   ├── base_repository.go        - Transaction management, Begin(), Commit()
│   ├── *_repository.go           - CRUD operations for each entity
│   └── interfaces: BaseRepository, CntMgtRepository, DBMgtRepository, etc
├── bootstrap/
│   └── load_data.go (119 LOC)   - Bootstrap 5 lookup tables into memory maps
├── utils/ (653 LOC, 7 files) - Utilities and helpers
│   ├── validator.go             - Struct validation, MySQL host validation
│   ├── conversion.go            - Safe int/uint converters (CWE-190)
│   ├── logger.go                - Gin middleware, response helpers
│   ├── hash.go                  - MD5 file hashing
│   ├── compression.go           - tar.gz compression
│   ├── policy_export.go         - exportDBFPolicy shell command
│   └── db_query_param.go        - Hex-encoded JSON payload builders
├── pkg/logger/ (298 LOC) - Structured logger with lumberjack rotation
├── mocks/                   - mockery-generated repository mocks
├── docs/                    - Technical documentation
├── .env.example            - Environment variable template
├── go.mod/go.sum          - Dependency management
└── CLAUDE.md              - Project coding standards & guidelines
```

## File Inventory by Layer

### Controllers (3,075 LOC, 16 files)

| File | LOC | Purpose |
|------|-----|---------|
| dbmgt_controller.go | 200 | Database management CRUD |
| dbactormgt_controller.go | 250 | Actor management CRUD |
| dbobjectmgt_controller.go | 220 | Object management CRUD |
| dbpolicy_controller.go | 350 | Policy CRUD + bulk operations |
| group_management_controller.go | 800 | Group CRUD + policy/actor assignment |
| session_controller.go | 100 | Kill database sessions |
| connection_test_controller.go | 120 | Test database connections |
| policy_compliance_controller.go | 100 | Compliance check initiation |
| job_status_controller.go | 200 | Job monitoring endpoints |
| backup_controller.go | 150 | Backup job submission |
| upload_controller.go | 100 | Upload file submission |
| download_controller.go | 100 | Download file submission |
| pdb_controller.go | 150 | Oracle PDB CRUD |
| swagger_examples.go | 200 | Swagger endpoint examples |
| backup_swagger_models.go | 100 | Swagger model definitions |
| upload_swagger_models.go | 75 | Swagger model definitions |

### Services (17,464 LOC, 33 files)

**Core Services:**
- group_management_service.go → `services/group/group_management_service.go` (1,963 LOC) - Group/policy/actor assignments (Phase 9)
- policy_compliance_service.go → `services/compliance/policy_compliance_service.go` (139 LOC) - Compliance check orchestration (Phase 10)

**Policy Services (`services/policy/`, Phase 8):**
- policy/dbpolicy_service.go (1,340 LOC) - GetByCntMgt, Create, Update, Delete, Bulk operations
- policy/policy_completion_handler.go (966 LOC) - Policy job completion callbacks
- policy/bulk_policy_completion_handler.go (298 LOC) - Bulk policy completion
- policy/oracle_privilege_queries.go - Oracle privilege query builders
- policy/init.go - Registry registration (breaks circular dependency)

**PDB Services (`services/pdb/`, Phase 8):**
- pdb/pdb_service.go (533 LOC) - PDB management CRUD

**Group Management Services (`services/group/`, Phase 9):**
- group/group_management_service.go (1,963 LOC) - Group CRUD + policy/actor assignments

**Compliance Services (`services/compliance/`, Phase 10):**
- compliance/policy_compliance_service.go (139 LOC) - Compliance check orchestration
- compliance/policy_compliance_completion_handler.go (321 LOC) - Compliance result processing

**Infrastructure Services:**
- agent/agent_api_service.go (563 LOC) - dbfAgentAPI orchestration (sub-package)
- job/job_monitor_service.go (634 LOC) - Job polling + callbacks (sub-package)
- fileops/backup_service.go (466 LOC), fileops/download_service.go (196 LOC), fileops/upload_service.go (145 LOC) - (sub-package)
- session/session_service.go (129 LOC), session/connection_test_service.go (144 LOC) - (sub-package)
- policy_compliance_completion_handler.go (321 LOC)

**Privilege Discovery — Shared (`services/privilege/`):**
- privilege/types.go (~60 LOC) - Shared types: PrivilegeSessionJobContext, QueryResult, PolicyEvaluator interface, registry func types
- privilege/registry.go (~72 LOC) - Registry pattern: RegisterNewPolicyEvaluator, RegisterRetrieveJobResults, RegisterGetEndpointForJob
- privilege/session.go (~101 LOC) - PrivilegeSession struct, ExecuteTemplate, ExecuteInDatabase, GetFreePort

**Privilege Discovery (MySQL) (`services/privilege/mysql/`):**
- privilege/mysql/session.go (~200 LOC) - NewMySQLPrivilegeSession, MySQL privilege table schemas, data loading
- privilege/mysql/handler.go (~1,991 LOC) - CreatePrivilegeSessionCompletionHandler, three-pass engine

**Privilege Discovery (Oracle) (`services/privilege/oracle/`):**
- privilege/oracle/handler.go (~1,600 LOC) - CreateOraclePrivilegeSessionCompletionHandler, three-pass engine
- privilege/oracle/privilege_session.go (~107 LOC) - In-memory Oracle setup
- privilege/oracle/queries.go (~275 LOC) - Oracle-specific queries
- privilege/oracle/connection_helper.go (~74 LOC) - CDB/PDB detection

**Job Completion Handlers:**
- policy/policy_completion_handler.go (966 LOC) - (in policy/)
- policy/bulk_policy_completion_handler.go (298 LOC) - (in policy/)
- entity/object_completion_handler.go (824 LOC) - (in entity/)
- fileops/backup_completion_handler.go (347 LOC) - (in fileops/)
- fileops/download_completion_handler.go (76 LOC) - (in fileops/)
- fileops/upload_completion_handler.go (133 LOC) - (in fileops/)
- policy_compliance_completion_handler.go (321 LOC)

### Models (390 LOC, 18 files)

| Model | Purpose | Key Fields |
|-------|---------|-----------|
| CntMgt | Connection management | Type (MySQL/Oracle/PG/MSSQL), Host, Port, Agent (FK to Endpoints) |
| DBMgt | Database instances | CntID (FK), DBName, DBType |
| DBActorMgt | Database actors/users | CntID (FK), ActorName, ActorType |
| DBObjectMgt | Database objects | DBMgtID (FK), ObjectName, ObjectType |
| DBPolicy | Policy enforcement | CntMgtID, DBMgtID, DBActorMgtID, DBObjectMgtID, SqlGetAllow, SqlGetDeny |
| DBPolicyDefault | Policy templates | PolicyName, SqlGet (hex-encoded) |
| Endpoints | Agent endpoints | ClientID, AgentIP, AgentPort |
| DBGroupMgt | Hierarchical groups | ParentGroupID (self-ref FK) |
| DBGroupListPolicies | Policy list definitions | PolicyName, RiskLevel |
| DBActorGroups | Actor-to-group mapping | ActorID, GroupID, CreatedAt, UpdatedAt |
| DBPolicyGroups | Policy-to-group mapping | PolicyID, GroupID, CreatedAt, UpdatedAt |

### Repository (1,445 LOC, 15 files)

All repositories follow the same pattern:
1. Interface definition with CRUD methods
2. Unexported struct `func (r *xxxRepository)`
3. Constructor `func NewXxxRepository(db *gorm.DB)`
4. Transaction support: `func (r *xxxRepository) WithTx(tx *gorm.DB)`

**Repositories:**
- BaseRepository - Begin(), Commit(), Rollback(), WithTx()
- CntMgtRepository, DBMgtRepository, DBActorMgtRepository
- DBObjectMgtRepository, DBPolicyRepository, DBTypeRepository
- DBActorRepository, DBObjectRepository, DBGroupMgtRepository
- DBGroupListPoliciesRepository, DBActorGroupsRepository, DBPolicyGroupsRepository
- EndpointRepository

## Key Architectural Patterns

### 1. Layered Architecture
```
Controllers (HTTP handlers)
      ↓
Services (Business logic + orchestration)
      ↓
Repository (Data access)
      ↓
Models (Domain entities)
      ↓
MySQL Database
```

### 2. dbfAgentAPI Integration Flow
1. Service builds hex-encoded JSON payload
2. Call `agent.ExecuteSqlAgentAPI()` with retry logic
3. For background jobs: return job_id
4. Register with `job_monitor_service.RegisterJob()` + completion callback
5. Job monitor polls `dbfsqlexecute checkstatus` every 10 seconds
6. On completion, trigger callback function
7. Callback processes results + updates database

### 3. Privilege Session Discovery
**MySQL flow:**
1. Controller calls `dbpolicy_service.GetByCntMgt(cntID)`
2. Service detects MySQL, creates in-memory go-mysql-server
3. Downloads privilege data from remote MySQL
4. Executes three-pass policy engine
5. Auto-creates DBPolicy + group assignments

**Oracle flow:**
1. Similar pattern, detects Oracle via connection type
2. Uses different in-memory server setup
3. Executes Oracle-specific privilege queries
4. Three-pass execution with Oracle syntax

### 4. Transaction Pattern
```go
// Service layer initiates transaction
tx := baseRepo.Begin()
if err != nil {
    return tx.Rollback().Error
}
// Pass tx to all repo calls
userRepo.WithTx(tx).Create(user)
actorRepo.WithTx(tx).Create(actor)
// Repo or service commits
return tx.Commit().Error
```

### 5. Dependency Injection
**Controllers:**
- Most use package-level vars + `Set*Service()` setters
- Called from main.go during startup
- Some (ConnectionTest, JobStatus) use struct-based DI

**Services:**
- All require Repository interfaces, accept GORM *db
- Constructor injection: `NewDBPolicyService(baseRepo, dbMgtRepo, ...)`
- Enables easy mocking for unit tests

### 6. Job Completion Callback Pattern
```go
// Register job with callback
jobMonitor.RegisterJob(jobID, &JobInfo{
    Type: "policy",
    OnComplete: func(jobID, jobInfo, statusResp) error {
        // Process results from dbfAgentAPI
        // Update database
        return nil
    },
})
// Monitor polls; callback invoked on completion
```

## Dependency Graph

### External Dependencies
- **github.com/gin-gonic/gin** (1.10.0) - REST framework
- **gorm.io/gorm** (1.25.12) - ORM
- **gorm.io/driver/mysql** (1.5.7) - MySQL driver
- **github.com/dolthub/go-mysql-server** (0.20.0) - In-memory MySQL
- **github.com/go-playground/validator** (10.20.0) - Validation
- **gopkg.in/natefinish/lumberjack** (2.2.1) - Log rotation
- **github.com/swaggo/swag** (1.16.4) - Swagger code generation

### Internal Dependencies
```
Controllers → Services → Repository → Models → GORM
           ↘ Services/agent (dbfAgentAPI)
           ↘ Services/entity (DBMgt, DBActorMgt, DBObjectMgt CRUD)
           ↘ Services/policy (Policy CRUD + privilege discovery)
           ↘ Services/pdb (PDB management)
           ↘ Services/group (Group management + assignments) - Phase 9
           ↘ Services/compliance (Compliance monitoring) - Phase 10
           ↘ Services/fileops (backup/download/upload)
           ↘ Services/session (session/connection-test)
           ↘ Services/job (job monitoring)
           ↘ Services/privilege (shared types, registry, session base)
           ↘ Services/privilege/mysql (MySQL privilege discovery)
           ↘ Services/privilege/oracle (Oracle privilege discovery)
           ↘ utils (validation, logger, conversion)
           ↘ pkg/logger (structured logging)
           ↘ config (DB connection, env vars)
           ↘ bootstrap (startup data loading)
```

## Configuration (30+ env vars)

**Database:** DB_HOST, DB_PORT, DB_USER, DB_PASS, DB_NAME
**Server:** PORT (default 8081)
**Logging:** LOG_LEVEL, LOG_FILE, LOG_MAX_SIZE, LOG_MAX_BACKUPS, LOG_MAX_AGE, LOG_COMPRESS
**Paths:** AGENT_API_PATH, DBFWEB_TEMP_DIR, VELO_RESULTS_DIR, NOTIFICATION_FILE_DIR, DOWNLOAD_FILE_DIR
**Agent API:** AGENT_EXECUTION_TIMEOUT, AGENT_MAX_RETRIES, AGENT_RETRY_BASE_DELAY
**Concurrency:** PRIVILEGE_LOAD_CONCURRENCY, PRIVILEGE_QUERY_CONCURRENCY, ENABLE_MYSQL_PRIVILEGE_QUERY_LOGGING

## Database Schema Relationships

```
endpoints ←─── CntMgt.Agent (FK)
                 ↓ CntMgt.ParentConnectionID (self-ref, Oracle PDB→CDB)
              DBMgt ←─── DBObjectMgt.DBMgt
                 ↓
              DBPolicy.DBMgt
                 ↓
              DBActorMgt ←─── DBPolicy.DBActorMgt
                 ↓
              DBActorGroups ←─── DBGroupMgt
                 ↓
              DBPolicyGroups ←─── DBGroupListPolicies
                 ↓
              DBPolicyDefault ←─── DBPolicy.DBPolicyDefault
```

## Key Design Decisions

1. **Hex-encoded SQL templates** - Prevents SQL injection by keeping templates encoded until execution time
2. **In-memory privilege sessions** - go-mysql-server enables privilege discovery without needing actual MySQL/Oracle instances
3. **Background job pattern** - dbfAgentAPI uses `--background` option for reliability + polling mechanism
4. **Three-pass policy engine** - Separates super privileges, action-wide, and object-specific checks
5. **Group hierarchies** - Self-referencing FKs enable nested group structures
6. **Atomic consistency** - Transactions ensure policy + group assignments succeed or fail together
7. **Safe integer conversion** - CWE-190 mitigation for integer overflow scenarios

## Testing Infrastructure

- **mockery** - Generates repository mocks (config: .mockery.yaml, output: mocks/)
- **go-sqlmock** - SQL mocking for database tests
- **Unit tests** - Pure logic without external dependencies
- **Integration tests** - With real database + dbfAgentAPI orchestration
- **Skip pattern** - Document unmockable code with test skip + comment

## Performance Characteristics

| Operation | Pattern | Scale |
|-----------|---------|-------|
| Policy creation | Atomic transaction | 1-1000 policies/request |
| Privilege discovery | In-memory session + polling | 100-10k privileges |
| Job monitoring | Periodic polling (10s interval) | 100s concurrent jobs |
| Group assignment | Bulk update transaction | 1000+ assignments |
| Backup/Upload | Background job + callback | Multi-GB files |

## Security Features

1. **SQL injection prevention** - Hex-encoded templates + parameterized queries
2. **Input validation** - go-playground/validator at controller boundaries
3. **Integer overflow protection** - Safe converters in utils/conversion.go
4. **Structured logging** - Audit trail with context (zap-style)
5. **Graceful shutdown** - Signal handling for active job cleanup
6. **No hardcoded credentials** - All secrets via environment variables

---

**Last Updated:** 2026-02-28
**Maintainer:** DBF Architecture Team
