# DBF Artifact API - Project Overview & Product Development Requirements

**Version:** 1.0
**Last Updated:** 2026-02-24
**Status:** Active Development
**Team:** DBF Architecture & Development

---

## Executive Summary

The DBF Artifact API is a Go-based Database Firewall/Security Policy Management System that manages database access policies across multiple database instances (MySQL, Oracle, PostgreSQL, MSSQL). It integrates with the dbfAgentAPI platform for remote policy execution, privilege discovery, and compliance monitoring across distributed endpoints.

### Key Value Proposition
- **Centralized Policy Management** - Define once, enforce everywhere across heterogeneous database environments
- **Automated Privilege Discovery** - In-memory privilege sessions discover policies without impacting production databases
- **Compliance Monitoring** - Real-time policy compliance checks and audit trails
- **Scalable Architecture** - Background jobs and polling mechanisms support 100+ concurrent operations

---

## Project Vision & Goals

### Vision
Enable organizations to establish, enforce, and audit database access policies across heterogeneous database environments with minimal operational overhead and maximum security assurance.

### Strategic Goals
1. **Unified Policy Enforcement** - Single source of truth for database security policies
2. **Operational Efficiency** - Automated policy discovery and application reduces manual configuration
3. **Security Compliance** - Audit trails and compliance monitoring for regulatory requirements
4. **Extensibility** - Support for multiple database types and endpoint agents

### Business Objectives
- Reduce manual policy management overhead by 80%
- Enable policy compliance visibility across 100+ database instances
- Support multi-tenant/multi-region deployments
- Maintain 99.9% uptime for policy enforcement

---

## Target Audience

| Audience | Needs | Use Cases |
|----------|-------|-----------|
| **Security Architects** | Policy design, compliance frameworks | Define security policies, audit compliance |
| **Database Administrators** | Policy deployment, monitoring | Apply policies, troubleshoot failures |
| **DevOps Engineers** | Automation, monitoring, scaling | Deploy agents, monitor job status |
| **Compliance Officers** | Audit trails, reporting | Generate compliance reports, evidence |
| **Application Developers** | Policy queries, behavior documentation | Understand access constraints, debug policies |

---

## Feature List

### Core Features (MVP)

#### 1. Connection Management
- Create/update/delete database connections (MySQL, Oracle, PostgreSQL, MSSQL)
- Support Oracle CDB→PDB hierarchies
- Connection testing and validation
- Agent endpoint mapping for remote execution

#### 2. Database Asset Management
- Discover and manage database instances, schemas, objects
- Track database actors (users, roles, service accounts)
- Maintain object hierarchies (databases → tables → columns)

#### 3. Policy Definition & Enforcement
- Define policies with SQL allow/deny templates
- Support wildcard matching for actors and objects (-1 = wildcard)
- Hex-encoded SQL templates for security
- Bulk policy creation and updates
- Policy categorization and risk levels

#### 4. Privilege Discovery
- MySQL: In-memory go-mysql-server privilege analysis
- Oracle: CDB/PDB-aware privilege discovery
- Three-pass policy execution (super → action-wide → object-specific)
- Automatic policy creation based on discovered privileges
- Actor-to-group assignment automation

#### 5. Group Management
- Hierarchical group structures (self-referencing)
- Bulk assignment of policies to groups
- Bulk assignment of actors to groups
- Group-based policy targeting

#### 6. Background Job Orchestration
- dbfAgentAPI integration for remote execution
- Job monitoring with polling mechanism
- Completion callbacks for result processing
- Support for policy deployment, object discovery, backup, upload, download jobs

#### 7. Compliance & Monitoring
- Policy compliance checks against database privilege sets
- Job status monitoring and retry logic
- Audit trails with structured logging
- Session management (kill sessions, test connections)

#### 8. Oracle-Specific Features
- CDB/PDB detection and management
- Oracle privilege table queries (sys.dba_sys_privs, sys.dba_role_sys_privs)
- Pluggable database support

---

## Functional Requirements

