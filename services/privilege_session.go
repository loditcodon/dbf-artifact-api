package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"

	sqle "github.com/dolthub/go-mysql-server"
	"github.com/dolthub/go-mysql-server/memory"
	"github.com/dolthub/go-mysql-server/server"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/types"

	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services/agent"
	"dbfartifactapi/services/dto"
	"dbfartifactapi/utils"
)

// PrivilegeSession represents a temporary in-memory MySQL server session for MySQL privilege analysis only.
type PrivilegeSession struct {
	server    *server.Server
	engine    *sqle.Engine
	provider  *memory.DbProvider
	port      int
	sessionID string
	cancel    context.CancelFunc
}

// NewPrivilegeSession creates temporary in-memory MySQL server for MySQL privilege analysis only.
// Returns ErrPortAllocation if no free port available.
// Returns ErrTableCreation if MySQL privilege table schema initialization fails.
// Returns ErrServerStart if MySQL server fails to start within timeout.
func NewPrivilegeSession(ctx context.Context, sessionID string) (*PrivilegeSession, error) {
	port, err := getFreePort()
	if err != nil {
		return nil, fmt.Errorf("failed to get free port: %w", err)
	}

	mysqlDB := memory.NewDatabase("mysql")
	infoSchemaDB := memory.NewDatabase("information_schema")

	provider := memory.NewDBProvider(mysqlDB, infoSchemaDB)
	engine := sqle.NewDefault(provider)

	session := memory.NewSession(sql.NewBaseSession(), provider)
	sqlCtx := sql.NewContext(ctx, sql.WithSession(session))
	sqlCtx.SetCurrentDatabase("mysql")

	if err := createPrivilegeTables(mysqlDB, infoSchemaDB); err != nil {
		return nil, fmt.Errorf("failed to create privilege tables: %w", err)
	}

	config := server.Config{
		Protocol: "tcp",
		Address:  fmt.Sprintf("localhost:%d", port),
	}

	s, err := server.NewServer(config, engine, sql.NewContext, memory.NewSessionBuilder(provider), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	serverCtx, cancel := context.WithCancel(ctx)

	// Goroutine cleanup strategy: cancel context triggers server shutdown
	go func() {
		if err := s.Start(); err != nil {
			logger.Errorf("Server error for session %s: %v", sessionID, err)
		}
	}()

	go func() {
		<-serverCtx.Done()
		if err := s.Close(); err != nil {
			logger.Warnf("Failed to close server for session %s: %v", sessionID, err)
		}
	}()

	// Poll server readiness with timeout to prevent indefinite blocking
	readyCtx, readyCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readyCancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-readyCtx.Done():
			cancel()
			return nil, fmt.Errorf("server failed to start within timeout for session %s: %w", sessionID, readyCtx.Err())
		case <-ticker.C:
			// Simple readiness check - try to get a connection
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 100*time.Millisecond)
			if err == nil {
				conn.Close()
				logger.Infof("Started temporary MySQL server on port %d for session %s", port, sessionID)
				return &PrivilegeSession{
					server:    s,
					engine:    engine,
					provider:  provider,
					port:      port,
					sessionID: sessionID,
					cancel:    cancel,
				}, nil
			}
		}
	}
}

// Close shuts down the temporary MySQL server.
// Triggers context cancellation to cleanup background goroutines.
func (s *PrivilegeSession) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	if err := s.server.Close(); err != nil {
		return fmt.Errorf("failed to close server: %w", err)
	}
	logger.Infof("Closed temporary MySQL server for session %s", s.sessionID)
	return nil
}

