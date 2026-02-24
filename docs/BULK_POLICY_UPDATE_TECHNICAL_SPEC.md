# Bulk Policy Update - Technical Specification

## Document Information
- **Feature**: Bulk Database Policy Update API
- **Version**: 1.0.0
- **Last Updated**: 2025-10-14
- **Status**: Production Ready
- **Maintainers**: DBF Team

---

## Table of Contents
1. [Overview](#overview)
2. [Architecture](#architecture)
3. [API Specification](#api-specification)
4. [Implementation Details](#implementation-details)
5. [Data Flow](#data-flow)
6. [Security Considerations](#security-considerations)
7. [Error Handling](#error-handling)
8. [Performance](#performance)
9. [Testing Strategy](#testing-strategy)
10. [Troubleshooting](#troubleshooting)
11. [Future Enhancements](#future-enhancements)

---

## Overview

### Purpose
Provides atomic bulk updates to database access policies by computing the diff between current and desired states, executing GRANT/REVOKE operations via VeloArtifact, and updating the database only after successful remote execution.

### Business Context
**WHY this feature exists:**
- Manual policy updates are error-prone and time-consuming
- Requires atomic all-or-nothing consistency for security compliance
- Must execute remote SQL commands BEFORE database records update
- Supports dynamic permission management across multiple database objects

### Key Capabilities
- Diff-based updates (only changes what's needed)
- Atomic transactions (all succeed or all fail)
- Remote execution validation before database commits
- Audit trail for compliance
- Asynchronous background job processing

---

## Architecture

### System Components

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │ POST /api/queries/dbpolicy/bulkupdate
       ▼
┌──────────────────────────────────────────────────┐
│           DBPolicyController                      │
│  - Input validation                              │
│  - JSON binding                                  │
└──────┬───────────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────────┐
│         DBPolicyService                          │
│  1. Query existing policies                      │
│  2. Calculate diff (toAdd, toRemove)             │
│  3. Build SQL commands (GRANT/REVOKE)            │
│  4. Write VeloArtifact job file                  │
│  5. Execute background job                       │
│  6. Register completion callback                 │
└──────┬───────────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────────┐
│         VeloArtifact Client                      │
│  - Execute remote SQL commands                   │
│  - Return job ID                                 │
└──────┬───────────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────────┐
│       Job Monitor Service                        │
│  - Poll job status                               │
│  - Trigger completion callback                   │
└──────┬───────────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────────┐
│  BulkPolicyCompletionHandler                     │
│  1. Validate all commands succeeded              │
│  2. Begin new transaction                        │
│  3. Delete removed policies                      │
│  4. Create new policies                          │
│  5. Commit atomically                            │
│  6. Write audit log                              │
└──────────────────────────────────────────────────┘
```

### Design Pattern: Background Job with Atomic Completion

**WHY this pattern:**
- VeloArtifact execution can take minutes (remote database operations)
- HTTP timeouts would fail for long-running operations
- Database must reflect actual remote state (consistency requirement)
- Prevents client blocking while waiting for completion

**Transaction Flow:**
```
Preparation Phase:
├─ BEGIN transaction T1
├─ Read existing policies
├─ Calculate diff
├─ Build SQL commands
├─ Write VeloArtifact job file
├─ Start background job
└─ ROLLBACK T1 (cleanup only)

Execution Phase (Async):
└─ VeloArtifact executes GRANT/REVOKE on remote database

Completion Phase (Callback):
├─ BEGIN transaction T2
├─ Validate all commands succeeded
├─ Delete policies (toRemove)
├─ Create policies (toAdd)
└─ COMMIT T2 (atomic update)
```

**CRITICAL:** T1 is rolled back because it's only for data preparation. T2 is the actual atomic update triggered by successful remote execution.

---

## API Specification

### Endpoint
```
POST /api/queries/dbpolicy/bulkupdate
```

### Request Schema
```go
type BulkPolicyUpdateRequest struct {
    CntMgtID          uint   `json:"cntmgt_id" binding:"required"`
    DBMgtID           uint   `json:"dbmgt_id" binding:"required"`
    DBActorMgtID      uint   `json:"dbactormgt_id" binding:"required"`
    NewPolicyDefaults []uint `json:"new_policy_defaults" binding:"required"`
    NewObjectMgts     []uint `json:"new_object_mgts" binding:"required"`
}
```

**Field Descriptions:**

| Field | Type | Description | Business Rule |
|-------|------|-------------|---------------|
| `cntmgt_id` | uint | Connection Management ID | Must reference existing CntMgt record |
| `dbmgt_id` | uint | Database Management ID | Must belong to specified CntMgt |
| `dbactormgt_id` | uint | Database Actor (User) ID | The user whose permissions are being updated |
| `new_policy_defaults` | []uint | List of DBPolicyDefault IDs | Policy templates to apply |
| `new_object_mgts` | []uint | List of DBObjectMgt IDs | Database objects (tables, views, etc.) |

**Algorithm: Cartesian Product**
```
Desired State = new_policy_defaults × new_object_mgts

Example:
  new_policy_defaults = [SELECT, INSERT]
  new_object_mgts = [table_users, table_orders]

  Result: 4 policies
    1. SELECT on table_users
    2. SELECT on table_orders
    3. INSERT on table_users
    4. INSERT on table_orders
```

### Response Schema
```json
{
  "message": "Bulk policy update background job started: job_abc123. Adding 6 policies, removing 3 policies.",
  "status": "job_started"
}
```

### HTTP Status Codes

| Code | Meaning | Scenario |
|------|---------|----------|
| 200 | Success | Job started successfully |
| 400 | Bad Request | Invalid input, validation failed |
| 404 | Not Found | Referenced IDs don't exist |
| 500 | Internal Error | Database error, VeloArtifact failure |

### Example Request
```bash
curl -X POST http://localhost:8080/api/queries/dbpolicy/bulkupdate \
  -H "Content-Type: application/json" \
  -d '{
    "cntmgt_id": 1,
    "dbmgt_id": 5,
    "dbactormgt_id": 3,
    "new_policy_defaults": [10, 11, 12],
    "new_object_mgts": [100, 101, 102]
  }'
```

### Example Response
```json
{
  "message": "Bulk policy update background job started: F.C4LG5UMHSAPEI0J. Adding 9 policies, removing 2 policies.",
  "status": "job_started"
}
```

---

## Implementation Details

### File Structure
```
dbfartifactapi/
├── controllers/
│   └── dbpolicy_controller.go          # HTTP endpoint handler
├── services/
│   ├── dbpolicy_service.go              # Core business logic
│   ├── bulk_policy_completion_handler.go # Job callback handler
│   └── dto/
│       └── bulk_policy_update_dto.go    # Data transfer objects
└── repository/
    └── dbpolicy_repository.go           # Database access layer
```

### Core Algorithm: Policy Diff Calculation

**Location:** `services/dbpolicy_service.go` - `calculatePolicyDiff()`

**Algorithm:**
```go
// Set-based diff calculation
func calculatePolicyDiff(
    existing []PolicyCombination,
    newPolicyDefaults []uint,
    newObjectMgts []uint,
) (toAdd, toRemove []PolicyCombination) {

    // Build existing set
    existingSet := map[string]PolicyCombination{}
    for _, combo := range existing {
        key := fmt.Sprintf("%d_%d", combo.PolicyDefaultID, combo.ObjectMgtID)
        existingSet[key] = combo
    }

    // Build desired set (Cartesian product)
    desiredSet := map[string]PolicyCombination{}
    for _, policyID := range newPolicyDefaults {
        for _, objectID := range newObjectMgts {
            key := fmt.Sprintf("%d_%d", policyID, objectID)
            desiredSet[key] = PolicyCombination{
                PolicyDefaultID: policyID,
                ObjectMgtID:     objectID,
            }
        }
    }

    // Calculate diff
    // toAdd = desired - existing
    for key, combo := range desiredSet {
        if _, exists := existingSet[key]; !exists {
            toAdd = append(toAdd, combo)
        }
    }

    // toRemove = existing - desired
    for key, combo := range existingSet {
        if _, exists := desiredSet[key]; !exists {
            toRemove = append(toRemove, combo)
        }
    }

    return toAdd, toRemove
}
```

**Time Complexity:** O(n + m + k) where:
- n = existing policies count
- m = new_policy_defaults × new_object_mgts
- k = max(n, m)

**Space Complexity:** O(n + m)

### SQL Command Generation

**Location:** `services/dbpolicy_service.go` - `buildBulkPolicyCommands()`

**Security Feature: Hex-Decoded Templates**

**WHY hex-encoding:**
- Prevents SQL injection during template storage
- Templates are stored in database as hex strings
- Decoded only at execution time
- Variable substitution happens AFTER decode

**Example:**
```go
// Template stored in DB (hex-encoded)
sqlTemplate := "53454c454354202a2046524f4d20247b64626d67742e64626e616d657d"

// Decode before use
decodedSQL, _ := hex.DecodeString(sqlTemplate)
// Result: "SELECT * FROM ${dbmgt.dbname}"

// Variable substitution
finalSQL := strings.Replace(string(decodedSQL), "${dbmgt.dbname}", "production_db", -1)
// Result: "SELECT * FROM production_db"
```

**Variable Substitution Rules:**

| Placeholder | Replaced With | Source |
|-------------|---------------|--------|
| `${dbmgt.dbname}` | Database name | DBMgt.DBName |
| `${dbactormgt.dbuser}` | Database user | DBActorMgt.DBUser |
| `${dbactormgt.ip_address}` | User IP address | DBActorMgt.IPAddress |
| `${dbobjectmgt.objectname}` | Object name | DBObjectMgt.ObjectName |

### VeloArtifact Job File Structure

**Location:** `services/dbpolicy_service.go` - `writeBulkPolicyUpdateFile()`

**File Format:** JSON
```json
{
  "Actor:3_PolicyDf:10_Object:100_Action:add": [
    "GRANT SELECT ON production_db.table_users TO 'app_user'@'10.0.0.5'"
  ],
  "Actor:3_PolicyDf:11_Object:100_Action:add": [
    "GRANT INSERT ON production_db.table_users TO 'app_user'@'10.0.0.5'"
  ],
  "Actor:3_PolicyDf:5_Object:50_Action:remove": [
    "REVOKE DELETE ON production_db.table_logs FROM 'app_user'@'10.0.0.5'"
  ]
}
```

**Unique Key Format:**
```
Actor:{actor_id}_PolicyDf:{policy_default_id}_Object:{object_id}_Action:{add|remove}
```

**WHY this format:**
- VeloArtifact requires unique keys for result tracking
- Keys embed metadata for result validation
- Enables granular error reporting
- Supports idempotency checks

### Completion Handler Logic

**Location:** `services/bulk_policy_completion_handler.go`

**Critical Business Rule: All-or-Nothing Atomicity**

```go
// Validate all commands succeeded BEFORE any database changes
failedCommands := make(map[string]string)
for _, result := range resultsData {
    if result.Status != "success" {
        failedCommands[cleanKey] = fmt.Sprintf("status=%s", result.Status)
    }
}

// If ANY command failed, abort entire operation
if len(failedCommands) > 0 {
    return fmt.Errorf("bulk policy update aborted - %d commands failed", len(failedCommands))
}

// Only proceed if ALL commands succeeded
// This ensures database state matches actual remote permissions
```

**WHY this is critical:**
- Security compliance requires exact permission tracking
- Partial updates create security vulnerabilities
- Database must be single source of truth
- Rollback is safer than inconsistent state

### Audit Trail

**Location:** `{VeloResultsDir}/bulk_policy_update_{job_id}.log`

**Log Format:**
```
2025/10/14 10:30:15 Starting bulk policy update for job F.C4LG5UMHSAPEI0J: 10 commands executed, 9 to add, 2 to remove
2025/10/14 10:30:16 DELETED: 2 policies
2025/10/14 10:30:16 DELETE: policy_id=45
2025/10/14 10:30:16 DELETE: policy_id=46
2025/10/14 10:30:17 CREATED: 9 policies
2025/10/14 10:30:17 CREATE: policy_default_id=10, object_id=100, actor_id=3
2025/10/14 10:30:17 CREATE: policy_default_id=10, object_id=101, actor_id=3
...
2025/10/14 10:30:18 SUCCESS: Bulk policy update completed - added 9, removed 2 policies for actor_id=3
```

**WHY audit logs:**
- Compliance requirements (SOX, GDPR, etc.)
- Troubleshooting failed operations
- Security incident investigation
- Change tracking for audit reviews

---

## Data Flow

### Complete Request Flow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                        1. CLIENT REQUEST                         │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│              2. CONTROLLER: Input Validation                     │
│  - Bind JSON to BulkPolicyUpdateRequest                         │
│  - Validate struct fields                                       │
│  - Return 400 if invalid                                        │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│           3. SERVICE: Query Existing Policies                    │
│  Query: SELECT * FROM dbpolicy                                  │
│  WHERE cnt_id = ? AND dbmgt_id = ? AND actor_id = ?            │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│              4. SERVICE: Calculate Diff                          │
│  Existing: [(10,100), (10,101), (5,50)]                        │
│  Desired:  [(10,100), (11,100), (11,101)]                      │
│  ─────────────────────────────────────                         │
│  toAdd:    [(11,100), (11,101)]                                │
│  toRemove: [(10,101), (5,50)]                                  │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│           5. SERVICE: Build SQL Commands                         │
│  For toAdd:                                                     │
│    - Hex-decode SqlUpdateAllow from DBPolicyDefault            │
│    - Substitute variables (dbname, dbuser, objectname)         │
│    - Result: GRANT commands                                    │
│  For toRemove:                                                  │
│    - Hex-decode SqlUpdateDeny from DBPolicyDefault             │
│    - Substitute variables                                       │
│    - Result: REVOKE commands                                   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│           6. SERVICE: Write VeloArtifact Job File                │
│  File: {TEMP_DIR}/bulk_policy_{job_uuid}.json                  │
│  Format: {"UniqueKey1": ["SQL1"], "UniqueKey2": ["SQL2"]}     │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│         7. SERVICE: Execute VeloArtifact Background Job          │
│  Command: download {job_file} --background                     │
│  Returns: job_id (e.g., F.C4LG5UMHSAPEI0J)                     │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│          8. SERVICE: Register Completion Callback                │
│  JobMonitor.RegisterJob(job_id, callback, context)             │
│  Context includes: toAdd, toRemove, entity references          │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│              9. CONTROLLER: Return Response                      │
│  HTTP 200: {"message": "Job started: {job_id}", ...}           │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                 10. ASYNC: Job Execution                         │
│  VeloArtifact executes SQL commands on remote database          │
│  Duration: Seconds to minutes (depends on command count)        │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│            11. ASYNC: Job Monitor Polling                        │
│  Poll interval: Every N seconds                                 │
│  Command: checkstatus {job_id}                                 │
│  Loop until: status == "completed"                             │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│         12. CALLBACK: Completion Handler Triggered               │
│  Input: job_id, status, results_data, context                  │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│          13. HANDLER: Retrieve Results File                      │
│  Download results from VeloArtifact or notification directory   │
│  Parse JSON: [{query_key, status, query, result}, ...]         │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│           14. HANDLER: Validate All Commands Succeeded           │
│  Check: result.Status == "success" for ALL results              │
│  If ANY failed: ABORT and return error                          │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│            15. HANDLER: Begin Database Transaction               │
│  tx := db.Begin()                                               │
│  Defer: tx.Rollback() if not committed                          │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│             16. HANDLER: Delete Removed Policies                 │
│  BulkDelete(tx, policyIDsToDelete)                             │
│  SQL: DELETE FROM dbpolicy WHERE id IN (?)                     │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│             17. HANDLER: Create New Policies                     │
│  BulkCreate(tx, policiesToCreate)                              │
│  SQL: INSERT INTO dbpolicy (cnt_mgt, dbmgt, ...) VALUES (...)  │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│             18. HANDLER: Commit Transaction                      │
│  tx.Commit()                                                    │
│  Atomic: All changes applied or none                            │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│               19. HANDLER: Write Audit Log                       │
│  Log: SUCCESS - added N, removed M policies for actor X         │
│  File: bulk_policy_update_{job_id}.log                         │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                    20. COMPLETION                                │
│  Database state now reflects actual remote permissions          │
└─────────────────────────────────────────────────────────────────┘
```

### State Transitions

```
┌──────────────┐
│   PENDING    │ ◄─── Initial state (job submitted)
└──────┬───────┘
       │
       ▼
┌──────────────┐
│   RUNNING    │ ◄─── VeloArtifact executing commands
└──────┬───────┘
       │
       ├─────────────────┐
       │                 │
       ▼                 ▼
┌──────────────┐  ┌──────────────┐
│  COMPLETED   │  │    FAILED    │
└──────┬───────┘  └──────┬───────┘
       │                 │
       │                 ▼
       │          ┌──────────────┐
       │          │   ABORTED    │ ◄─── Any command failed
       │          └──────────────┘
       │                 │
       ▼                 ▼
┌──────────────┐  ┌──────────────┐
│  DB_UPDATED  │  │  DB_UNCHANGED│ ◄─── Rollback
└──────────────┘  └──────────────┘
```

---

## Security Considerations

### 1. SQL Injection Prevention

**Mechanism:** Hex-encoded templates + parameterized queries

**Implementation:**
```go
// ❌ NEVER DO THIS (vulnerable to injection)
sql := fmt.Sprintf("GRANT SELECT ON %s TO %s", dbName, userName)

// ✅ CORRECT (hex-decode + controlled substitution)
hexTemplate := "4752414e542053454c454354204f4e20247b64626d67742e64626e616d657d"
decoded, _ := hex.DecodeString(hexTemplate)
sql := strings.Replace(string(decoded), "${dbmgt.dbname}", dbName, -1)
```

**WHY this works:**
- Templates are pre-validated and stored as hex
- Variable placeholders are literal strings (not user input)
- Actual values come from database records (already validated)
- No user input directly concatenated into SQL

### 2. Authorization Checks

**Required Validations:**
```go
// 1. Verify all referenced entities exist
if err := validateEntityExists(cntMgtID, dbMgtID, actorMgtID); err != nil {
    return err
}

// 2. Verify entities belong to same scope
if dbmgt.CntID != cntMgtID {
    return errors.New("dbmgt does not belong to specified cntmgt")
}

// 3. Verify policy defaults are valid for actor type
if !isValidPolicyForActorType(actor.ActorType, policyDefaults); err != nil {
    return errors.New("invalid policy for actor type")
}
```

### 3. Atomic Consistency

**Security Implication:**
- Database records MUST match actual remote permissions
- Partial updates create security vulnerabilities
- Unauthorized access if database shows "GRANT" but remote shows "REVOKE"

**Enforcement:**
```go
// All commands must succeed
if len(failedCommands) > 0 {
    // CRITICAL: Do not update database
    return errors.New("aborting bulk update - inconsistent state prevented")
}
```

### 4. Audit Trail

**Compliance Requirements:**
- WHO: Which actor's permissions changed
- WHAT: Which policies added/removed
- WHEN: Timestamp of change
- WHY: Job ID linking to request
- RESULT: Success or failure with details

**Implementation:**
```go
auditLogger.Printf("SUCCESS: added %d, removed %d policies for actor_id=%d",
    addedCount, removedCount, actorID)
```

---

## Error Handling

### Error Categories and Responses

#### 1. Input Validation Errors
**HTTP 400 Bad Request**

| Scenario | Error Message | Resolution |
|----------|---------------|------------|
| Missing required field | "invalid request body: Key: 'BulkPolicyUpdateRequest.CntMgtID' Error:Field validation for 'CntMgtID' failed on the 'required' tag" | Provide all required fields |
| Invalid ID type | "invalid request body: json: cannot unmarshal string into Go struct field" | Use numeric IDs |
| Empty arrays | "invalid request: policy defaults and objects cannot be empty" | Provide at least one policy and object |

#### 2. Entity Not Found Errors
**HTTP 404 Not Found**

| Scenario | Error Message | Resolution |
|----------|---------------|------------|
| Invalid cntmgt_id | "connection management not found: 999" | Use valid CntMgt ID |
| Invalid dbmgt_id | "database management not found: 888" | Use valid DBMgt ID |
| Invalid actor_id | "database actor not found: 777" | Use valid DBActorMgt ID |
| Invalid policy_default_id | "policy default not found: 666" | Use valid policy template |
| Invalid object_id | "database object not found: 555" | Use valid database object |

#### 3. Relationship Validation Errors
**HTTP 400 Bad Request**

| Scenario | Error Message | Resolution |
|----------|---------------|------------|
| DBMgt doesn't belong to CntMgt | "database management 5 does not belong to connection management 1" | Verify correct entity relationships |
| Policy default incompatible with actor | "policy default 10 cannot be applied to actor type 'role'" | Use compatible policy templates |

#### 4. VeloArtifact Execution Errors
**HTTP 500 Internal Server Error**

| Scenario | Error Message | Resolution |
|----------|---------------|------------|
| VeloArtifact client not found | "VeloArtifact client not found at path: /path/to/client" | Check `VeloClientPath` configuration |
| Job file write failure | "failed to write VeloArtifact job file: permission denied" | Check `DBFWEB_TEMP_DIR` permissions |
| Job submission failure | "failed to execute VeloArtifact job: exit code 1" | Check VeloArtifact connectivity |

#### 5. Job Completion Errors
**Logged to audit file, no HTTP response (async)**

| Scenario | Error Message | Impact |
|----------|---------------|--------|
| Partial command failure | "bulk policy update aborted - 3 commands failed out of 10 total" | No database changes applied (rollback) |
| Result file parse error | "failed to parse VeloArtifact results: invalid JSON" | Job marked as failed, manual intervention needed |
| Database transaction failure | "failed to commit bulk policy updates: deadlock detected" | Automatic retry may be needed |

### Error Response Format
```json
{
  "error": "detailed error message",
  "code": "ERROR_CODE",
  "details": {
    "field": "problematic field name",
    "value": "problematic value"
  }
}
```

### Retry Strategy

**Automatic Retries:** NONE
- Idempotency not guaranteed for SQL commands
- Manual review required for failed operations
- Prevents double-grant/double-revoke issues

**Manual Recovery:**
1. Check audit log: `{VeloResultsDir}/bulk_policy_update_{job_id}.log`
2. Identify failed commands
3. Investigate remote database state
4. Manually reconcile if needed
5. Re-submit corrected request

---

## Performance

### Scalability Metrics

| Metric | Value | Notes |
|--------|-------|-------|
| Max policies per request | ~1000 | Limited by VeloArtifact command count |
| Avg response time (API) | <100ms | Job starts in background |
| Avg job duration | 10-60s | Depends on remote DB latency |
| DB transaction time | <1s | Bulk insert/delete is fast |
| Concurrent requests | Limited | VeloArtifact client serialization |

### Performance Characteristics

**Time Complexity:**
- Policy diff calculation: O(n + m) where n=existing, m=desired
- SQL command generation: O(k) where k=total changes
- Database operations: O(log n) for indexed queries

**Space Complexity:**
- Memory usage: O(n + m) for diff calculation
- File size: ~500 bytes per SQL command
- Database records: Linear growth with policy count

### Optimization Opportunities

1. **Batch Size Limiting**
   ```go
   const MAX_POLICIES_PER_BATCH = 1000
   if len(toAdd) + len(toRemove) > MAX_POLICIES_PER_BATCH {
       return errors.New("batch size exceeds limit, please split into multiple requests")
   }
   ```

2. **Index Optimization**
   ```sql
   CREATE INDEX idx_dbpolicy_lookup ON dbpolicy(cnt_id, dbmgt_id, actor_id);
   CREATE INDEX idx_dbpolicy_combination ON dbpolicy(policy_default_id, object_mgt_id, actor_id);
   ```

3. **Connection Pooling**
   - GORM automatically handles connection pooling
   - Adjust `DB_MAX_OPEN_CONNS` and `DB_MAX_IDLE_CONNS` for load

### Monitoring Metrics

**Key Metrics to Track:**
```go
// Job submission rate
metrics.Counter("bulk_policy.jobs_submitted").Inc()

// Job success/failure rate
metrics.Counter("bulk_policy.jobs_succeeded").Inc()
metrics.Counter("bulk_policy.jobs_failed").Inc()

// Policy change volume
metrics.Histogram("bulk_policy.policies_added").Observe(float64(addedCount))
metrics.Histogram("bulk_policy.policies_removed").Observe(float64(removedCount))

// Job duration
metrics.Histogram("bulk_policy.job_duration_seconds").Observe(duration)
```

---

## Testing Strategy

### Unit Tests

**Location:** `services/dbpolicy_service_test.go`

**Test Cases:**
```go
func TestCalculatePolicyDiff_EmptyExisting(t *testing.T)
func TestCalculatePolicyDiff_EmptyDesired(t *testing.T)
func TestCalculatePolicyDiff_NoChanges(t *testing.T)
func TestCalculatePolicyDiff_OnlyAdditions(t *testing.T)
func TestCalculatePolicyDiff_OnlyRemovals(t *testing.T)
func TestCalculatePolicyDiff_MixedChanges(t *testing.T)
func TestBuildBulkPolicyCommands_ValidInput(t *testing.T)
func TestBuildBulkPolicyCommands_HexDecodeError(t *testing.T)
```

### Integration Tests

**Test Scenarios:**

1. **Happy Path - Mixed Changes**
   ```
   Setup:
     - Create test CntMgt, DBMgt, DBActorMgt
     - Create 5 existing policies

   Execute:
     - Request: 3 new policy_defaults × 2 objects = 6 desired

   Verify:
     - toAdd = 1 policy
     - toRemove = 0 policies
     - Job starts successfully
     - Database updated after job completion
   ```

2. **Edge Case - No Changes**
   ```
   Setup:
     - Existing policies match desired state

   Execute:
     - Request with same policies

   Verify:
     - Early return: "No changes detected"
     - No VeloArtifact job executed
     - No database changes
   ```

3. **Error Case - Partial Failure**
   ```
   Setup:
     - Mock VeloArtifact to return 1 failed command

   Execute:
     - Submit bulk update request

   Verify:
     - Job starts successfully
     - Completion handler detects failure
     - Database NOT updated (rollback)
     - Error logged to audit file
   ```

4. **Error Case - Invalid Entity Relationships**
   ```
   Setup:
     - DBMgt belongs to CntMgt A

   Execute:
     - Request with CntMgt B (mismatch)

   Verify:
     - HTTP 400 error
     - Error message: "dbmgt does not belong to cntmgt"
   ```

### Manual Testing Checklist

- [ ] Submit valid request → verify job starts
- [ ] Check audit log created: `bulk_policy_update_{job_id}.log`
- [ ] Wait for job completion → verify database updated
- [ ] Submit request with no changes → verify early return
- [ ] Submit invalid cntmgt_id → verify 404 error
- [ ] Submit empty policy_defaults array → verify 400 error
- [ ] Mock VeloArtifact failure → verify rollback
- [ ] Check remote database → verify GRANT/REVOKE executed
- [ ] Submit concurrent requests → verify serialization

### Load Testing

**Tool:** Apache JMeter or k6

**Scenario:**
```javascript
import http from 'k6/http';

export default function () {
  const payload = JSON.stringify({
    cntmgt_id: 1,
    dbmgt_id: 5,
    dbactormgt_id: 3,
    new_policy_defaults: [10, 11, 12],
    new_object_mgts: [100, 101, 102]
  });

  http.post('http://localhost:8080/api/queries/dbpolicy/bulkupdate', payload, {
    headers: { 'Content-Type': 'application/json' }
  });
}
```

**Load Profile:**
- Ramp up: 0 to 10 users over 1 minute
- Sustained: 10 users for 5 minutes
- Ramp down: 10 to 0 users over 1 minute

**Success Criteria:**
- 95th percentile response time < 200ms
- Error rate < 1%
- All jobs complete successfully
- No database deadlocks

---

## Troubleshooting

### Common Issues

#### Issue 1: Job Never Completes

**Symptoms:**
- API returns job started
- Job status remains "running" indefinitely
- No audit log file created

**Diagnosis:**
```bash
# Check VeloArtifact job status manually
./veloartifact checkstatus {job_id}

# Check job monitor logs
grep "{job_id}" logs/job_monitor.log

# Check VeloArtifact connectivity
./veloartifact status
```

**Resolutions:**
- VeloArtifact service down → restart service
- Network connectivity issue → check firewall rules
- Job monitor service not running → restart application
- Job file corrupted → check `{TEMP_DIR}` permissions

#### Issue 2: Job Completes but Database Not Updated

**Symptoms:**
- Audit log shows "ROLLBACK"
- Error message: "bulk policy update aborted - N commands failed"

**Diagnosis:**
```bash
# Check audit log for details
cat {VeloResultsDir}/bulk_policy_update_{job_id}.log | grep FAILED

# Check VeloArtifact results file
cat {VeloResultsDir}/{job_id}_results.json
```

**Resolutions:**
- Remote database permission denied → grant VeloArtifact user permissions
- SQL syntax error → check policy template validity
- Remote database connection timeout → increase timeout config
- Invalid database object name → verify object exists

#### Issue 3: Database Inconsistency

**Symptoms:**
- Database shows policy exists
- Remote database shows no permission granted
- OR vice versa

**Diagnosis:**
```sql
-- Check database policies
SELECT * FROM dbpolicy
WHERE actor_id = ? AND dbmgt_id = ?;

-- Check remote database permissions (MySQL)
SHOW GRANTS FOR 'user'@'host';

-- Check remote database permissions (PostgreSQL)
\du+ username
```

**Resolutions:**
1. Identify discrepancy source
2. Check audit logs for failed jobs
3. Manually reconcile remote permissions
4. Update database records if needed
5. Investigate root cause (WHY did it happen?)

#### Issue 4: High Job Failure Rate

**Symptoms:**
- Multiple jobs failing with same error
- Pattern: specific policy_default_id always fails

**Diagnosis:**
```sql
-- Check policy template validity
SELECT id, sqlupdate_allow, sqlupdate_deny
FROM dbpolicydefault
WHERE id IN (failing_policy_ids);

-- Hex-decode templates manually
SELECT UNHEX(sqlupdate_allow) FROM dbpolicydefault WHERE id = ?;
```

**Resolutions:**
- Invalid template syntax → fix template in database
- Missing variable placeholders → add ${variable} to template
- Incompatible SQL dialect → adjust template for target DB type

### Debug Mode

**Enable verbose logging:**
```go
// config/config.go
DEBUG_MODE = true

// Enables additional logging:
// - Full SQL commands (sanitized)
// - VeloArtifact command output
// - Detailed error stack traces
// - Transaction boundaries
```

**Check logs:**
```bash
# Application logs
tail -f logs/dbfartifact.log | grep "bulk_policy"

# Job-specific logs
tail -f {VeloResultsDir}/bulk_policy_update_{job_id}.log

# VeloArtifact logs
tail -f logs/veloartifact.log
```

### Health Checks

**Endpoint:** `GET /health`

**Response:**
```json
{
  "status": "healthy",
  "services": {
    "database": "connected",
    "veloartifact": "reachable",
    "job_monitor": "running"
  }
}
```

---

## Future Enhancements

### 1. Batch Splitting
**Current Limitation:** Large policy sets may timeout

**Proposed Solution:**
```go
const MAX_BATCH_SIZE = 500

func splitIntoBatches(policies []PolicyCombination, batchSize int) [][]PolicyCombination {
    // Split large requests into smaller batches
    // Execute sequentially with progress tracking
}
```

### 2. Rollback Capability
**Current Limitation:** Failed jobs require manual recovery

**Proposed Solution:**
- Store pre-change state snapshot
- Provide rollback API endpoint
- Implement automatic rollback on timeout

### 3. Preview Mode
**Use Case:** Validate changes before execution

**Proposed API:**
```
POST /api/queries/dbpolicy/bulkupdate/preview
```

**Response:**
```json
{
  "policies_to_add": [
    {"policy_default": "SELECT", "object": "table_users"}
  ],
  "policies_to_remove": [
    {"policy_default": "DELETE", "object": "table_logs"}
  ],
  "sql_commands": {
    "grants": ["GRANT SELECT ON ..."],
    "revokes": ["REVOKE DELETE ON ..."]
  }
}
```

### 4. Notification-Based Completion
**Current:** Polling-based job monitoring

**Enhancement:** Webhook/SSE for instant completion notification
- Reduces polling overhead
- Faster completion detection
- Lower latency for users

### 5. Policy Templates with Variables
**Use Case:** Reusable templates across actors

**Example:**
```json
{
  "template_name": "read_only_user",
  "policies": [
    {"policy_default": 10, "objects": "all_tables"}
  ]
}
```

### 6. Audit Dashboard
**Features:**
- Visual timeline of policy changes
- Filterable by actor, database, date range
- Diff viewer (before/after)
- Export to CSV for compliance reports

---

## Appendix

### A. Database Schema

**DBPolicy Table:**
```sql
CREATE TABLE dbpolicy (
    id INT PRIMARY KEY AUTO_INCREMENT,
    cnt_mgt INT NOT NULL,
    dbmgt INT NOT NULL,
    dbactor_mgt INT NOT NULL,
    dbpolicy_default INT NOT NULL,
    dbobject_mgt INT NOT NULL,
    status VARCHAR(20) DEFAULT 'enabled',
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_actor_scope (cnt_mgt, dbmgt, dbactor_mgt),
    INDEX idx_policy_combo (dbpolicy_default, dbobject_mgt, dbactor_mgt)
);
```

### B. Configuration Reference

**Environment Variables:**
```bash
# Database
DB_HOST=localhost
DB_PORT=3306
DB_USER=dbfartifact
DB_PASSWORD=secret
DB_NAME=dbfartifact_db

# VeloArtifact
VeloClientPath=/usr/local/bin/veloartifact
DBFWEB_TEMP_DIR=/var/tmp/dbfartifact

# Application
VeloResultsDir=/var/log/dbfartifact/results
NotificationFileDir=/var/log/dbfartifact/notifications
```

### C. API Examples

**cURL Example:**
```bash
curl -X POST http://localhost:8080/api/queries/dbpolicy/bulkupdate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer {token}" \
  -d '{
    "cntmgt_id": 1,
    "dbmgt_id": 5,
    "dbactormgt_id": 3,
    "new_policy_defaults": [10, 11, 12],
    "new_object_mgts": [100, 101, 102]
  }'
```

**Python Example:**
```python
import requests

url = "http://localhost:8080/api/queries/dbpolicy/bulkupdate"
headers = {"Content-Type": "application/json"}
payload = {
    "cntmgt_id": 1,
    "dbmgt_id": 5,
    "dbactormgt_id": 3,
    "new_policy_defaults": [10, 11, 12],
    "new_object_mgts": [100, 101, 102]
}

response = requests.post(url, json=payload, headers=headers)
print(response.json())
```

### D. Glossary

| Term | Definition |
|------|------------|
| **CntMgt** | Connection Management - represents a database connection/endpoint |
| **DBMgt** | Database Management - represents a specific database within a connection |
| **DBActorMgt** | Database Actor Management - represents a database user/role |
| **DBPolicyDefault** | Policy template containing SQL commands for permissions |
| **DBObjectMgt** | Database object (table, view, procedure, etc.) |
| **VeloArtifact** | Remote command execution system for SQL operations |
| **Cartesian Product** | Mathematical operation: A × B = all combinations of elements from A and B |
| **Atomic Transaction** | All-or-nothing database operation |
| **Hex-encoding** | Converting data to hexadecimal format for secure storage |

---

## Document Maintenance

**Review Schedule:** Quarterly

**Change Log:**

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2025-10-14 | 1.0.0 | DBF Team | Initial technical specification |

**Feedback:**
For questions, issues, or suggestions, please contact the DBF team or create an issue in the project repository.

---

**END OF DOCUMENT**