### FR-1: RESTful API Design
- All operations exposed via Gin-based REST endpoints
- Base path: `/api/queries/`
- Standard HTTP methods (POST/GET/PUT/DELETE)
- JSON request/response bodies
- Swagger/OpenAPI documentation

### FR-2: Database Entity Management
- CRUD operations for: connections, databases, actors, objects, policies, groups
- Bulk create/update/delete operations
- Transaction support for atomic consistency
- Referential integrity enforcement

### FR-3: Policy Execution Flow
1. User submits policy request (via API)
2. Service builds hex-encoded JSON payload
3. Execute via dbfAgentAPI with background job option
4. Register callback with job monitor
5. Monitor polls job status every 10 seconds
6. On completion, callback processes results
7. Update database with policy records

### FR-4: Privilege Discovery
1. User requests privilege discovery for connection
2. System detects database type (MySQL/Oracle)
3. Create in-memory privilege session
4. Load privilege data from remote database
5. Execute three-pass policy engine
6. Auto-create policy records and group assignments

### FR-5: Error Handling & Recovery
- Graceful error messages with context
- Retry logic for transient failures (max retries configurable)
- Exponential backoff for agent API calls
- Transaction rollback on any failure
- Structured logging for debugging

### FR-6: Input Validation
- Validate all request bodies at controller boundaries
- MySQL host validation (resolve DNS, check connectivity)
- Safe integer conversion (prevent CWE-190 overflow)
- Policy parameter bounds checking

---

## Non-Functional Requirements

### NFR-1: Performance
- Policy creation: < 5 seconds (1000 policies)
- Privilege discovery: < 30 seconds (10k privileges)
- Job polling interval: 10 seconds
- Job monitor latency: < 100ms (register/cancel)

### NFR-2: Scalability
- Support 100+ concurrent background jobs
- Group assignments: 10k+ actors per group
- Connection management: 1000+ database instances
- Query response time: < 2 seconds for 100k records

### NFR-3: Reliability
- Job completion rate: 99.9% (with retry)
- Transaction atomicity: 100% (all-or-nothing)
- Graceful shutdown: Active job cleanup, no data loss
- Database connection pooling: 50+ concurrent connections

### NFR-4: Security
- SQL injection prevention: Hex-encoded templates + parameterized queries
- Input validation: All user inputs validated
- Integer overflow protection: Safe converters
- Audit logging: All policy changes logged
- No hardcoded credentials: Environment variables only

### NFR-5: Maintainability
- Code comments: Business context, not obvious actions
- Testability: Mockable repositories, dependency injection
- Documentation: README, technical specs, code examples
- Logging: Structured logs with context (zap-style)

### NFR-6: Availability
- Server uptime: 99.9% SLA
- Graceful degradation: Partial failures don't block other operations
- Log rotation: Prevent disk space exhaustion
- Database connection resilience: Automatic reconnection

---

## Technical Constraints

### Architecture
- **Framework:** Gin web framework (Go 1.24.1)
- **ORM:** GORM with MySQL driver
- **Database:** MySQL (primary), Oracle (optional), PG/MSSQL (connection only)
- **In-Memory Sessions:** go-mysql-server (privilege discovery)
- **Job Monitoring:** Polling-based (not event-driven)
- **Logging:** Lumberjack for log rotation

### Deployment
- **Containerization:** Not required (standalone binary)
- **Port:** Configurable (default 8081)
- **Log Storage:** File-based (rotation at 100MB)
- **Temp Storage:** DBFWEB_TEMP_DIR for hex-encoded payloads

### Database Schema
- **No migrations:** Schema assumed pre-existing
- **Implicit FKs:** No GORM associations (manual FK validation)
- **Wildcard support:** Integer fields use -1 for "all" (dbmgt_id, dbactormgt_id, dbobjectmgt_id)
- **Temporal tracking:** actor_groups, policy_groups have created_at/updated_at

