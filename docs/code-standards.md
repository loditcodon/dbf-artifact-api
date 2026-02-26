# DBF Artifact API - Code Standards & Guidelines

**Version:** 1.0
**Last Updated:** 2026-02-24
**Language:** Go 1.24.1
**Status:** Active

---

## Overview

This document defines mandatory coding standards for the DBF Artifact API project. All code must follow these standards before merging to main branch.

**Key Principle:** Code is read 10x more often than written. Optimize for clarity and maintainability.

---

## Go Language Standards

### Comment Requirements - CRITICAL

**ONLY comment WHY, not WHAT.** Explain business context, security implications, or non-obvious decisions. Do NOT explain what the code obviously does.

#### Bad Comments (What, not Why)
```go
// Creating a new user variable
var userID = req.UserID

// Check if user ID is zero
if userID == 0 {
    return errors.New("invalid user ID")
}
```

#### Good Comments (Why, not What)
```go
// User ID must be > 0 per business rule BR-001
// Zero indicates anonymous user which violates security policy
if userID == 0 {
    return errors.New("invalid user ID")
}
```

### Function Documentation - Required

Every public function must have a comment documenting:
1. **What it does** (brief, 1 sentence)
2. **Return values** including error cases
3. **Important side effects** (database changes, file I/O, etc.)

#### Format
```go
// GetUserByID retrieves the user record by unique identifier.
// Returns ErrUserNotFound if no user exists with the given ID.
// Returns ErrDatabaseConnection if unable to connect to database.
func GetUserByID(ctx context.Context, id uint64) (*User, error) {
    // ...
}
```

### Complex Logic - Explain Decisions

Add WHY comments for non-obvious algorithms or security-critical code:

```go
// CRITICAL: Hex-decoding prevents SQL injection during template storage.
// Templates must remain encoded until execution time for security.
// Raw SQL cannot be stored directly without injection risk.
sqlTemplate, err := hex.DecodeString(template.SqlGet)
if err != nil {
    return fmt.Errorf("failed to decode policy template: %w", err)
}
```

### Error Handling - Always Wrap with Context

Never return bare errors. Wrap with business context using `fmt.Errorf`:

```go
// BAD
if err != nil {
    return err
}

// GOOD
if err != nil {
    return fmt.Errorf("failed to execute policy for database %s: %w", dbName, err)
}
```

**Why:** Debugging becomes trivial when errors include context. Stack traces are useless without knowing WHAT failed and WHY.

### Naming Conventions

| Type | Convention | Example |
|------|-----------|---------|
| Variables | camelCase | `userID`, `dbConnection` |
| Constants | UPPER_SNAKE_CASE | `MAX_RETRIES`, `DEFAULT_TIMEOUT` |
| Functions | PascalCase (public), camelCase (private) | `CreateUser`, `validateInput` |
| Structs | PascalCase | `User`, `DBPolicy` |
| Interfaces | PascalCase | `UserRepository`, `PolicyService` |
| Methods | PascalCase | `(r *User) GetID()` |
| Packages | lowercase, single word | `utils`, `models`, `services` |

### Variable Scope Rules

- **Short scope** (< 5 lines) → Short names OK: `i`, `v`, `err`
- **Medium scope** (5-30 lines) → Descriptive names: `userID`, `dbConnection`
- **Long scope** (> 30 lines) → Very descriptive: `policyConnectionID`, `targetDatabaseName`

### Function Design Rules

1. **Single Responsibility** - One function, one purpose
   ```go
   // Good: Creates policy, separate function validates
   func CreatePolicy(ctx context.Context, req *CreatePolicyRequest) error {
       if err := validatePolicyRequest(req); err != nil {
           return err
       }
       // Create logic
   }
   ```

2. **Input Validation** - Validate at service boundaries (controllers call services)
   ```go
   // Service validates before processing
   func (s *dbpolicyService) Create(ctx context.Context, req *CreatePolicyRequest) error {
       if err := validateRequest(req); err != nil {
           return fmt.Errorf("invalid request: %w", err)
       }
       // Business logic
   }
   ```

3. **Context Usage** - Always pass context.Context as first parameter
   ```go
   // GOOD
   func (s *dbpolicyService) GetByID(ctx context.Context, id uint64) (*DBPolicy, error)

   // BAD
   func (s *dbpolicyService) GetByID(id uint64) (*DBPolicy, error)
   ```