// createPrivilegeTables creates MySQL privilege tables with flexible schemas for in-memory privilege analysis.
// Uses TEXT type for all columns to avoid strict type checking and allow any MySQL privilege data.
// Column order must match MySQL privilege table SELECT queries for correct data loading.
// Tables created: mysql.user, mysql.db, mysql.tables_priv, mysql.procs_priv, mysql.role_edges,
// mysql.global_grants, mysql.proxies_priv, and mysql.infoschema_* equivalents.
func createPrivilegeTables(mysqlDB, infoSchemaDB *memory.Database) error {
	// mysql.user table - column order matches query: Host, User, Select_priv, Insert_priv, ...
	userSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "Host", Type: types.Text, Source: "user", Nullable: false, PrimaryKey: true},
		{Name: "User", Type: types.Text, Source: "user", Nullable: false, PrimaryKey: true},
		{Name: "Select_priv", Type: types.Text, Source: "user"},
		{Name: "Insert_priv", Type: types.Text, Source: "user"},
		{Name: "Update_priv", Type: types.Text, Source: "user"},
		{Name: "Delete_priv", Type: types.Text, Source: "user"},
		{Name: "Create_priv", Type: types.Text, Source: "user"},
		{Name: "Drop_priv", Type: types.Text, Source: "user"},
		{Name: "Reload_priv", Type: types.Text, Source: "user"},
		{Name: "Shutdown_priv", Type: types.Text, Source: "user"},
		{Name: "Process_priv", Type: types.Text, Source: "user"},
		{Name: "File_priv", Type: types.Text, Source: "user"},
		{Name: "Grant_priv", Type: types.Text, Source: "user"},
		{Name: "References_priv", Type: types.Text, Source: "user"},
		{Name: "Index_priv", Type: types.Text, Source: "user"},
		{Name: "Alter_priv", Type: types.Text, Source: "user"},
		{Name: "Show_db_priv", Type: types.Text, Source: "user"},
		{Name: "Super_priv", Type: types.Text, Source: "user"},
		{Name: "Create_tmp_table_priv", Type: types.Text, Source: "user"},
		{Name: "Lock_tables_priv", Type: types.Text, Source: "user"},
		{Name: "Execute_priv", Type: types.Text, Source: "user"},
		{Name: "Repl_slave_priv", Type: types.Text, Source: "user"},
		{Name: "Repl_client_priv", Type: types.Text, Source: "user"},
		{Name: "Create_view_priv", Type: types.Text, Source: "user"},
		{Name: "Show_view_priv", Type: types.Text, Source: "user"},
		{Name: "Create_routine_priv", Type: types.Text, Source: "user"},
		{Name: "Alter_routine_priv", Type: types.Text, Source: "user"},
		{Name: "Create_user_priv", Type: types.Text, Source: "user"},
		{Name: "Event_priv", Type: types.Text, Source: "user"},
		{Name: "Trigger_priv", Type: types.Text, Source: "user"},
		{Name: "Create_tablespace_priv", Type: types.Text, Source: "user"},
		// {Name: "Create_role_priv", Type: types.Text, Source: "user"},
		// {Name: "Resource_group_admin_priv", Type: types.Text, Source: "user"},
	})
	userTable := memory.NewTable(mysqlDB, "user", userSchema, mysqlDB.GetForeignKeyCollection())
	mysqlDB.AddTable("user", userTable)

	dbSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "Host", Type: types.Text, Source: "db", Nullable: false, PrimaryKey: true},
		{Name: "Db", Type: types.Text, Source: "db", Nullable: false, PrimaryKey: true},
		{Name: "User", Type: types.Text, Source: "db", Nullable: false, PrimaryKey: true},
		{Name: "Select_priv", Type: types.Text, Source: "db"},
		{Name: "Insert_priv", Type: types.Text, Source: "db"},
		{Name: "Update_priv", Type: types.Text, Source: "db"},
		{Name: "Delete_priv", Type: types.Text, Source: "db"},
		{Name: "Create_priv", Type: types.Text, Source: "db"},
		{Name: "Drop_priv", Type: types.Text, Source: "db"},
		{Name: "Grant_priv", Type: types.Text, Source: "db"},
		{Name: "References_priv", Type: types.Text, Source: "db"},
		{Name: "Index_priv", Type: types.Text, Source: "db"},
		{Name: "Alter_priv", Type: types.Text, Source: "db"},
		{Name: "Create_tmp_table_priv", Type: types.Text, Source: "db"},
		{Name: "Lock_tables_priv", Type: types.Text, Source: "db"},
		{Name: "Create_view_priv", Type: types.Text, Source: "db"},
		{Name: "Show_view_priv", Type: types.Text, Source: "db"},
		{Name: "Create_routine_priv", Type: types.Text, Source: "db"},
		{Name: "Alter_routine_priv", Type: types.Text, Source: "db"},
		{Name: "Execute_priv", Type: types.Text, Source: "db"},
		{Name: "Event_priv", Type: types.Text, Source: "db"},
		{Name: "Trigger_priv", Type: types.Text, Source: "db"},
	})
	dbTable := memory.NewTable(mysqlDB, "db", dbSchema, mysqlDB.GetForeignKeyCollection())
	mysqlDB.AddTable("db", dbTable)

	tablesPrivSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "Host", Type: types.Text, Source: "tables_priv", Nullable: false, PrimaryKey: true},
		{Name: "Db", Type: types.Text, Source: "tables_priv", Nullable: false, PrimaryKey: true},
		{Name: "User", Type: types.Text, Source: "tables_priv", Nullable: false, PrimaryKey: true},
		{Name: "Table_name", Type: types.Text, Source: "tables_priv", Nullable: false, PrimaryKey: true},
		{Name: "Grantor", Type: types.Text, Source: "tables_priv"},
		{Name: "Timestamp", Type: types.Text, Source: "tables_priv"},
		{Name: "Table_priv", Type: types.Text, Source: "tables_priv"},
		{Name: "Column_priv", Type: types.Text, Source: "tables_priv"},
	})
	tablesPrivTable := memory.NewTable(mysqlDB, "tables_priv", tablesPrivSchema, mysqlDB.GetForeignKeyCollection())
	mysqlDB.AddTable("tables_priv", tablesPrivTable)

	procsPrivSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "Host", Type: types.Text, Source: "procs_priv", Nullable: false, PrimaryKey: true},
		{Name: "Db", Type: types.Text, Source: "procs_priv", Nullable: false, PrimaryKey: true},
		{Name: "User", Type: types.Text, Source: "procs_priv", Nullable: false, PrimaryKey: true},
		{Name: "Routine_name", Type: types.Text, Source: "procs_priv", Nullable: false, PrimaryKey: true},
		{Name: "Routine_type", Type: types.Text, Source: "procs_priv", Nullable: false, PrimaryKey: true},
		{Name: "Grantor", Type: types.Text, Source: "procs_priv"},
		{Name: "Timestamp", Type: types.Text, Source: "procs_priv"},
		{Name: "Proc_priv", Type: types.Text, Source: "procs_priv"},
	})
	procsPrivTable := memory.NewTable(mysqlDB, "procs_priv", procsPrivSchema, mysqlDB.GetForeignKeyCollection())
	mysqlDB.AddTable("procs_priv", procsPrivTable)

	roleEdgesSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "FROM_HOST", Type: types.Text, Source: "role_edges", Nullable: false, PrimaryKey: true},
		{Name: "FROM_USER", Type: types.Text, Source: "role_edges", Nullable: false, PrimaryKey: true},
		{Name: "TO_HOST", Type: types.Text, Source: "role_edges", Nullable: false, PrimaryKey: true},
		{Name: "TO_USER", Type: types.Text, Source: "role_edges", Nullable: false, PrimaryKey: true},
		{Name: "WITH_ADMIN_OPTION", Type: types.Text, Source: "role_edges"},
	})
	roleEdgesTable := memory.NewTable(mysqlDB, "role_edges", roleEdgesSchema, mysqlDB.GetForeignKeyCollection())
	mysqlDB.AddTable("role_edges", roleEdgesTable)

	globalGrantsSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "USER", Type: types.Text, Source: "global_grants", Nullable: false, PrimaryKey: true},
		{Name: "HOST", Type: types.Text, Source: "global_grants", Nullable: false, PrimaryKey: true},
		{Name: "PRIV", Type: types.Text, Source: "global_grants", Nullable: false, PrimaryKey: true},
		{Name: "WITH_GRANT_OPTION", Type: types.Text, Source: "global_grants"},
	})
	globalGrantsTable := memory.NewTable(mysqlDB, "global_grants", globalGrantsSchema, mysqlDB.GetForeignKeyCollection())
	mysqlDB.AddTable("global_grants", globalGrantsTable)

	proxiesPrivSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "Host", Type: types.Text, Source: "proxies_priv", Nullable: false, PrimaryKey: true},
		{Name: "User", Type: types.Text, Source: "proxies_priv", Nullable: false, PrimaryKey: true},
		{Name: "Proxied_host", Type: types.Text, Source: "proxies_priv", Nullable: false, PrimaryKey: true},
		{Name: "Proxied_user", Type: types.Text, Source: "proxies_priv", Nullable: false, PrimaryKey: true},
		{Name: "With_grant", Type: types.Text, Source: "proxies_priv"},
		{Name: "Grantor", Type: types.Text, Source: "proxies_priv"},
		{Name: "Timestamp", Type: types.Text, Source: "proxies_priv"},
	})
	proxiesPrivTable := memory.NewTable(mysqlDB, "proxies_priv", proxiesPrivSchema, mysqlDB.GetForeignKeyCollection())
	mysqlDB.AddTable("proxies_priv", proxiesPrivTable)

	// Create copies of information_schema tables in mysql database with prefix infoschema_
	// This is necessary because go-mysql-server doesn't allow INSERT into information_schema
	userPrivilegesSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "GRANTEE", Type: types.Text, Source: "infoschema_user_privileges", Nullable: false},
		{Name: "TABLE_CATALOG", Type: types.Text, Source: "infoschema_user_privileges"},
		{Name: "PRIVILEGE_TYPE", Type: types.Text, Source: "infoschema_user_privileges"},
		{Name: "IS_GRANTABLE", Type: types.Text, Source: "infoschema_user_privileges"},
	})
	userPrivilegesTable := memory.NewTable(mysqlDB, "infoschema_user_privileges", userPrivilegesSchema, mysqlDB.GetForeignKeyCollection())
	mysqlDB.AddTable("infoschema_user_privileges", userPrivilegesTable)

	schemaPrivilegesSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "GRANTEE", Type: types.Text, Source: "infoschema_schema_privileges", Nullable: false},
		{Name: "TABLE_CATALOG", Type: types.Text, Source: "infoschema_schema_privileges"},
		{Name: "TABLE_SCHEMA", Type: types.Text, Source: "infoschema_schema_privileges"},
		{Name: "PRIVILEGE_TYPE", Type: types.Text, Source: "infoschema_schema_privileges"},
		{Name: "IS_GRANTABLE", Type: types.Text, Source: "infoschema_schema_privileges"},
	})
	schemaPrivilegesTable := memory.NewTable(mysqlDB, "infoschema_schema_privileges", schemaPrivilegesSchema, mysqlDB.GetForeignKeyCollection())
	mysqlDB.AddTable("infoschema_schema_privileges", schemaPrivilegesTable)

	tablePrivilegesSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "GRANTEE", Type: types.Text, Source: "infoschema_table_privileges", Nullable: false},
		{Name: "TABLE_CATALOG", Type: types.Text, Source: "infoschema_table_privileges"},
		{Name: "TABLE_SCHEMA", Type: types.Text, Source: "infoschema_table_privileges"},
		{Name: "TABLE_NAME", Type: types.Text, Source: "infoschema_table_privileges"},
		{Name: "PRIVILEGE_TYPE", Type: types.Text, Source: "infoschema_table_privileges"},
		{Name: "IS_GRANTABLE", Type: types.Text, Source: "infoschema_table_privileges"},
	})
	tablePrivilegesTable := memory.NewTable(mysqlDB, "infoschema_table_privileges", tablePrivilegesSchema, mysqlDB.GetForeignKeyCollection())
	mysqlDB.AddTable("infoschema_table_privileges", tablePrivilegesTable)

	logger.Infof("Created all MySQL privilege tables with flexible TEXT schema")
	return nil
}

