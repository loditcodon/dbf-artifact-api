# DBF Artifact API

**Database Firewall/Security Policy Management System**

A Go-based REST API for managing database access policies across heterogeneous database environments (MySQL, Oracle, PostgreSQL, MSSQL). Integrates with dbfAgentAPI for remote policy execution and provides automated privilege discovery through in-memory database sessions.

**Status:** Active Development | **Language:** Go 1.24.1 | **License:** Internal Use

---

## Quick Links

- **Documentation:** `./docs/` directory
- **API Specification:** [Swagger Docs](#swagger-documentation)
- **Getting Started:** [Setup & Run](#setup--run)
- **Architecture:** [System Architecture](./docs/system-architecture.md)
- **Development:** [Code Standards](./docs/code-standards.md)
- **Roadmap:** [Project Roadmap](./docs/project-roadmap.md)

---

## Overview

The DBF Artifact API enables organizations to:

- **Centralize Policy Management** - Define and enforce database access policies from a single platform
- **Discover Privileges Safely** - Analyze database privileges without impacting production systems
- **Manage at Scale** - Handle 1000+ database instances and 100k+ policies
- **Ensure Compliance** - Monitor policy compliance and generate audit trails
- **Automate Assignments** - Bulk operations for policies, actors, and groups

### Key Features

| Feature | Status | Details |
|---------|--------|---------|
| Connection Management | ✅ Complete | MySQL, Oracle, PostgreSQL, MSSQL |
| Policy CRUD | ✅ Complete | Create, update, delete, bulk operations |
| Privilege Discovery | ✅ Complete | MySQL & Oracle in-memory analysis |
| Group Management | ✅ In Progress | Hierarchical groups, policy assignment |
| Job Monitoring | ✅ Complete | Background job tracking, callbacks |
| Policy Compliance | 🔄 Planned | Compliance checks, audit trails |
| API Rate Limiting | 🔄 Planned | Per-user and per-endpoint limits |

---

## System Architecture

```
HTTP Clients
    ↓
Controllers (Gin REST handlers)
    ↓
Services (Business logic + orchestration)
    ↓
Repository Layer (Data access with GORM)
    ↓
Models (Domain entities)
    ↓
MySQL Database
```

**Additional Components:**
- **Job Monitor Service** - Background job polling (10s intervals)
- **Agent API Service** - dbfAgentAPI integration with retry logic
- **Privilege Session Handlers** - MySQL/Oracle in-memory privilege analysis
- **Completion Handlers** - Job result processing and database updates

For detailed architecture, see [System Architecture](./docs/system-architecture.md).

---

## Prerequisites

- **Go:** 1.24.1 or later
- **MySQL:** 5.7+ or 8.0+
- **Environment:** Linux or macOS (Windows with WSL2)

**Optional:**
- **Oracle:** 19c+ (if Oracle database support needed)
- **Docker:** For containerized deployment

---

## Setup & Run

### 1. Clone Repository

```bash
git clone <repository-url>
cd dbfartifactapi_151
```

### 2. Install Dependencies

```bash
go mod download
go mod verify
```

### 3. Configure Environment

Copy `.env.example` to `.env` and update values:

```bash
cp .env.example .env
```

**Required environment variables:**

```bash
# Database Configuration
DB_HOST=localhost
DB_PORT=3306
DB_USER=dbf_user
DB_PASS=your_password
DB_NAME=dbfartifactapi

# Server Configuration
PORT=8081

# Logging
LOG_LEVEL=info
LOG_FILE=/var/log/dbf/dbfartifactapi.log
LOG_MAX_SIZE=100
LOG_MAX_BACKUPS=10
LOG_MAX_AGE=30
LOG_COMPRESS=true

# Paths
DBFWEB_TEMP_DIR=/tmp/dbfweb
AGENT_API_PATH=/usr/local/bin/dbfAgentAPI

# Agent Configuration
AGENT_EXECUTION_TIMEOUT=300
AGENT_MAX_RETRIES=3
AGENT_RETRY_BASE_DELAY=1000

# Concurrency
PRIVILEGE_LOAD_CONCURRENCY=5
PRIVILEGE_QUERY_CONCURRENCY=3
```

**Optional configuration:**
```bash
# Filters
SYSTEM_DATABASES=mysql,information_schema,performance_schema
SYSTEM_USERS=root,mysql,dbf_agent

# VeloArtifact Integration (legacy)
VELO_EXECUTION_TIMEOUT=300
VELO_MAX_RETRIES=3

# Oracle Support
DBF_CHECK_POLICY_COMPLIANCE_PATH=/etc/v2/dbf/bin/v2dbfsqldetector
```

### 4. Build Application

```bash
go build -o dbfartifactapi .
```

Or with optimizations:

```bash
go build -ldflags="-s -w" -o dbfartifactapi .
```

### 5. Run Application

```bash
./dbfartifactapi
```

**Expected output:**
```
[GIN] Starting server at 0.0.0.0:8081
Server started on port 8081
Visit http://localhost:8081/swagger/index.html for API documentation
```

### 6. Verify Installation

Test the API:

```bash
curl -X GET http://localhost:8081/api/queries/dbmgt/all \
  -H "Content-Type: application/json"
```

---

## API Overview

### Base Path

```
/api/queries/
```

### Main Endpoints

#### Connections (Database Management)
```
POST   /api/queries/dbmgt                    Create connection
GET    /api/queries/dbmgt/all                List all connections
GET    /api/queries/dbmgt/:id                Get connection details
PUT    /api/queries/dbmgt/:id                Update connection
DELETE /api/queries/dbmgt/:id                Delete connection
```

#### Actors (Database Users)
```
POST   /api/queries/dbactormgt               Create actor
GET    /api/queries/dbactormgt/all           List actors
PUT    /api/queries/dbactormgt/:id           Update actor
DELETE /api/queries/dbactormgt/:id           Delete actor
```

#### Objects (Database Objects)
```
POST   /api/queries/dbobjectmgt              Create object
GET    /api/queries/dbobjectmgt/all          List objects
PUT    /api/queries/dbobjectmgt/:id          Update object
DELETE /api/queries/dbobjectmgt/:id          Delete object
```

#### Policies
```
POST   /api/queries/dbpolicy                 Create policy
GET    /api/queries/dbpolicy/all             List policies
POST   /api/queries/dbpolicy/bulk-update     Bulk update policies
POST   /api/queries/dbpolicy/bulk-delete     Bulk delete policies
```

#### Groups
```
POST   /api/queries/groups                   Create group
GET    /api/queries/groups/all               List groups
PUT    /api/queries/groups/:id               Update group
POST   /api/queries/groups/bulk-assign-policies   Bulk assign policies
```

#### Job Status
```
GET    /api/jobs/:job-id                     Get job status
POST   /api/jobs/:job-id/cancel              Cancel job
```

### Complete API Documentation

Visit Swagger UI after starting the server:

```
http://localhost:8081/swagger/index.html
```

---

## Development

### Project Structure

```
dbfartifactapi_151/
├── main.go                 - Entry point
├── config/                 - Configuration & database setup
├── controllers/            - HTTP handlers
├── services/               - Business logic (33 services)
├── models/                 - GORM domain entities
├── repository/             - Data access layer
├── bootstrap/              - Startup data loading
├── utils/                  - Shared utilities
├── pkg/logger/             - Logging infrastructure
├── mocks/                  - Test mocks (mockery-generated)
├── docs/                   - Technical documentation
└── CLAUDE.md              - Development standards
```

For detailed explanation, see [Codebase Summary](./docs/codebase-summary.md).

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run integration tests
go test -tags=integration ./...

# Run specific test
go test -run TestName ./...
```

### Code Standards

All code must follow standards in [Code Standards](./docs/code-standards.md):

- **Comments:** Explain WHY, not WHAT
- **Error Handling:** Always wrap with context
- **Testing:** Unit tests + integration tests required
- **Security:** SQL injection prevention, integer overflow protection
- **Logging:** Structured logging only (no fmt.Print*)

### Building Mocks

```bash
mockery  # Generates mocks/ directory from interfaces
```

### Code Review

Before submitting PR:

```bash
# Format code
go fmt ./...

# Run linter
go vet ./...

# Run tests
go test ./...

# Check for issues
go staticcheck ./...
```

---

## Configuration Reference

### Database Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| DB_HOST | - | MySQL hostname |
| DB_PORT | 3306 | MySQL port |
| DB_USER | - | Database user |
| DB_PASS | - | Database password |
| DB_NAME | - | Database name |

### Server Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| PORT | 8081 | Server port |
| LOG_LEVEL | info | Logging level (debug, info, warn, error) |
| LOG_FILE | - | Log file path |
| LOG_MAX_SIZE | 100 | Max log file size (MB) |
| LOG_MAX_BACKUPS | 10 | Number of backup files |
| LOG_MAX_AGE | 30 | Backup retention (days) |

### Path Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| DBFWEB_TEMP_DIR | /tmp/dbfweb | Temp directory for payloads |
| AGENT_API_PATH | /usr/local/bin/dbfAgentAPI | Path to agent API binary |
| VELO_RESULTS_DIR | /tmp/velo | Legacy VeloArtifact results |

### Agent Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| AGENT_EXECUTION_TIMEOUT | 300 | Command timeout (seconds) |
| AGENT_MAX_RETRIES | 3 | Maximum retry attempts |
| AGENT_RETRY_BASE_DELAY | 1000 | Base retry delay (ms) |

### Advanced Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| PRIVILEGE_LOAD_CONCURRENCY | 5 | Concurrent privilege loads |
| PRIVILEGE_QUERY_CONCURRENCY | 3 | Concurrent privilege queries |
| ENABLE_MYSQL_PRIVILEGE_QUERY_LOGGING | false | Debug privilege queries |

---

## Environment Configuration Examples

### Development Environment

```bash
DB_HOST=localhost
DB_PORT=3306
DB_USER=dev_user
DB_PASS=dev_password
DB_NAME=dbfartifactapi_dev
PORT=8081
LOG_LEVEL=debug
LOG_FILE=./logs/dbfartifactapi.log
DBFWEB_TEMP_DIR=/tmp/dbfweb_dev
AGENT_API_PATH=/usr/local/bin/dbfAgentAPI
```

### Production Environment

```bash
DB_HOST=prod-db.example.com
DB_PORT=3306
DB_USER=dbf_service
DB_PASS=${DB_PASSWORD}  # From secrets manager
DB_NAME=dbfartifactapi
PORT=8081
LOG_LEVEL=warn
LOG_FILE=/var/log/dbf/dbfartifactapi.log
LOG_MAX_SIZE=500
LOG_MAX_BACKUPS=20
LOG_MAX_AGE=90
LOG_COMPRESS=true
DBFWEB_TEMP_DIR=/var/tmp/dbfweb
AGENT_API_PATH=/usr/bin/dbfAgentAPI
AGENT_EXECUTION_TIMEOUT=600
AGENT_MAX_RETRIES=5
PRIVILEGE_LOAD_CONCURRENCY=10
PRIVILEGE_QUERY_CONCURRENCY=5
```

---

## Common Tasks

### Create a Database Connection

```bash
curl -X POST http://localhost:8081/api/queries/dbmgt \
  -H "Content-Type: application/json" \
  -d '{
    "type": "mysql",
    "host": "db.example.com",
    "port": 3306,
    "username": "dbf_user",
    "password": "password",
    "database": "production",
    "agent": "agent-1"
  }'
```

### Discover Privileges

```bash
curl -X POST http://localhost:8081/api/queries/dbpolicy/get-by-cntmgt \
  -H "Content-Type: application/json" \
  -d '{
    "cntmgt_id": 1
  }'