4. **Return Values** - Keep to max 2 (data, error)
   ```go
   // Good: 2 returns
   func GetPolicy(ctx context.Context, id uint64) (*DBPolicy, error)

   // Bad: 3+ returns (use struct if needed)
   func GetPolicy(ctx context.Context, id uint64) (*DBPolicy, int, error)
   ```

5. **Dependency Injection** - Pass dependencies, don't create them
   ```go
   // Good: Injected repo for testing
   func NewDBPolicyService(repo repository.DBPolicyRepository) *dbpolicyService

   // Bad: Creates repo internally (hard to test)
   func NewDBPolicyService() *dbpolicyService {
       repo := repository.NewDBPolicyRepository()
   }
   ```

---

## Database & ORM Patterns

### Repository Pattern - Mandatory

All database access goes through repositories:

```go
// Interface definition (required for mocking)
type DBPolicyRepository interface {
    Create(ctx context.Context, policy *DBPolicy) error
    GetByID(ctx context.Context, id uint64) (*DBPolicy, error)
    Update(ctx context.Context, policy *DBPolicy) error
    Delete(ctx context.Context, id uint64) error
}

// Implementation
type dbpolicyRepository struct {
    db *gorm.DB
}

func (r *dbpolicyRepository) Create(ctx context.Context, policy *DBPolicy) error {
    return r.db.WithContext(ctx).Create(policy).Error
}

// Constructor
func NewDBPolicyRepository(db *gorm.DB) DBPolicyRepository {
    return &dbpolicyRepository{db: db}
}

// Transaction support
func (r *dbpolicyRepository) WithTx(tx *gorm.DB) DBPolicyRepository {
    return &dbpolicyRepository{db: tx}
}
```

### Transaction Management

Use BaseRepository for transaction coordination:

```go
// Service initiates transaction
func (s *dbpolicyService) BulkCreate(ctx context.Context, policies []*DBPolicy) error {
    // Begin transaction
    tx := s.baseRepo.Begin()
    if tx == nil {
        return errors.New("failed to begin transaction")
    }

    // Pass tx to all repo calls
    for _, policy := range policies {
        policyRepo := s.policyRepo.WithTx(tx)
        if err := policyRepo.Create(ctx, policy); err != nil {
            return tx.Rollback().Error  // Rollback and return error
        }
    }

    // Commit at end
    return tx.Commit().Error
}
```

### Parameterized Queries - MANDATORY

Never use string interpolation in SQL:

```go
// BAD - SQL Injection vulnerability
query := fmt.Sprintf("SELECT * FROM users WHERE id = %d", userID)
db.Raw(query).Scan(&user)

// GOOD - Parameterized (GORM handles this)
db.Where("id = ?", userID).First(&user)
```

---

## Testing Standards

### Test Naming Convention

Use format: `Test{Function}_{Scenario}_{Expected}`

```go
func TestGetPolicy_ValidID_ReturnsPolicy(t *testing.T)
func TestCreate_DuplicateDatabase_ReturnsError(t *testing.T)
func TestBulkUpdate_EmptyList_SkipsProcessing(t *testing.T)
```

### Unit Tests

Test business logic without external dependencies:

```go
func TestValidatePolicyRequest_MissingName_ReturnsError(t *testing.T) {
    req := &CreatePolicyRequest{
        // Name is missing
        SqlAllow: "SELECT * FROM users",
    }

    err := validatePolicyRequest(req)
    if err == nil {
        t.Fatal("expected validation error, got nil")
    }
    if !strings.Contains(err.Error(), "name required") {
        t.Errorf("unexpected error: %v", err)
    }
}
```

### Integration Tests

Test components with real external systems:

```go
//go:build integration
// +build integration

func TestDBPolicy_Integration_CreateAndQuery(t *testing.T) {
    // Setup test database
    db := setupTestDB(t)
    defer db.Close()

    repo := NewDBPolicyRepository(db)

    // Create policy
    policy := &DBPolicy{Name: "test"}
    if err := repo.Create(context.Background(), policy); err != nil {
        t.Fatalf("create failed: %v", err)
    }

    // Query and verify
    retrieved, err := repo.GetByID(context.Background(), policy.ID)
    if err != nil {
        t.Fatalf("get failed: %v", err)
    }
    if retrieved.Name != "test" {
        t.Errorf("expected name 'test', got %q", retrieved.Name)
    }
}

// Run with: go test -tags=integration ./...
```