// createOraclePrivilegeTables creates Oracle privilege tables in go-mysql-server.
// Uses same approach as MySQL - creates tables that mirror Oracle system views.
// Tables: DBA_SYS_PRIVS, DBA_TAB_PRIVS, DBA_ROLE_PRIVS, V_PWFILE_USERS, CDB_SYS_PRIVS.
func createOraclePrivilegeTables(oracleDB *memory.Database) error {
	// DBA_SYS_PRIVS - System privileges
	dbaSysPrivsSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "GRANTEE", Type: types.Text, Source: "DBA_SYS_PRIVS", Nullable: false, PrimaryKey: true},
		{Name: "PRIVILEGE", Type: types.Text, Source: "DBA_SYS_PRIVS", Nullable: false, PrimaryKey: true},
		{Name: "ADMIN_OPTION", Type: types.Text, Source: "DBA_SYS_PRIVS"},
		{Name: "COMMON", Type: types.Text, Source: "DBA_SYS_PRIVS"},
		{Name: "INHERITED", Type: types.Text, Source: "DBA_SYS_PRIVS"},
	})
	dbaSysPrivsTable := memory.NewTable(oracleDB, "DBA_SYS_PRIVS", dbaSysPrivsSchema, oracleDB.GetForeignKeyCollection())
	oracleDB.AddTable("DBA_SYS_PRIVS", dbaSysPrivsTable)

	// DBA_TAB_PRIVS - Object privileges
	dbaTabPrivsSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "GRANTEE", Type: types.Text, Source: "DBA_TAB_PRIVS", Nullable: false, PrimaryKey: true},
		{Name: "OWNER", Type: types.Text, Source: "DBA_TAB_PRIVS", Nullable: false, PrimaryKey: true},
		{Name: "TABLE_NAME", Type: types.Text, Source: "DBA_TAB_PRIVS", Nullable: false, PrimaryKey: true},
		{Name: "GRANTOR", Type: types.Text, Source: "DBA_TAB_PRIVS"},
		{Name: "PRIVILEGE", Type: types.Text, Source: "DBA_TAB_PRIVS", Nullable: false, PrimaryKey: true},
		{Name: "GRANTABLE", Type: types.Text, Source: "DBA_TAB_PRIVS"},
		{Name: "HIERARCHY", Type: types.Text, Source: "DBA_TAB_PRIVS"},
		{Name: "COMMON", Type: types.Text, Source: "DBA_TAB_PRIVS"},
		{Name: "TYPE", Type: types.Text, Source: "DBA_TAB_PRIVS"},
		{Name: "INHERITED", Type: types.Text, Source: "DBA_TAB_PRIVS"},
	})
	dbaTabPrivsTable := memory.NewTable(oracleDB, "DBA_TAB_PRIVS", dbaTabPrivsSchema, oracleDB.GetForeignKeyCollection())
	oracleDB.AddTable("DBA_TAB_PRIVS", dbaTabPrivsTable)

	// DBA_ROLE_PRIVS - Role grants
	dbaRolePrivsSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "GRANTEE", Type: types.Text, Source: "DBA_ROLE_PRIVS", Nullable: false, PrimaryKey: true},
		{Name: "GRANTED_ROLE", Type: types.Text, Source: "DBA_ROLE_PRIVS", Nullable: false, PrimaryKey: true},
		{Name: "ADMIN_OPTION", Type: types.Text, Source: "DBA_ROLE_PRIVS"},
		{Name: "DELEGATE_OPTION", Type: types.Text, Source: "DBA_ROLE_PRIVS"},
		{Name: "DEFAULT_ROLE", Type: types.Text, Source: "DBA_ROLE_PRIVS"},
		{Name: "COMMON", Type: types.Text, Source: "DBA_ROLE_PRIVS"},
		{Name: "INHERITED", Type: types.Text, Source: "DBA_ROLE_PRIVS"},
	})
	dbaRolePrivsTable := memory.NewTable(oracleDB, "DBA_ROLE_PRIVS", dbaRolePrivsSchema, oracleDB.GetForeignKeyCollection())
	oracleDB.AddTable("DBA_ROLE_PRIVS", dbaRolePrivsTable)

	// V_PWFILE_USERS - Password file users (V$PWFILE_USERS renamed because $ not allowed in table name)
	vPwfileUsersSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "USERNAME", Type: types.Text, Source: "V_PWFILE_USERS", Nullable: false, PrimaryKey: true},
		{Name: "SYSDBA", Type: types.Text, Source: "V_PWFILE_USERS"},
		{Name: "SYSOPER", Type: types.Text, Source: "V_PWFILE_USERS"},
		{Name: "SYSASM", Type: types.Text, Source: "V_PWFILE_USERS"},
		{Name: "SYSBACKUP", Type: types.Text, Source: "V_PWFILE_USERS"},
		{Name: "SYSDG", Type: types.Text, Source: "V_PWFILE_USERS"},
		{Name: "SYSKM", Type: types.Text, Source: "V_PWFILE_USERS"},
	})
	vPwfileUsersTable := memory.NewTable(oracleDB, "V_PWFILE_USERS", vPwfileUsersSchema, oracleDB.GetForeignKeyCollection())
	oracleDB.AddTable("V_PWFILE_USERS", vPwfileUsersTable)

	// CDB_SYS_PRIVS - CDB-wide system privileges (for CDB connections)
	cdbSysPrivsSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "GRANTEE", Type: types.Text, Source: "CDB_SYS_PRIVS", Nullable: false, PrimaryKey: true},
		{Name: "PRIVILEGE", Type: types.Text, Source: "CDB_SYS_PRIVS", Nullable: false, PrimaryKey: true},
		{Name: "ADMIN_OPTION", Type: types.Text, Source: "CDB_SYS_PRIVS"},
		{Name: "COMMON", Type: types.Text, Source: "CDB_SYS_PRIVS"},
		{Name: "INHERITED", Type: types.Text, Source: "CDB_SYS_PRIVS"},
		{Name: "CON_ID", Type: types.Text, Source: "CDB_SYS_PRIVS", Nullable: false, PrimaryKey: true},
	})
	cdbSysPrivsTable := memory.NewTable(oracleDB, "CDB_SYS_PRIVS", cdbSysPrivsSchema, oracleDB.GetForeignKeyCollection())
	oracleDB.AddTable("CDB_SYS_PRIVS", cdbSysPrivsTable)

	// DBA_TS_QUOTAS - Tablespace quotas for users
	dbaTsQuotasSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "TABLESPACE_NAME", Type: types.Text, Source: "DBA_TS_QUOTAS", Nullable: false, PrimaryKey: true},
		{Name: "USERNAME", Type: types.Text, Source: "DBA_TS_QUOTAS", Nullable: false, PrimaryKey: true},
		{Name: "BYTES", Type: types.Text, Source: "DBA_TS_QUOTAS"},
		{Name: "MAX_BYTES", Type: types.Text, Source: "DBA_TS_QUOTAS"},
		{Name: "BLOCKS", Type: types.Text, Source: "DBA_TS_QUOTAS"},
		{Name: "MAX_BLOCKS", Type: types.Text, Source: "DBA_TS_QUOTAS"},
		{Name: "DROPPED", Type: types.Text, Source: "DBA_TS_QUOTAS"},
	})
	dbaTsQuotasTable := memory.NewTable(oracleDB, "DBA_TS_QUOTAS", dbaTsQuotasSchema, oracleDB.GetForeignKeyCollection())
	oracleDB.AddTable("DBA_TS_QUOTAS", dbaTsQuotasTable)

	logger.Infof("Created all Oracle privilege tables with flexible TEXT schema")
	return nil
}