### Integration Points
- **dbfAgentAPI:** External binary for remote execution
- **go-mysql-server:** In-memory MySQL for privilege analysis
- **Oracle drivers:** Optional, only if Oracle connections needed

---

## Success Metrics

### Functional Success
- All CRUD endpoints operational and tested
- Bulk policy update with 100% atomic consistency
- Privilege discovery completes without production impact
- Job completion callbacks process 99.9% of results

### Operational Success
- Application startup in < 5 seconds
- Response times < 2 seconds for 99th percentile
- Zero unhandled panics in production
- Graceful shutdown with active job cleanup

### Code Quality Success
- 80%+ test coverage for business logic
- Zero security vulnerabilities (verified via code review)
- All critical paths documented with comments
- Zero hardcoded credentials in codebase

### User Success
- New developers can understand system in 1 hour
- API usage examples cover 80% of common scenarios
- Troubleshooting guide resolves 90% of issues
- Swagger documentation is current and accurate

---

## Assumptions & Dependencies

### Assumptions
1. **Database schema pre-exists** - No schema migrations required
2. **dbfAgentAPI available** - Remote agents running and accessible
3. **MySQL connectivity** - Application can connect to MySQL database
4. **No external auth** - Authentication assumed handled upstream (e.g., API gateway)
5. **Hex-encoded payloads sufficient** - SQL injection prevention via encoding

### External Dependencies
- **dbfAgentAPI** (binary) - Remote policy execution and file operations
- **MySQL database** - Application schema and data storage
- **go-mysql-server** (library) - In-memory privilege discovery
- **Gin framework** (library) - HTTP request handling
- **GORM library** (library) - Database abstraction

### Environment Dependencies
- **DBFWEB_TEMP_DIR** - Directory for hex-encoded JSON files
- **AGENT_API_PATH** - Path to dbfAgentAPI binary
- **LOG_FILE** - Log file location with write permissions

---

## Design Decisions & Rationale

### 1. Hex-Encoded SQL Templates
**Decision:** Store policy templates hex-encoded, decode only at execution time
**Rationale:** Prevents SQL injection by ensuring templates never expose raw SQL
**Trade-off:** Requires decode step, slight performance impact
**Alternative Rejected:** Parameterized templates (less flexible)

### 2. In-Memory Privilege Sessions
**Decision:** Create transient in-memory MySQL/Oracle instances for discovery
**Rationale:** Analyze privileges without impacting production databases
**Trade-off:** Requires additional memory, must load privilege data first
**Alternative Rejected:** Direct database queries (would lock production systems)

### 3. Polling-Based Job Monitoring
**Decision:** Poll job status every 10 seconds
**Rationale:** Simpler than event-driven; compatible with dbfAgentAPI
**Trade-off:** 10-second latency for completion detection
**Alternative Rejected:** Event-driven callbacks (requires agent-side support)

### 4. Three-Pass Policy Engine
**Decision:** Execute policies in three passes (super → action-wide → object)
**Rationale:** Mirrors database privilege hierarchy for accuracy
**Trade-off:** More complex logic, requires two database connections
**Alternative Rejected:** Single-pass (might miss privilege grants at higher levels)

### 5. Self-Referencing Group Hierarchies
**Decision:** Use self-ref FKs for nested group structures (ParentGroupID)
**Rationale:** Flexible group hierarchies with unlimited nesting
**Trade-off:** Query complexity for deep hierarchies
**Alternative Rejected:** Nested set model (harder to maintain)

### 6. Implicit FKs (No GORM Associations)
**Decision:** Define FKs in SQL, reference manually in code
**Rationale:** Simpler GORM usage, avoid lazy-loading issues
**Trade-off:** Manual FK validation, no automatic cascading
**Alternative Rejected:** GORM associations (complexity, lazy-loading gotchas)

### 7. Safe Integer Converters
**Decision:** Use custom converters for int/uint to prevent overflow
**Rationale:** CWE-190 mitigation for integer overflow
**Trade-off:** Extra validation layer
**Alternative Rejected:** Unchecked casting (security vulnerability)

---

