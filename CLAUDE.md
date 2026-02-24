# CLAUDE.md

## Project Context

**DBF Artifact API** - Database Firewall/Security Policy Management System in Go. Manages database access policies across multiple instances, integrates with **dbfAgentAPI** for remote policy execution on agents.

### Key Architecture
- **Controllers**: Gin REST endpoints under `/api/queries/`
- **Services**: Business logic + dbfAgentAPI orchestration
- **Repository**: GORM data access with MySQL
- **Models**: Domain entities

### Critical Domain Flow
`DBPolicyService.GetByDBMgt()` - Most complex operation:
1. Hex-decode SQL templates from `DBPolicyDefault`
2. Variable substitution (`${dbmgt.dbname}`, `${dbactormgt.dbuser}`)
3. Write JSON batches to `DBFWEB_TEMP_DIR`
4. Execute via dbfAgentAPI background jobs
5. Poll job status until completion
6. Auto-create `DBPolicy` records based on `SqlGetAllow`/`SqlGetDeny` rules

### Security-Critical Details
- **SQL Templates**: Stored hex-encoded to prevent injection
- **dbfAgentAPI Commands**: `dbfAgentAPI --json cmd <agentID> '/etc/v2/dbf/bin/dbfsqlexecute download <hex_json> --background'`
- **Job Monitoring**: `dbfsqlexecute checkstatus <job_id>` until "completed"

### dbfAgentAPI Command Formats
```bash
# SQL execute (immediate)
dbfAgentAPI --json cmd <agentID> '/etc/v2/dbf/bin/dbfsqlexecute execute <hex_json>'

# Background job (download, policycompliance)
dbfAgentAPI --json cmd <agentID> '/etc/v2/dbf/bin/dbfsqlexecute download <hex_json> --background'

# Simple commands (checkstatus, getresults)
dbfAgentAPI --json cmd <agentID> '/etc/v2/dbf/bin/dbfsqlexecute checkstatus <job_id>'
dbfAgentAPI --json cmd <agentID> '/etc/v2/dbf/bin/dbfsqlexecute getresults <job_id>'

# File download from agent
dbfAgentAPI getfile <agentID> <remote_path> <local_path>

# Connection test
dbfAgentAPI --json cmd <agentID> '/etc/v2/dbf/bin/v2dbfsqldetector --action test_connection --type mysql --host <host> --port <port> --username <user> --password <pass>'
```

### Binary Paths
- **Linux**: `/etc/v2/dbf/bin/dbfsqlexecute`, `/etc/v2/dbf/bin/v2dbfsqldetector`
- **Windows**: `C:/PROGRA~1/V2/DBF/bin/dbfsqlexecute`, `C:/PROGRA~1/V2/DBF/bin/sqldetector.exe`

## Build Commands
```bash
go build -o dbfartifactapi.exe .    # Build
./dbfartifactapi.exe                # Run (needs .env)
swag init                           # Update Swagger docs
```

## Go Coding Standards - MANDATORY

### Comment Requirements - CRITICAL

**ONLY comment WHY, not WHAT:**
```go
// GOOD: Business context and warnings
// User ID must be > 0 per business rule BR-001
// Zero indicates anonymous user which violates security policy
if userID == 0 {
    return errors.New("invalid user ID")
}

// BAD: Explaining obvious code actions
// Creating a new variable to store the user ID
userID := req.UserID
```

**Function Comments - Required:**
```go
// GetUserByID retrieves user record by unique identifier.
// Returns ErrUserNotFound if no user exists with given ID.
// Returns ErrDatabaseConnection if unable to connect to database.
func GetUserByID(id uint64) (*User, error)
```

**Complex Logic - Explain WHY:**
```go
// CRITICAL: Hex-decoding prevents SQL injection during template storage
// Templates must remain encoded until execution time for security
sqlTemplate, err := hex.DecodeString(template.SqlGet)
```