// NewOraclePrivilegeSession creates temporary in-memory server for Oracle privilege analysis.
// Uses same go-mysql-server but with Oracle privilege table schemas.
func NewOraclePrivilegeSession(ctx context.Context, sessionID string) (*PrivilegeSession, error) {
	port, err := getFreePort()
	if err != nil {
		return nil, fmt.Errorf("failed to get free port: %w", err)
	}

	// Create "oracle" database for Oracle privilege tables
	oracleDB := memory.NewDatabase("oracle")

	provider := memory.NewDBProvider(oracleDB)
	engine := sqle.NewDefault(provider)

	session := memory.NewSession(sql.NewBaseSession(), provider)
	sqlCtx := sql.NewContext(ctx, sql.WithSession(session))
	sqlCtx.SetCurrentDatabase("oracle")

	if err := createOraclePrivilegeTables(oracleDB); err != nil {
		return nil, fmt.Errorf("failed to create oracle privilege tables: %w", err)
	}

	config := server.Config{
		Protocol: "tcp",
		Address:  fmt.Sprintf("localhost:%d", port),
	}

	s, err := server.NewServer(config, engine, sql.NewContext, memory.NewSessionBuilder(provider), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	serverCtx, cancel := context.WithCancel(ctx)

	go func() {
		if err := s.Start(); err != nil {
			logger.Errorf("Oracle server error for session %s: %v", sessionID, err)
		}
	}()

	go func() {
		<-serverCtx.Done()
		if err := s.Close(); err != nil {
			logger.Warnf("Failed to close oracle server for session %s: %v", sessionID, err)
		}
	}()

	readyCtx, readyCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readyCancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-readyCtx.Done():
			cancel()
			return nil, fmt.Errorf("oracle server failed to start within timeout for session %s: %w", sessionID, readyCtx.Err())
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 100*time.Millisecond)
			if err == nil {
				conn.Close()
				logger.Infof("Started temporary Oracle server on port %d for session %s", port, sessionID)
				return &PrivilegeSession{
					server:    s,
					engine:    engine,
					provider:  provider,
					port:      port,
					sessionID: sessionID,
					cancel:    cancel,
				}, nil
			}
		}
	}
}

