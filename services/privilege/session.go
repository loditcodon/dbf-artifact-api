package privilege

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"

	sqle "github.com/dolthub/go-mysql-server"
	"github.com/dolthub/go-mysql-server/memory"
	"github.com/dolthub/go-mysql-server/server"
	"github.com/dolthub/go-mysql-server/sql"

	"dbfartifactapi/pkg/logger"
)

// PrivilegeSession represents a temporary in-memory MySQL server session for privilege analysis.
// Used by both MySQL and Oracle privilege discovery subsystems.
type PrivilegeSession struct {
	Server    *server.Server
	Engine    *sqle.Engine
	Provider  *memory.DbProvider
	Port      int
	SessionID string
	Cancel    context.CancelFunc
}

// Close shuts down the temporary MySQL server.
// Triggers context cancellation to cleanup background goroutines.
func (s *PrivilegeSession) Close() error {
	if s.Cancel != nil {
		s.Cancel()
	}
	if err := s.Server.Close(); err != nil {
		return fmt.Errorf("failed to close server: %w", err)
	}
	logger.Infof("Closed temporary MySQL server for session %s", s.SessionID)
	return nil
}

// ExecuteTemplate executes SQL template against temporary server in mysql database context.
func (s *PrivilegeSession) ExecuteTemplate(sqlTemplate string, variables map[string]string) ([]map[string]interface{}, error) {
	return s.ExecuteInDatabase(sqlTemplate, "mysql", variables)
}

// ExecuteInDatabase executes SQL template against temporary server in specified database context.
func (s *PrivilegeSession) ExecuteInDatabase(sqlTemplate string, database string, variables map[string]string) ([]map[string]interface{}, error) {
	finalSQL := sqlTemplate
	for varName, varValue := range variables {
		finalSQL = strings.ReplaceAll(finalSQL, "${"+varName+"}", varValue)
	}

	session := memory.NewSession(sql.NewBaseSession(), s.Provider)
	ctx := sql.NewContext(context.Background(), sql.WithSession(session))
	ctx.SetCurrentDatabase(database)

	schema, rowIter, _, err := s.Engine.Query(ctx, finalSQL)
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

// GetFreePort finds an available TCP port.
func GetFreePort() (int, error) {
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