### Error Handling
```go
// ALWAYS wrap errors with business context
if err != nil {
    return fmt.Errorf("failed to execute policy for database %s: %w", dbName, err)
}
```

### Function Design
- **Single responsibility** - One function, one purpose
- **Input validation** - Validate at service boundaries
- **Context usage** - Always use `context.Context` for cancellation

### Database Patterns
```go
// Use transactions for multi-step operations
return s.db.Transaction(func(tx *gorm.DB) error {
    // Multiple operations here
})

// Define interfaces for testability
type PolicyRepository interface {
    Create(ctx context.Context, policy *DBPolicy) error
    GetByID(ctx context.Context, id uint64) (*DBPolicy, error)
}
```

### Security Requirements
- **Input validation** - Sanitize all user inputs
- **SQL injection prevention** - Use parameterized queries only
- **No hardcoded credentials** - Use environment variables

### Testing Standards

**Test Naming:** `TestFunction_Scenario_Expected`
```go
func TestGetPolicy_ValidID_ReturnsPolicy(t *testing.T)
func TestCreate_DuplicateDatabase_ReturnsError(t *testing.T)
```

**Mocking:** Use `mockery` - config: `.mockery.yaml`, run: `mockery`, output: `mocks/`

**Unit Tests** - Pure business logic without external dependencies:
- Validation rules
- Error handling
- Data transformations
- Algorithm logic

**Integration Tests** - Components interacting with external systems:
- Database operations (transactions, queries, migrations)
- External API calls (dbfAgentAPI executor, HTTP clients)
- File system operations (temp files, config loading)
- Background jobs and polling mechanisms
- Full request/response cycles (controller → service → repository → DB)

**Skip Pattern for Coupled Code:**
```go
func TestCreate_WithTransactions(t *testing.T) {
    t.Skip("Requires transaction mocking. See integration tests.")
    // Document test intent in comments
}
```

**Dependency Injection Pattern:**
```go
// Production: wire real dependencies
func NewDBMgtService() DBMgtService

// Testing: inject mocks
func NewDBMgtServiceWithDeps(
    baseRepo repository.BaseRepository,
    dbMgtRepo repository.DBMgtRepository,
    ...
) DBMgtService
```

**Integration Tests:**
```go
//go:build integration
// Run: go test -tags=integration ./...

func TestDBMgt_Integration_CreateAndDelete(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()
    // Test with real database + dbfAgentAPI
}
```

**Philosophy:** Quality over coverage. Skip unit tests when mocking is impractical.

### Logging - Structured Only
```go
logger.Error("dbfAgentAPI job failed",
    zap.String("job_id", jobID),
    zap.String("database", dbName),
    zap.Error(err),
)
```

## CRITICAL RULES - NEVER VIOLATE

1. **NEVER ignore errors** - Always handle or propagate
2. **NEVER use panic()** in production code
3. **NEVER hardcode credentials** or sensitive data
4. **NEVER use `fmt.Print*`** for logging - use structured logger
5. **NEVER create goroutines without cleanup strategy**
6. **ALWAYS validate inputs** at service boundaries
7. **ALWAYS use context.Context** for cancellation
8. **ALWAYS document public APIs** with comprehensive comments

## Project-Specific Notes

### dbfAgentAPI Integration
- Background job pattern required for reliability (use `--background` option)
- Job status polling is critical - don't skip
- Response format: `{"status": "success|error|timeout", "output": "...", "exit_code": 0}`

### Key Service Functions
- `executeSqlAgentAPI()` - Execute SQL commands with hex-encoded JSON
- `executeAgentAPISimpleCommand()` - Simple commands like checkstatus, getresults
- `downloadFileAgentAPI()` - Download files from agent
- `executeConnectionTestAgentAPI()` - Test database connections

### Bootstrap Data
- `DBPolicyDefaultsAllMap` loaded at startup
- Critical for policy template lookups
- Memory-resident for performance