// LoadOraclePrivilegeDataFromResults loads parsed Oracle privilege data into in-memory tables.
// Data comes from QueryResult parsed by oracle_privilege_session_handler.
func (s *PrivilegeSession) LoadOraclePrivilegeDataFromResults(privilegeData *OraclePrivilegeData) error {
	session := memory.NewSession(sql.NewBaseSession(), s.provider)
	ctx := sql.NewContext(context.Background(), sql.WithSession(session))
	ctx.SetCurrentDatabase("oracle")

	// Load DBA_SYS_PRIVS
	for _, priv := range privilegeData.SysPrivs {
		insertSQL := fmt.Sprintf(
			"INSERT INTO DBA_SYS_PRIVS (GRANTEE, PRIVILEGE, ADMIN_OPTION, COMMON, INHERITED) VALUES ('%s', '%s', '%s', '%s', '%s')",
			escapeSQL(priv.Grantee), escapeSQL(priv.Privilege), escapeSQL(priv.AdminOption),
			escapeSQL(priv.Common), escapeSQL(priv.Inherited))

		if _, _, _, err := s.engine.Query(ctx, insertSQL); err != nil {
			logger.Warnf("Failed to insert into DBA_SYS_PRIVS: %v", err)
		}
	}
	logger.Debugf("Loaded %d rows into DBA_SYS_PRIVS", len(privilegeData.SysPrivs))

	// Load DBA_TAB_PRIVS
	for _, priv := range privilegeData.TabPrivs {
		insertSQL := fmt.Sprintf(
			"INSERT INTO DBA_TAB_PRIVS (GRANTEE, OWNER, TABLE_NAME, GRANTOR, PRIVILEGE, GRANTABLE, HIERARCHY, COMMON, TYPE, INHERITED) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s')",
			escapeSQL(priv.Grantee), escapeSQL(priv.Owner), escapeSQL(priv.TableName),
			escapeSQL(priv.Grantor), escapeSQL(priv.Privilege), escapeSQL(priv.Grantable),
			escapeSQL(priv.Hierarchy), escapeSQL(priv.Common), escapeSQL(priv.Type), escapeSQL(priv.Inherited))

		if _, _, _, err := s.engine.Query(ctx, insertSQL); err != nil {
			logger.Warnf("Failed to insert into DBA_TAB_PRIVS: %v", err)
		}
	}
	logger.Debugf("Loaded %d rows into DBA_TAB_PRIVS", len(privilegeData.TabPrivs))

	// Load DBA_ROLE_PRIVS
	for _, priv := range privilegeData.RolePrivs {
		insertSQL := fmt.Sprintf(
			"INSERT INTO DBA_ROLE_PRIVS (GRANTEE, GRANTED_ROLE, ADMIN_OPTION, DELEGATE_OPTION, DEFAULT_ROLE, COMMON, INHERITED) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s')",
			escapeSQL(priv.Grantee), escapeSQL(priv.GrantedRole), escapeSQL(priv.AdminOption),
			escapeSQL(priv.DelegateOpt), escapeSQL(priv.DefaultRole), escapeSQL(priv.Common), escapeSQL(priv.Inherited))

		if _, _, _, err := s.engine.Query(ctx, insertSQL); err != nil {
			logger.Warnf("Failed to insert into DBA_ROLE_PRIVS: %v", err)
		}
	}
	logger.Debugf("Loaded %d rows into DBA_ROLE_PRIVS", len(privilegeData.RolePrivs))

	// Load V_PWFILE_USERS
	for _, user := range privilegeData.PwFileUsers {
		insertSQL := fmt.Sprintf(
			"INSERT INTO V_PWFILE_USERS (USERNAME, SYSDBA, SYSOPER, SYSASM, SYSBACKUP, SYSDG, SYSKM) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s')",
			escapeSQL(user.Username), escapeSQL(user.Sysdba), escapeSQL(user.Sysoper),
			escapeSQL(user.Sysasm), escapeSQL(user.Sysbackup), escapeSQL(user.Sysdg), escapeSQL(user.Syskm))

		if _, _, _, err := s.engine.Query(ctx, insertSQL); err != nil {
			logger.Warnf("Failed to insert into V_PWFILE_USERS: %v", err)
		}
	}
	logger.Debugf("Loaded %d rows into V_PWFILE_USERS", len(privilegeData.PwFileUsers))

	// Load CDB_SYS_PRIVS
	for _, priv := range privilegeData.CdbSysPrivs {
		insertSQL := fmt.Sprintf(
			"INSERT INTO CDB_SYS_PRIVS (GRANTEE, PRIVILEGE, ADMIN_OPTION, COMMON, INHERITED, CON_ID) VALUES ('%s', '%s', '%s', '%s', '%s', '%d')",
			escapeSQL(priv.Grantee), escapeSQL(priv.Privilege), escapeSQL(priv.AdminOption),
			escapeSQL(priv.Common), escapeSQL(priv.Inherited), priv.ConID)

		if _, _, _, err := s.engine.Query(ctx, insertSQL); err != nil {
			logger.Warnf("Failed to insert into CDB_SYS_PRIVS: %v", err)
		}
	}
	logger.Debugf("Loaded %d rows into CDB_SYS_PRIVS", len(privilegeData.CdbSysPrivs))

	logger.Infof("Loaded Oracle privilege data into temporary server for session %s", s.sessionID)
	return nil
}