```

### Create a Policy

```bash
curl -X POST http://localhost:8081/api/queries/dbpolicy \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Allow SELECT from Users",
    "cnt_mgt_id": 1,
    "db_mgt_id": 1,
    "db_actor_mgt_id": 5,
    "db_object_mgt_id": 10,
    "sql_allow": "SELECT",
    "sql_deny": ""
  }'
```

### Bulk Update Policies

```bash
curl -X POST http://localhost:8081/api/queries/dbpolicy/bulk-update \
  -H "Content-Type: application/json" \
  -d '{
    "policies": [
      {"id": 1, "sql_allow": "SELECT,INSERT"},
      {"id": 2, "sql_allow": "SELECT"}
    ]
  }'
```

---

## Troubleshooting

### Server fails to start

**Error:** "failed to connect to MySQL"

**Solution:**
1. Verify DB_HOST, DB_PORT, DB_USER, DB_PASS are correct
2. Check MySQL is running: `mysql -h localhost -u root -p -e "SELECT 1"`
3. Check network connectivity: `ping <DB_HOST>`

### API returns 500 error

**Solution:**
1. Check logs: `tail -f /var/log/dbf/dbfartifactapi.log`
2. Verify request format matches API documentation
3. Check database connectivity
4. Review error details in structured logs

### Job hangs during privilege discovery

**Solution:**
1. Check remote database connectivity
2. Verify database user has SELECT permissions on privilege tables
3. Increase AGENT_EXECUTION_TIMEOUT if discovery takes > 5 minutes
4. Check memory usage (discovery loads privilege data into memory)

### Privilege discovery returns incomplete results

**Possible causes:**
1. User doesn't have SELECT on all privilege tables
2. Large privilege set exceeds memory limits (>100k privileges)
3. Timeout during remote database load

**Solution:**
1. Grant SELECT permissions: `GRANT SELECT ON mysql.* TO user;`
2. Increase PRIVILEGE_LOAD_CONCURRENCY
3. Increase AGENT_EXECUTION_TIMEOUT

For more troubleshooting, see [Project Roadmap](./docs/project-roadmap.md).

---

## Documentation

### For New Developers
1. Start with [Project Overview](./docs/project-overview-pdr.md)
2. Read [System Architecture](./docs/system-architecture.md)
3. Review [Code Standards](./docs/code-standards.md)
4. Explore actual code in `services/` and `controllers/`

### For Operations/DevOps
1. [System Architecture](./docs/system-architecture.md) - Deployment patterns
2. Configuration reference above
3. Troubleshooting section in this README
4. Log file location and rotation settings

### For API Users
1. [Quick Start](#quick-links) in this README
2. Swagger documentation (http://localhost:8081/swagger)
3. [System Architecture](./docs/system-architecture.md) - API overview
4. Common tasks section above

### For Architects
1. [Project Overview](./docs/project-overview-pdr.md) - Vision and requirements
2. [System Architecture](./docs/system-architecture.md) - Component design
3. [Codebase Summary](./docs/codebase-summary.md) - Implementation patterns
4. [Project Roadmap](./docs/project-roadmap.md) - Future direction

---

## Deployment

### Docker Deployment

Build Docker image:

```bash
docker build -t dbf-artifact-api:latest .
```

Run container:

```bash
docker run -d \
  -e DB_HOST=mysql.example.com \
  -e DB_USER=dbf_user \
  -e DB_PASS=password \
  -e PORT=8081 \
  -p 8081:8081 \
  dbf-artifact-api:latest
