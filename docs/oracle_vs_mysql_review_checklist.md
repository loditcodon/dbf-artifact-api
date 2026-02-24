# Oracle vs MySQL Privilege Session - Review Checklist

## Mục đích
Kiểm tra xem Oracle implementation đã thực hiện đầy đủ các bước như MySQL chưa.

## Tổng kết

| Trạng thái | Ý nghĩa |
|------------|---------|
| ✅ PASS | Oracle đã implement đầy đủ và đúng logic |
| ⚠️ DIFF | Có khác biệt nhưng là thiết kế có chủ đích |
| ❌ FAIL | Thiếu hoặc sai logic |
| ⏳ Pending | Chưa kiểm tra |

---

## Checklist Chi Tiết

### 1. Entry Point
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Function | `GetByCntMgt()` → `GetByCntMgtWithPrivilegeSession()` | `GetByCntMgt()` → `GetByCntMgtWithOraclePrivilegeSession()` | ✅ PASS |
| Routing | `cntType == "mysql"` | `cntType == "oracle"` | ✅ PASS |
| File | [dbpolicy_service.go:96-121](../services/dbpolicy_service.go#L96-L121) | [dbpolicy_service.go:654-784](../services/dbpolicy_service.go#L654-L784) | ✅ PASS |

**Chi tiết:**
- Oracle routing hoạt động đúng tại `dbpolicy_service.go:114-115`
- Có kiểm tra `cntType` và route đến handler phù hợp

---

### 2. Build Privilege Queries
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Function | `buildPrivilegeDataQueries()` | `buildOraclePrivilegeDataQueries()` | ✅ PASS |
| File | [dbpolicy_service.go:823-903](../services/dbpolicy_service.go#L823-L903) | [oracle_privilege_queries.go:20-99](../services/oracle_privilege_queries.go#L20-L99) | ✅ PASS |
| Actor Filter | `(User, Host) IN (...)` | `GRANTEE IN (...)` | ✅ PASS |
| Database Filter | `Db IN (...)` | N/A (Oracle dùng OWNER filter trong DBA_TAB_PRIVS) | ⚠️ DIFF |

**Privilege Tables Queried:**

| MySQL | Oracle |
|-------|--------|
| mysql.user | DBA_SYS_PRIVS |
| mysql.db | DBA_TAB_PRIVS |
| mysql.tables_priv | DBA_ROLE_PRIVS |
| mysql.procs_priv | V$PWFILE_USERS |
| mysql.role_edges | CDB_SYS_PRIVS (CDB only) |
| mysql.global_grants | - |
| mysql.proxies_priv | - |
| information_schema.* | - |

**Ghi chú:**
- Oracle query thêm CDB_SYS_PRIVS chỉ cho CDB connections (`connType == OracleConnectionCDB`)
- Oracle không có tương đương `information_schema` tables

---

### 3. Write Queries to JSON File
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Function | `writePrivilegeQueryFile()` | `writeOraclePrivilegeQueryFile()` | ✅ PASS |
| File | [dbpolicy_service.go:920-950](../services/dbpolicy_service.go#L920-L950) | [oracle_privilege_queries.go:104-137](../services/oracle_privilege_queries.go#L104-L137) | ✅ PASS |
| File naming | `getprivilegedata_{cntMgtID}_{timestamp}.json` | `oracle_privileges_{cntMgtID}_{timestamp}.json` | ✅ PASS |
| Directory | `config.Cfg.DBFWebTempDir` | `config.Cfg.DBFWebTempDir` | ✅ PASS |
| JSON encoding | `encoder.SetEscapeHTML(false)`, `SetIndent` | Same | ✅ PASS |

---

### 4. Execute via dbfAgentAPI Background Job
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Action | `download` | `download` | ✅ PASS |
| Option | `--background` | `--background` | ✅ PASS |
| executeSqlAgentAPI call | ✅ | ✅ | ✅ PASS |
| JobResponse parsing | ✅ | ✅ | ✅ PASS |

**Chi tiết:**
- MySQL: [dbpolicy_service.go:600-614](../services/dbpolicy_service.go#L600-L614)
- Oracle: [dbpolicy_service.go:738-752](../services/dbpolicy_service.go#L738-L752)
- Cả hai sử dụng cùng pattern: build queryParam → create hexJSON → execute với `--background`

---

### 5. Job Monitor Registration
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| GetJobMonitorService() | ✅ | ✅ | ✅ PASS |
| AddJobWithCallback() | ✅ | ✅ | ✅ PASS |
| Context key | `"privilege_session_context"` | `"oracle_privilege_session_context"` | ✅ PASS |

**Chi tiết:**
- MySQL: [dbpolicy_service.go:634-637](../services/dbpolicy_service.go#L634-L637)
- Oracle: [dbpolicy_service.go:774-777](../services/dbpolicy_service.go#L774-L777)

---

### 6. Completion Handler
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Function | `CreatePrivilegeSessionCompletionHandler()` | `CreateOraclePrivilegeSessionCompletionHandler()` | ✅ PASS |
| File | privilege_session_handler.go | oracle_privilege_session_handler.go | ✅ PASS |
| Notification handling | ✅ | ✅ | ✅ PASS |
| VeloArtifact polling | ✅ | ✅ | ✅ PASS |

**Chi tiết:**
- Oracle handler: [oracle_privilege_session_handler.go:28-46](../services/oracle_privilege_session_handler.go#L28-L46)
- Hỗ trợ cả notification-based và VeloArtifact polling flow

---

### 7. Parse Privilege Results
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Function | N/A (trong loadPrivilegeDataFromResults) | `parseOraclePrivilegeResults()` | ✅ PASS |
| File | privilege_session_handler.go | oracle_privilege_session_handler.go:1119-1192 | ✅ PASS |
| Data structure | Direct column mapping | `OraclePrivilegeData` struct | ✅ PASS |

**Oracle parse functions:**
- `parseDbasSysPrivs()` - DBA_SYS_PRIVS
- `parseDbasTabPrivs()` - DBA_TAB_PRIVS
- `parseDbasRolePrivs()` - DBA_ROLE_PRIVS
- `parsePwFileUsers()` - V$PWFILE_USERS
- `parseCdbSysPrivs()` - CDB_SYS_PRIVS

---

### 8. Create In-Memory Server
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Function | `NewPrivilegeSession()` | `NewOraclePrivilegeSession()` | ✅ PASS |
| File | [privilege_session.go:35-114](../services/privilege_session.go#L35-L114) | [privilege_session.go:367-441](../services/privilege_session.go#L367-L441) | ✅ PASS |
| Database name | `"mysql"` | `"oracle"` | ✅ PASS |
| Table creation | `createPrivilegeTables()` | `createOraclePrivilegeTables()` | ✅ PASS |
| Server timeout | 5 seconds | 5 seconds | ✅ PASS |
| Goroutine cleanup | context cancellation | context cancellation | ✅ PASS |

**Oracle Privilege Tables Created:**
- DBA_SYS_PRIVS
- DBA_TAB_PRIVS
- DBA_ROLE_PRIVS
- V_PWFILE_USERS (renamed from V$PWFILE_USERS)
- CDB_SYS_PRIVS

---

### 9. Load Privilege Data into Session
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Function | `loadPrivilegeDataFromResults()` | `LoadOraclePrivilegeDataFromResults()` | ✅ PASS |
| File | privilege_session_handler.go | [privilege_session.go:443-518](../services/privilege_session.go#L443-L518) | ✅ PASS |
| INSERT logic | Per-row INSERT | Per-row INSERT | ✅ PASS |
| Error handling | Log warning, continue | Log warning, continue | ✅ PASS |

---

### 10. PASS-1: Super Privileges
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Function | (trong createPoliciesWithPrivilegeData) | `executeOracleSuperPrivilegeQueries()` | ✅ PASS |
| File | privilege_session_handler.go | oracle_privilege_session_handler.go:752-841 | ✅ PASS |
| Policy ID | 1 (hardcoded) | 1 (hardcoded) | ✅ PASS |
| Result | actor_id=<id>, object_id=-1, dbmgt_id=-1 | Same | ✅ PASS |
| Skip PASS-2/3 | `superPrivActors.markSuperPrivilege()` | Same | ✅ PASS |
| Concurrent execution | Semaphore pattern | Semaphore pattern | ✅ PASS |
| Panic recovery | ✅ | ✅ | ✅ PASS |

---

### 11. PASS-2: Action-Wide Privileges
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Function | (trong createPoliciesWithPrivilegeData) | `executeOracleActionWideQueries()` | ✅ PASS |
| File | privilege_session_handler.go | oracle_privilege_session_handler.go:843-941 | ✅ PASS |
| Source | DBGroupListPolicies (database_type_id=1) | DBGroupListPolicies (database_type_id=3) | ✅ PASS |
| Result | object_id=-1, dbmgt_id=-1 | Same | ✅ PASS |
| Skip condition | `superPrivActors.hasSuperPrivilege()` | Same | ✅ PASS |
| Cache granted actions | `grantedActions.markGranted()` | Same | ✅ PASS |
| Concurrent execution | Semaphore pattern | Semaphore pattern | ✅ PASS |

---

### 12. PASS-3: Object-Specific Privileges
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Function | (trong createPoliciesWithPrivilegeData) | `executeOracleObjectSpecificQueries()` | ✅ PASS |
| File | privilege_session_handler.go | oracle_privilege_session_handler.go:943-1043 | ✅ PASS |
| Skip conditions | Super privilege + action granted | Same | ✅ PASS |
| General queries | objectId=-1 | objectId=-1 | ✅ PASS |
| Specific queries | objectId!=1 | objectId!=1 | ✅ PASS |
| Concurrent execution | Semaphore pattern | Semaphore pattern | ✅ PASS |

---

### 13. General SQL Templates Processing
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Function | `processGeneralSQLTemplatesForSession()` | `processOracleSQLTemplatesForSession()` | ✅ PASS |
| File | privilege_session_handler.go | oracle_privilege_session_handler.go:493-553 | ✅ PASS |
| Uses SqlGet | ✅ | ✅ | ✅ PASS |
| Returns objectId=-1 | ✅ | ✅ | ✅ PASS |

**Variable Substitutions:**

| Variable | MySQL | Oracle |
|----------|-------|--------|
| `${dbmgt.dbname}` | ✅ | ✅ |
| `${dbactormgt.dbuser}` | ✅ | ✅ |
| `${dbactormgt.ip_address}` | ✅ | ✅ |
| `${dbobjectmgt.objectname}` | Set to `*` | Set to `*` |
| `${dbobject.objecttype}` | N/A | ✅ (CDB: "*", PDB: "PDB") |

---

### 14. Specific SQL Templates Processing
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Function | `processSpecificSQLTemplatesForSession()` | `processOracleSpecificSQLTemplatesForSession()` | ✅ PASS |
| File | privilege_session_handler.go | oracle_privilege_session_handler.go:555-687 | ✅ PASS |
| Uses SqlGetSpecific | ✅ | ✅ | ✅ PASS |
| Object lookup | cache.objectsByKey | cache.objectsByKey | ✅ PASS |

---

### 15. Policy Allowed Check
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Function | `isPolicyAllowed()` | `isOraclePolicyAllowed()` | ✅ PASS |
| File | dbpolicy_service.go:809-821 | oracle_privilege_session_handler.go:1045-1075 | ✅ PASS |
| Logic | resAllow, resDeny, "NOT NULL" | Same | ✅ PASS |
| SqlGetAllow/SqlGetDeny | Plain strings (NOT hex) | Plain strings (NOT hex) | ✅ PASS |

---

### 16. Query Build Cache
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Type | `queryBuildCache` | `oracleQueryBuildCache` | ✅ PASS |
| File | privilege_session_handler.go | oracle_privilege_session_handler.go:689-750 | ✅ PASS |
| Key format | `"objectId:dbMgtId"` | `"objectId:dbMgtId"` | ✅ PASS |
| Prevents N+1 | ✅ | ✅ | ✅ PASS |

---

### 17. Assign Actors to Groups
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Function | `assignActorsToGroups()` | `assignOracleActorsToGroups()` | ✅ PASS |
| File | privilege_session_handler.go:291-526 | oracle_privilege_session_handler.go:1364-1578 | ✅ PASS |
| Super priv group | Group ID = 1 | Group ID = 1000 | ⚠️ DIFF |
| Two-level matching | ✅ | ✅ | ✅ PASS |
| database_type_id | 1 (MySQL) | 2 (Oracle) | ✅ PASS |

**Khác biệt có chủ đích:**
- MySQL super privilege group: **1**
- Oracle super privilege group: **1000**

---

### 18. Export DBF Policy
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Function | `utils.ExportDBFPolicy()` | `utils.ExportDBFPolicy()` | ✅ PASS |
| File | privilege_session_handler.go | oracle_privilege_session_handler.go:366-371 | ✅ PASS |
| Called after commit | ✅ | ✅ | ✅ PASS |

---

### 19. Parallel Query Execution
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Concurrency config | `config.GetPrivilegeQueryConcurrency()` | Same | ✅ PASS |
| Semaphore pattern | ✅ | ✅ | ✅ PASS |
| Results channel | ✅ | ✅ | ✅ PASS |
| Serial DB operations | ✅ | ✅ | ✅ PASS |

---

### 20. Error Handling
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Idempotency check | `processedPrivilegeJobs` sync.Map | `processedOraclePrivilegeJobs` sync.Map | ✅ PASS |
| Transaction rollback | ✅ | ✅ | ✅ PASS |
| Panic recovery in goroutines | ✅ | ✅ | ✅ PASS |
| Graceful missing table handling | ✅ | ✅ | ✅ PASS |

---

### 21. Query Rewriting (Oracle-specific)
| Mục | MySQL | Oracle | Status |
|-----|-------|--------|--------|
| Table name rewriting | information_schema → mysql.infoschema_* | V$PWFILE_USERS → V_PWFILE_USERS | ✅ PASS |
| Reason | go-mysql-server không cho INSERT vào information_schema | `$` không được phép trong table name | ✅ PASS |

---

### 22. Connection Type Detection (Oracle-specific)
| Mục | Oracle | Status |
|-----|--------|--------|
| Function | `GetOracleConnectionType()` | ✅ PASS |
| File | [oracle_connection_helper.go:37-42](../services/oracle_connection_helper.go#L37-L42) | ✅ PASS |
| CDB detection | `ParentConnectionID == nil || 0` | ✅ PASS |
| PDB detection | `ParentConnectionID != nil && != 0` | ✅ PASS |
| Object type wildcard | CDB: "*", PDB: "PDB" | ✅ PASS |

---

### 23. Oracle-specific Object Queries
| Mục | Oracle | Status |
|-----|--------|--------|
| Function | `buildOracleObjectQueries()` | ✅ PASS |
| File | [oracle_privilege_queries.go:145-245](../services/oracle_privilege_queries.go#L145-L245) | ✅ PASS |

**Tables queried:**
- ALL_TABLES
- ALL_VIEWS
- ALL_PROCEDURES
- ALL_SEQUENCES
- ALL_INDEXES
- ALL_TRIGGERS
- DBA_USERS
- DBA_ROLES
- DBA_PDBS (CDB only)
- V$DATABASE
- V$INSTANCE

---

## Kết luận

### Tổng hợp

| Trạng thái | Số lượng |
|------------|----------|
| ✅ PASS | 21 |
| ⚠️ DIFF (thiết kế có chủ đích) | 2 |
| ❌ FAIL | 0 |

### Khác biệt có chủ đích

1. **Super privilege group ID:**
   - MySQL: 1
   - Oracle: 1000
   - Lý do: Phân biệt giữa MySQL và Oracle trong cùng hệ thống

2. **Database filter trong privilege queries:**
   - MySQL: Sử dụng `Db IN (...)` filter
   - Oracle: Không có filter tương tự (Oracle dùng schema ownership thay vì database)

### Đánh giá chung

**Oracle implementation ĐÃ THỰC HIỆN ĐẦY ĐỦ tất cả các bước như MySQL:**

1. ✅ Entry point và routing
2. ✅ Build privilege queries (với Oracle-specific tables)
3. ✅ Write queries to JSON file
4. ✅ Execute via dbfAgentAPI background job
5. ✅ Job monitor registration
6. ✅ Completion handler (notification + polling)
7. ✅ Parse privilege results
8. ✅ Create in-memory server (go-mysql-server với Oracle tables)
9. ✅ Load privilege data into session
10. ✅ Three-pass execution (PASS-1, PASS-2, PASS-3)
11. ✅ SQL template processing (general + specific)
12. ✅ Policy allowed check
13. ✅ Query build cache
14. ✅ Assign actors to groups
15. ✅ Export DBF policy
16. ✅ Parallel query execution (semaphore pattern)
17. ✅ Error handling (idempotency, transaction, panic recovery)
18. ✅ Oracle-specific features (CDB/PDB detection, table rewriting)

---

## Files Reviewed

| File | Purpose |
|------|---------|
| services/dbpolicy_service.go | Entry point, routing |
| services/oracle_privilege_queries.go | Query building |
| services/oracle_privilege_session_handler.go | Main handler, 3-pass execution |
| services/oracle_privilege_session.go | Types and data structures |
| services/oracle_connection_helper.go | CDB/PDB detection |
| services/privilege_session.go | In-memory server |
| services/privilege_session_handler.go | MySQL handler (for comparison) |

---

*Review completed: 2026-02-06*