// ExecuteOracleTemplate executes SQL template against Oracle privilege tables.
// Replaces V$* dynamic performance views with V_* since $ not allowed in table/view names.
func (s *PrivilegeSession) ExecuteOracleTemplate(sqlTemplate string, variables map[string]string) ([]map[string]interface{}, error) {
	// Replace Oracle V$ views with V_ equivalents ($ not allowed in go-mysql-server identifiers)
	finalSQL := replaceOracleDollarViews(sqlTemplate)

	return s.ExecuteInDatabase(finalSQL, "oracle", variables)
}

// replaceOracleDollarViews replaces Oracle V$* and GV$* dynamic performance views with V_* and GV_* equivalents.
// go-mysql-server doesn't support $ in identifiers, so we rename these views.
// Uses case-insensitive regex to catch ALL V$* and GV$* patterns (both v$pwfile_users and V$PWFILE_USERS).
// Also replaces other Oracle internal tables with $ like X$*.
func replaceOracleDollarViews(sqlStr string) string {
	// Pattern 1: V$VIEW_NAME -> V_VIEW_NAME (case-insensitive)
	// Matches v$ or V$ followed by alphanumeric characters and underscores
	vDollarRegex := regexp.MustCompile(`(?i)\bV\$([A-Za-z0-9_]+)`)
	result := vDollarRegex.ReplaceAllString(sqlStr, "V_$1")

	// Pattern 2: GV$VIEW_NAME -> GV_VIEW_NAME (case-insensitive)
	gvDollarRegex := regexp.MustCompile(`(?i)\bGV\$([A-Za-z0-9_]+)`)
	result = gvDollarRegex.ReplaceAllString(result, "GV_$1")

	// Pattern 3: X$TABLE_NAME -> X_TABLE_NAME (case-insensitive)
	xDollarRegex := regexp.MustCompile(`(?i)\bX\$([A-Za-z0-9_]+)`)
	result = xDollarRegex.ReplaceAllString(result, "X_$1")

	return result
}