## Constraints & Limitations

### Known Limitations
1. **Single MySQL instance** - No sharding or replication support
2. **Polling latency** - Job completion detected within 10 seconds
3. **No async API** - Requests block until job queued (not execution)
4. **Oracle optional** - Full support requires Oracle drivers
5. **Hex-encoding size** - Large payloads (>10MB) may hit limits
6. **No authentication** - Assumes upstream auth (API gateway)

### Scaling Limitations
- **Concurrent jobs:** 100+ recommended (tunable via config)
- **Group size:** 10k+ actors per group (single transaction)
- **Policy count:** 100k+ policies (query indexing required)
- **Privilege discovery:** 10k+ privileges (memory constraints)

---

## Development Roadmap

### Phase 1: Foundation (Complete)
- Core entity CRUD (connections, databases, actors, objects)
- REST API with Swagger documentation
- Database connection and authentication
- Structured logging infrastructure

### Phase 2: Policy Engine (Complete)
- Policy creation, update, delete
- Bulk policy operations
- dbfAgentAPI integration
- Job monitoring and callbacks

### Phase 3: Privilege Discovery (In Progress)
- MySQL privilege session implementation
- Oracle privilege session implementation
- Three-pass policy execution
- Auto-policy creation from discovered privileges

### Phase 4: Group Management (In Progress)
- Hierarchical group structures
- Group-based policy assignment
- Bulk actor assignment to groups
- Policy list definitions with risk levels

### Phase 5: Advanced Features (Planned)
- Policy compliance monitoring
- Advanced filtering and search
- Policy versioning and rollback
- API rate limiting
- Multi-tenant support

### Phase 6: Operations (Planned)
- Comprehensive monitoring dashboard
- Advanced logging and analytics
- Performance optimization
- Enterprise deployment support

---

## Open Questions & Risks

### Open Questions
1. **Authentication:** How should upstream auth integrate with API?
2. **Multi-tenancy:** Should single instance support multiple organizations?
3. **Policy versioning:** Should we track historical policy versions?
4. **Disaster recovery:** What's the recovery strategy for policy data loss?

### Identified Risks

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|-----------|
| dbfAgentAPI unavailable | No policy execution | Medium | Retry logic, fallback agents |
| Large policy payloads | Job timeout | Low | Payload size validation |
| Oracle privilege complexity | Incomplete discovery | Medium | Comprehensive testing |
| Concurrent job bottleneck | Performance degradation | Low | Job queue tuning |
| Integer overflow in policies | Incorrect policy targeting | Low | Safe converters + validation |

---

## Success Criteria

The project will be considered successful when:

1. **Functional Completeness**
   - All CRUD endpoints implemented and tested
   - Bulk operations working with 99.9% atomic consistency
   - Privilege discovery working for MySQL and Oracle
   - Job monitoring with callback completion

2. **Code Quality**
   - 80%+ test coverage for business logic
   - All public functions documented with comments
   - Security review passed (no SQL injection, CWE-190 mitigated)
   - Code follows CLAUDE.md standards

3. **Operational Readiness**
   - Deployment guide written and tested
   - Runbook for common issues created
   - Performance benchmarks documented
   - Monitoring and alerting configured

4. **User Adoption**
   - API documentation complete with examples
   - Quick start guide available
   - Troubleshooting guide covers 90% of issues
   - Team trained on system operation

---

## Maintenance & Evolution

### Maintenance Obligations
- **Bug fixes:** Address within 48 hours for critical issues
- **Security updates:** Immediate response for vulnerabilities
- **Log rotation:** Ensure disk space doesn't exceed limits
- **Documentation:** Update when functionality changes

### Evolution Strategy
1. Monitor usage patterns and performance metrics
2. Collect feedback from operators and users
3. Prioritize improvements based on impact
4. Test thoroughly before production deployment
5. Maintain backward compatibility where possible

---

**Document Owner:** DBF Architecture Team
**Review Frequency:** Quarterly
**Last Reviewed:** 2026-02-24