### Mocking with Mockery

Use `mockery` to generate mocks (config: `.mockery.yaml`):

```bash
mockery  # Generates mocks/ directory
```

In tests:

```go
func TestBulkUpdate_RepositoryError_Rollback(t *testing.T) {
    // Create mock repository
    mockRepo := &mocks.MockDBPolicyRepository{}
    mockRepo.On("Create", mock.Anything, mock.Anything).
        Return(errors.New("database error"))

    service := NewDBPolicyService(mockRepo)

    err := service.Create(context.Background(), &DBPolicy{})
    if err == nil {
        t.Fatal("expected error")
    }
}
```

### Skip Pattern for Unmockable Code

Document unmockable tests with Skip + comment:

```go
func TestPrivilegeSession_WithRealDatabase_CreatesInMemoryServer(t *testing.T) {
    t.Skip("Requires real MySQL connection. See integration tests.")
    // Document test intent in comments
    // This validates that in-memory server loads privilege data correctly
    // and three-pass execution produces accurate policies.
}
```

### Test Philosophy

- **Quality over coverage** - Skip unit tests when mocking is impractical
- **Comprehensive integration tests** - Test real database + agent interactions
- **Document why** - Comment explains what scenario we're testing
- **No test cheating** - Use real data, actual functions (no fake mocks)

---

## Security Standards

### SQL Injection Prevention

1. **Use parameterized queries** (GORM handles this)
2. **Hex-encode sensitive SQL** - Store templates encoded
3. **Decode only at execution** - Never expose raw template SQL
4. **Validate input bounds** - Actor/object IDs within expected ranges

### Integer Overflow Protection

Use safe converters from `utils/conversion.go`:

```go
// Bad: Direct cast (CWE-190 overflow)
intValue := int(userInput)

// Good: Safe converter with bounds checking
intValue, err := utils.SafeStringToInt(userInput)
if err != nil {
    return fmt.Errorf("invalid input: %w", err)
}
```

### Input Validation - Controller Boundaries

Always validate at service entry points:

```go
func (c *dbpolicyController) Create(ctx *gin.Context) {
    var req CreatePolicyRequest
    if err := ctx.BindJSON(&req); err != nil {
        ctx.JSON(400, gin.H{"error": "invalid request"})
        return
    }

    // Service validates again (defensive)
    if err := c.service.Create(ctx.Request.Context(), &req); err != nil {
        ctx.JSON(400, gin.H{"error": err.Error()})
        return
    }

    ctx.JSON(200, gin.H{"status": "created"})
}
```

### No Hardcoded Secrets

All configuration via environment variables:

```go
// BAD
const dbPassword = "secret123"

// GOOD
dbPassword := os.Getenv("DB_PASS")
if dbPassword == "" {
    log.Fatal("DB_PASS environment variable not set")
}
```

---

## Logging Standards

### Structured Logging Only

Use structured logger (zap-style), never `fmt.Print*`:

```go
// BAD
fmt.Println("User created:", userID)
fmt.Printf("Error: %v", err)

// GOOD
logger.Infof("user created",
    zap.String("user_id", userID),
    zap.String("email", email),
)

logger.Error("failed to create policy",
    zap.String("policy_name", req.Name),
    zap.Error(err),
)
```

### Log Levels

- **ERROR** - Critical failures, needs investigation
- **WARN** - Recoverable issues, degraded state
- **INFO** - Business-significant events (job started, policy created)
- **DEBUG** - Development/troubleshooting only

### Sensitive Data

Never log passwords, API keys, or PII:

```go
// BAD
logger.Infof("connecting to DB", zap.String("password", dbPassword))

// GOOD
logger.Infof("connecting to DB",
    zap.String("host", dbHost),
    zap.String("database", dbName),
)
```

---

## API & Controller Standards

### Endpoint Structure

```
POST   /api/queries/dbpolicy          - Create
GET    /api/queries/dbpolicy/:id      - Read
PUT    /api/queries/dbpolicy/:id      - Update
DELETE /api/queries/dbpolicy/:id      - Delete
POST   /api/queries/dbpolicy/bulk-update - Bulk update
```

### Request/Response Format

All endpoints use JSON:

```go
type CreatePolicyRequest struct {
    Name      string `json:"name" binding:"required"`
    CntID     uint64 `json:"cnt_id" binding:"required"`
    DBMgtID   uint64 `json:"db_mgt_id" binding:"required"`
    SqlAllow  string `json:"sql_allow"`
    SqlDeny   string `json:"sql_deny"`
}

type CreatePolicyResponse struct {
    ID     uint64    `json:"id"`
    Status string    `json:"status"`
    Error  *string   `json:"error,omitempty"`
}
```

**Case Convention:** Match JSON keys to Swagger/database schema case (snake_case for DB fields).

### Error Responses

Standard error format:

```json
{
  "error": "failed to create policy: policy name already exists",
  "code": "POLICY_EXISTS",
  "timestamp": "2026-02-24T10:30:00Z"
}
```

---

## Package Organization

### Package Structure
```
package models      - Domain entities only (GORM models)
package repository  - Data access interfaces + implementations
package services    - Business logic + orchestration
package controllers - HTTP handlers + request/response mapping
package utils       - Shared utilities (validators, converters, loggers)
package config      - Configuration loading
package pkg/logger  - Logging infrastructure
```

### Import Organization

Within each file:

```go
import (
    "context"
    "errors"
    "fmt"

    "gorm.io/gorm"
    "github.com/gin-gonic/gin"

    "dbfartifactapi/models"
    "dbfartifactapi/repository"
    "dbfartifactapi/utils"
)
```

Order: standard library → third-party → internal

---

## File Size Limits

- **Controllers:** < 300 LOC (split large ones into separate endpoints)
- **Services:** < 1000 LOC (split into domain-specific files)
- **Repositories:** < 200 LOC (one entity per repo)
- **Models:** < 100 LOC (one model per file)

---

## Code Review Checklist

Before submitting PR, verify:

- [ ] All functions have WHY comments, not WHAT comments
- [ ] Errors are wrapped with business context (fmt.Errorf)
- [ ] No hardcoded credentials or secrets
- [ ] No fmt.Print* calls (use structured logger)
- [ ] All inputs validated at service boundaries
- [ ] Parameterized queries used (no string interpolation)
- [ ] Transactions use BaseRepository pattern
- [ ] Tests cover error cases and edge conditions
- [ ] Integration tests validate real database behavior
- [ ] Code follows naming conventions
- [ ] No panic() in production code (only init)
- [ ] context.Context passed to all long-running functions
- [ ] Logs are structured (zap-style, not printf)
- [ ] Security review: no SQL injection, integer overflow, PII in logs

---

## Critical Rules - NEVER VIOLATE

These rules are non-negotiable:

1. **NEVER ignore errors** - Always handle or propagate with context
2. **NEVER use panic()** in production code (only during init/tests)
3. **NEVER hardcode credentials** or secrets
4. **NEVER use fmt.Print*** for logging - use structured logger
5. **NEVER create goroutines without cleanup** - Always have shutdown strategy
6. **NEVER skip input validation** at service boundaries
7. **NEVER use raw SQL strings** - Use parameterized queries
8. **NEVER log sensitive data** - No passwords, API keys, PII

Violations will be rejected during code review.

---

## Common Mistakes to Avoid

| Mistake | Problem | Fix |
|---------|---------|-----|
| Ignoring errors | Silently fail, hard to debug | Wrap and return with context |
| Comments explain what | Noise, maintainers read code | Comment WHY and non-obvious |
| Errors without context | "failed" tells nothing | Include what operation, what object |
| Global variables | Hard to test, hidden dependencies | Use dependency injection |
| Unbounded goroutines | Memory leaks, shutdown hangs | Use WaitGroup, channels for cleanup |
| Direct DB calls in controllers | No reusability, hard to test | Always use repository |
| fmt.Println for logging | Loss of structure, hard to parse | Use structured logger |
| Panics in production | Unhandled crashes | Return errors instead |

---

## Continuous Improvement

### Code Review Process
1. Self-review: Check checklist above
2. Peer review: At least 1 approval before merge
3. Automated: Tests must pass, no linting errors
4. Security: Verify no SQL injection, CWE-190, hardcoded secrets

### Standards Evolution
- Review quarterly for improvement opportunities
- Document decisions in CLAUDE.md
- Team agreement before changing standards
- Backward compatibility for existing code

---

**Last Updated:** 2026-02-24
**Reviewed by:** DBF Architecture Team
**Next Review:** 2026-05-24