// LoadPrivilegeData loads privilege data from real MySQL database into temporary in-memory server.
// Executes SELECT queries on MySQL system tables to retrieve privilege information for specified actors and databases.
// Queries: mysql.user, mysql.db, mysql.tables_priv, mysql.procs_priv, mysql.role_edges, mysql.global_grants, mysql.proxies_priv.
func (s *PrivilegeSession) LoadPrivilegeData(cmt *models.CntMgt, actors []models.DBActorMgt, databases []models.DBMgt, endpointRepo repository.EndpointRepository) error {
	actorPairs := []string{}
	for _, actor := range actors {
		actorPairs = append(actorPairs, fmt.Sprintf("('%s', '%s')",
			escapeSQL(actor.DBUser), escapeSQL(actor.IPAddress)))
	}
	actorFilter := strings.Join(actorPairs, ", ")

	dbNames := []string{}
	for _, db := range databases {
		dbNames = append(dbNames, fmt.Sprintf("'%s'", escapeSQL(db.DbName)))
	}
	dbFilter := strings.Join(dbNames, ", ")

	ep, err := endpointRepo.GetByID(nil, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		return fmt.Errorf("failed to get endpoint: %w", err)
	}

	tables := []struct {
		tableName string
		query     string
	}{
		{
			tableName: "mysql.user",
			query:     fmt.Sprintf("SELECT * FROM mysql.user WHERE (user, host) IN (%s)", actorFilter),
		},
		{
			tableName: "mysql.db",
			query:     fmt.Sprintf("SELECT * FROM mysql.db WHERE (user, host) IN (%s)", actorFilter),
		},
		{
			tableName: "mysql.tables_priv",
			query:     fmt.Sprintf("SELECT * FROM mysql.tables_priv WHERE (user, host) IN (%s) AND db IN (%s)", actorFilter, dbFilter),
		},
		{
			tableName: "mysql.procs_priv",
			query:     fmt.Sprintf("SELECT * FROM mysql.procs_priv WHERE (user, host) IN (%s) AND db IN (%s)", actorFilter, dbFilter),
		},
		{
			tableName: "mysql.role_edges",
			query:     fmt.Sprintf("SELECT * FROM mysql.role_edges WHERE (TO_USER, TO_HOST) IN (%s)", actorFilter),
		},
		{
			tableName: "mysql.global_grants",
			query:     fmt.Sprintf("SELECT * FROM mysql.global_grants WHERE (USER, HOST) IN (%s)", actorFilter),
		},
		{
			tableName: "mysql.proxies_priv",
			query:     fmt.Sprintf("SELECT * FROM mysql.proxies_priv WHERE (user, host) IN (%s)", actorFilter),
		},
	}

	for _, tbl := range tables {
		if err := s.loadTable(tbl.tableName, tbl.query, cmt, ep); err != nil {
			logger.Warnf("Failed to load %s: %v", tbl.tableName, err)
		}
	}

	logger.Infof("Loaded privilege data into temporary server for session %s", s.sessionID)
	return nil
}

