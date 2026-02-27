package services

import (
	"context"
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

	"dbfartifactapi/pkg/logger"
)

// PrivilegeSession represents a temporary in-memory MySQL server session for privilege analysis.
// Used by Oracle privilege subsystem. MySQL privilege analysis uses services/privilege/ package.
type PrivilegeSession struct {
	server    *server.Server
	engine    *sqle.Engine
	provider  *memory.DbProvider
	port      int
	sessionID string
	cancel    context.CancelFunc
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
	logger.Infof("Closed temporary server for session %s", s.sessionID)
	return nil
}

// ExecuteInDatabase executes SQL template against temporary server in specified database context.
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

// getFreePort finds an available TCP port.
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

// createOraclePrivilegeTables creates Oracle privilege tables with flexible schemas for in-memory privilege analysis.
// Uses TEXT type for all columns to avoid strict type checking and allow any Oracle privilege data.
// Tables: DBA_SYS_PRIVS, DBA_TAB_PRIVS, DBA_ROLE_PRIVS, V_PWFILE_USERS, CDB_SYS_PRIVS, DBA_TS_QUOTAS.
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
