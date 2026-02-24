# Bulk Policy Update - Quick Start Guide

## 🚀 Quick Overview

**What it does:** Atomically updates database access policies by computing the diff between current and desired states.

**Key Features:**
- ✅ Diff-based updates (only changes what's needed)
- ✅ Atomic transactions (all succeed or all fail)
- ✅ Remote execution validation before database commits
- ✅ Asynchronous background processing
- ✅ Full audit trail

---

## 📋 Prerequisites

1. Valid CntMgt (Connection Management) ID
2. Valid DBMgt (Database Management) ID
3. Valid DBActorMgt (Database User) ID
4. List of DBPolicyDefault IDs (policy templates)
5. List of DBObjectMgt IDs (database objects)

---

## 🔧 API Usage

### Endpoint
```
POST /api/queries/dbpolicy/bulkupdate
```

### Request Body
```json
{
  "cntmgt_id": 1,
  "dbmgt_id": 5,
  "dbactormgt_id": 3,
  "new_policy_defaults": [10, 11, 12],
  "new_object_mgts": [100, 101, 102]
}
```

### Response
```json
{
  "message": "Bulk policy update background job started: F.C4LG5UMHSAPEI0J. Adding 6 policies, removing 3 policies.",
  "status": "job_started"
}
```

### cURL Example
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

---

## 🔄 How It Works (5 Steps)

```
1. CLIENT → Sends desired policy state
              ↓
2. SERVICE → Calculates diff (toAdd, toRemove)
              ↓
3. SERVICE → Builds GRANT/REVOKE SQL commands
              ↓
4. VELOARTIFACT → Executes commands on remote database
              ↓
5. HANDLER → Updates database ONLY if all commands succeed
```

---

## 💡 Key Concepts

### Cartesian Product
Your desired policies = `policy_defaults × object_mgts`

**Example:**
```
Input:
  policy_defaults = [SELECT, INSERT]
  object_mgts = [users_table, orders_table]

Result: 4 policies
  1. SELECT on users_table
  2. SELECT on orders_table
  3. INSERT on users_table
  4. INSERT on orders_table
```

### Diff Algorithm
```
Existing = current policies from database
Desired  = policy_defaults × object_mgts

toAdd    = Desired - Existing
toRemove = Existing - Desired
```

### Atomic Execution
**CRITICAL:** All commands must succeed before database updates.

```
If ANY command fails:
  → Entire operation aborted
  → No database changes applied
  → Error logged to audit file
```

---

## 📊 Common Scenarios

### Scenario 1: Grant New Permissions
```json
{
  "cntmgt_id": 1,
  "dbmgt_id": 5,
  "dbactormgt_id": 3,
  "new_policy_defaults": [10, 11],  // SELECT, INSERT
  "new_object_mgts": [100]          // users_table
}
```
**Result:** Adds SELECT and INSERT permissions on users_table

---

### Scenario 2: Revoke All Permissions
```json
{
  "cntmgt_id": 1,
  "dbmgt_id": 5,
  "dbactormgt_id": 3,
  "new_policy_defaults": [],  // Empty = no permissions
  "new_object_mgts": []       // Empty = no objects
}
```
**Result:** Removes all policies for this actor

---

### Scenario 3: Replace Permissions
```json
{
  "cntmgt_id": 1,
  "dbmgt_id": 5,
  "dbactormgt_id": 3,
  "new_policy_defaults": [10],  // Only SELECT
  "new_object_mgts": [100, 101] // users_table, orders_table
}
```
**Result:**
- Removes any INSERT/DELETE/UPDATE permissions
- Keeps/adds SELECT on specified tables

---

## ⚠️ Error Handling

### HTTP 400 - Bad Request
**Causes:**
- Missing required fields
- Empty policy_defaults or object_mgts arrays
- Invalid data types

**Example:**
```json
{
  "error": "invalid request body: Key: 'BulkPolicyUpdateRequest.CntMgtID' Error:Field validation for 'CntMgtID' failed on the 'required' tag"
}
```

---

### HTTP 404 - Not Found
**Causes:**
- Invalid cntmgt_id, dbmgt_id, or dbactormgt_id
- Policy default or object not found

**Example:**
```json
{
  "error": "database management not found: 999"
}
```

---

### HTTP 500 - Internal Server Error
**Causes:**
- VeloArtifact execution failure
- Database connection error
- File system permission issues

**Example:**
```json
{
  "error": "failed to execute VeloArtifact job: exit code 1"
}
```

---

## 🔍 Monitoring

### Check Job Status
```bash
# Application logs
tail -f logs/dbfartifact.log | grep "bulk_policy"

# Job-specific audit log
tail -f {VeloResultsDir}/bulk_policy_update_{job_id}.log
```

### Audit Log Format
```
2025/10/14 10:30:15 Starting bulk policy update for job F.C4LG5UMHSAPEI0J
2025/10/14 10:30:16 DELETED: 2 policies
2025/10/14 10:30:17 CREATED: 6 policies
2025/10/14 10:30:18 SUCCESS: added 6, removed 2 policies for actor_id=3
```

---

## 🛠️ Troubleshooting

### Issue: Job Never Completes

**Check:**
```bash
# VeloArtifact status
./veloartifact checkstatus {job_id}

# Job monitor logs
grep "{job_id}" logs/job_monitor.log
```

**Fix:**
- Restart VeloArtifact service
- Check network connectivity
- Verify job file exists in TEMP_DIR

---

### Issue: Job Completes but Database Not Updated

**Check:**
```bash
# Audit log for failures
cat {VeloResultsDir}/bulk_policy_update_{job_id}.log | grep FAILED
```

**Common Causes:**
- Remote database permission denied
- SQL syntax error in policy template
- Database object doesn't exist

**Fix:**
- Grant VeloArtifact user permissions on remote database
- Verify policy template validity
- Check database object names

---

### Issue: Database Inconsistency

**Symptoms:**
- Database shows policy exists
- Remote database shows no permission granted

**Diagnosis:**
```sql
-- Check database policies
SELECT * FROM dbpolicy WHERE actor_id = ? AND dbmgt_id = ?;

-- Check remote permissions (MySQL)
SHOW GRANTS FOR 'user'@'host';
```

**Resolution:**
1. Review audit logs for failed jobs
2. Manually reconcile remote permissions
3. Update database records if needed

---

## 🔐 Security Notes

### SQL Injection Prevention
- ✅ Templates stored as hex-encoded strings
- ✅ Variable substitution uses validated database values
- ✅ No user input directly concatenated into SQL

### Authorization
- ✅ All entity references validated before execution
- ✅ Relationship integrity checked (DBMgt belongs to CntMgt)
- ✅ Policy templates validated for actor type

### Audit Trail
- ✅ WHO: actor_id logged
- ✅ WHAT: policies added/removed logged
- ✅ WHEN: timestamp logged
- ✅ WHY: job_id links to request
- ✅ RESULT: success/failure logged

---

## 📈 Performance

| Metric | Value |
|--------|-------|
| API response time | <100ms |
| Job duration | 10-60s (depends on remote DB) |
| Max policies per request | ~1000 |
| Concurrent requests | Limited by VeloArtifact |

---

## 📚 Next Steps

1. **Read Full Documentation:** See `BULK_POLICY_UPDATE_TECHNICAL_SPEC.md`
2. **Test in Dev:** Use Postman/cURL to test the API
3. **Review Code:** Check implementation in:
   - `controllers/dbpolicy_controller.go`
   - `services/dbpolicy_service.go`
   - `services/bulk_policy_completion_handler.go`
4. **Monitor Production:** Set up alerts for job failures

---

## 📞 Support

**For Issues:**
- Check audit logs: `{VeloResultsDir}/bulk_policy_update_{job_id}.log`
- Review application logs: `logs/dbfartifact.log`
- Contact DBF Team

**For Questions:**
- Read technical spec: `BULK_POLICY_UPDATE_TECHNICAL_SPEC.md`
- Check implementation plan: `IMPLEMENTATION_PLAN.md`

---

**Happy Policy Managing! 🎉**