// loadTable fetches data from real MySQL and inserts into temporary server
func (s *PrivilegeSession) loadTable(tableName, query string, cmt *models.CntMgt, ep *models.Endpoint) error {
	queryParam := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cmt.CntType)).
		SetHost(cmt.IP).
		SetPort(cmt.Port).
		SetUser(cmt.Username).
		SetPassword(cmt.Password).
		Build()
	queryParam.Query = query

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		return fmt.Errorf("failed to create agent command JSON: %w", err)
	}

	stdout, err := agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", false)
	if err != nil {
		return fmt.Errorf("failed to execute query: %w", err)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &results); err != nil {
		return fmt.Errorf("failed to parse results: %w", err)
	}

	if len(results) == 0 {
		logger.Debugf("No data for %s", tableName)
		return nil
	}

	session := memory.NewSession(sql.NewBaseSession(), s.provider)
	ctx := sql.NewContext(context.Background(), sql.WithSession(session))
	ctx.SetCurrentDatabase("mysql")

	for _, row := range results {
		columns := []string{}
		values := []string{}

		for col, val := range row {
			columns = append(columns, col)
			if val == nil {
				values = append(values, "NULL")
			} else {
				values = append(values, fmt.Sprintf("'%v'", escapeSQL(fmt.Sprintf("%v", val))))
			}
		}

		insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			tableName,
			strings.Join(columns, ", "),
			strings.Join(values, ", "))

		_, _, _, err := s.engine.Query(ctx, insertSQL)
		if err != nil {
			logger.Warnf("Failed to insert into %s: %v", tableName, err)
			continue
		}
	}

	logger.Debugf("Loaded %d rows into %s", len(results), tableName)
	return nil
}

// ExecuteTemplate executes SQL template against temporary server
func (s *PrivilegeSession) ExecuteTemplate(sqlTemplate string, variables map[string]string) ([]map[string]interface{}, error) {
	return s.ExecuteInDatabase(sqlTemplate, "mysql", variables)
}

// ExecuteInDatabase executes SQL template against temporary server in specified database context
func (s *PrivilegeSession) ExecuteInDatabase(sqlTemplate string, database string, variables map[string]string) ([]map[string]interface{}, error) {
	finalSQL := sqlTemplate
	for varName, varValue := range variables {
		finalSQL = strings.ReplaceAll(finalSQL, "${"+varName+"}", varValue)
	}

	session := memory.NewSession(sql.NewBaseSession(), s.provider)
	ctx := sql.NewContext(context.Background(), sql.WithSession(session))
	ctx.SetCurrentDatabase(database)

	schema, rowIter, _, err := s.engine.Query(ctx, finalSQL)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	defer rowIter.Close(ctx)

	results := []map[string]interface{}{}

	for {
		row, err := rowIter.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to fetch row: %w", err)
		}

		rowMap := make(map[string]interface{})
		for i, col := range schema {
			rowMap[col.Name] = row[i]
		}

		results = append(results, rowMap)
	}

	return results, nil
}

// getFreePort finds an available TCP port
func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port, nil
}