```

### Kubernetes Deployment

Use Helm charts (when available):

```bash
helm install dbf-artifact-api ./helm/dbf-artifact-api \
  --set database.host=mysql.example.com \
  --set database.user=dbf_user
```

### Systemd Service

Create `/etc/systemd/system/dbfartifactapi.service`:

```ini
[Unit]
Description=DBF Artifact API
After=network.target

[Service]
Type=simple
User=dbf
WorkingDirectory=/opt/dbfartifactapi
ExecStart=/opt/dbfartifactapi/dbfartifactapi
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
systemctl enable dbfartifactapi
systemctl start dbfartifactapi
```

---

## Contributing

### Development Workflow

1. Create feature branch: `git checkout -b feature/description`
2. Make changes following [Code Standards](./docs/code-standards.md)
3. Write tests for new functionality
4. Run tests: `go test ./...`
5. Submit PR with description of changes

### Pull Request Checklist

- [ ] Code follows standards in CLAUDE.md
- [ ] All tests pass
- [ ] Tests cover new functionality
- [ ] No hardcoded credentials
- [ ] Errors wrapped with context
- [ ] Comments explain WHY, not WHAT
- [ ] No fmt.Print* calls (use structured logger)
- [ ] Documentation updated if needed

---

## Support & Contact

- **Issues:** Create issue in repository
- **Documentation:** See `./docs/` directory
- **Architecture Questions:** Refer to [System Architecture](./docs/system-architecture.md)
- **Code Standards Questions:** See [Code Standards](./docs/code-standards.md)

---

## License

Internal Use Only - DBF Project Team

---

## Version History

| Version | Date | Status | Notes |
|---------|------|--------|-------|
| 1.0.0 | 2026-02-24 | Active | Core features complete, privilege discovery working |
| 0.9.0 | 2026-01-15 | Previous | Policy engine complete |
| 0.5.0 | 2025-12-01 | Previous | Foundation phase |

---

## Quick Reference

### Health Check

```bash
curl -X GET http://localhost:8081/api/queries/dbmgt/all
```

### View Swagger Docs

```
http://localhost:8081/swagger/index.html
```

### View Logs

```bash
tail -f /var/log/dbf/dbfartifactapi.log
```

### Stop Server

```bash
kill <pid>  # Graceful shutdown with active job cleanup
```

---

**Last Updated:** 2026-02-24
**Maintained by:** DBF Architecture Team
**Repository:** [Internal]
