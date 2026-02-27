package mysql

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
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
	"dbfartifactapi/services/privilege"
	"dbfartifactapi/utils"
)

// NewPrivilegeSession creates temporary in-memory MySQL server for MySQL privilege analysis only.
// Returns ErrPortAllocation if no free port available.
// Returns ErrTableCreation if MySQL privilege table schema initialization fails.
// Returns ErrServerStart if MySQL server fails to start within timeout.
func NewPrivilegeSession(ctx context.Context, sessionID string) (*privilege.PrivilegeSession, error) {
	port, err := privilege.GetFreePort()
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
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 100*time.Millisecond)
			if err == nil {
				conn.Close()
				logger.Infof("Started temporary MySQL server on port %d for session %s", port, sessionID)
				return &privilege.PrivilegeSession{
					Server:    s,
					Engine:    engine,
					Provider:  provider,
					Port:      port,
					SessionID: sessionID,
					Cancel:    cancel,
				}, nil
			}
		}
	}
}

// LoadPrivilegeData loads privilege data from real MySQL database into temporary in-memory server.
// Executes SELECT queries on MySQL system tables to retrieve privilege information for specified actors and databases.
func LoadPrivilegeData(ps *privilege.PrivilegeSession, cmt *models.CntMgt, actors []models.DBActorMgt, databases []models.DBMgt, endpointRepo repository.EndpointRepository) error {
	actorPairs := []string{}
	for _, actor := range actors {
		actorPairs = append(actorPairs, fmt.Sprintf("('%s', '%s')",
			utils.EscapeSQL(actor.DBUser), utils.EscapeSQL(actor.IPAddress)))
	}
	actorFilter := strings.Join(actorPairs, ", ")

	dbNames := []string{}
	for _, db := range databases {
		dbNames = append(dbNames, fmt.Sprintf("'%s'", utils.EscapeSQL(db.DbName)))
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
		if err := loadTable(ps, tbl.tableName, tbl.query, cmt, ep); err != nil {
			logger.Warnf("Failed to load %s: %v", tbl.tableName, err)
		}
	}

	logger.Infof("Loaded privilege data into temporary server for session %s", ps.SessionID)
	return nil
}

// loadTable fetches data from real MySQL and inserts into temporary server
func loadTable(ps *privilege.PrivilegeSession, tableName, query string, cmt *models.CntMgt, ep *models.Endpoint) error {
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

	session := memory.NewSession(sql.NewBaseSession(), ps.Provider)
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
				values = append(values, fmt.Sprintf("'%v'", utils.EscapeSQL(fmt.Sprintf("%v", val))))
			}
		}

		insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			tableName,
			strings.Join(columns, ", "),
			strings.Join(values, ", "))

		_, _, _, err := ps.Engine.Query(ctx, insertSQL)
		if err != nil {
			logger.Warnf("Failed to insert into %s: %v", tableName, err)
			continue
		}
	}

	logger.Debugf("Loaded %d rows into %s", len(results), tableName)
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
