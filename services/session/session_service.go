package session

import (
	"context"
	"fmt"
	"strings"

	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services/agent"
	"dbfartifactapi/services/dto"
	"dbfartifactapi/utils"
)

// SessionService interface for session management operations
type SessionService interface {
	KillSession(ctx context.Context, cntmgtID uint, sessionID string) (string, error)
}

type sessionService struct {
	baseRepo     repository.BaseRepository
	cntmgtRepo   repository.CntMgtRepository
	endpointRepo repository.EndpointRepository
}

// NewSessionService creates a new session service instance
func NewSessionService() SessionService {
	return &sessionService{
		baseRepo:     repository.NewBaseRepository(),
		cntmgtRepo:   repository.NewCntMgtRepository(),
		endpointRepo: repository.NewEndpointRepository(),
	}
}

// KillSession executes kill session command on database
func (s *sessionService) KillSession(ctx context.Context, cntmgtID uint, sessionID string) (string, error) {
	logger.Infof("Starting kill session process for cntmgt_id=%d, session_id=%s", cntmgtID, sessionID)

	// Get connection management info
	cntMgt, err := s.cntmgtRepo.GetCntMgtByID(nil, cntmgtID)
	if err != nil {
		return "", fmt.Errorf("failed to get connection management with id=%d: %w", cntmgtID, err)
	}

	logger.Infof("Found connection management: id=%d, name=%s, type=%s", cntMgt.ID, cntMgt.CntName, cntMgt.CntType)

	// Get endpoint for this connection management
	endpoint, err := s.endpointRepo.GetByID(nil, utils.MustIntToUint(cntMgt.Agent))
	if err != nil {
		return "", fmt.Errorf("failed to get endpoint for agent=%d: %w", cntMgt.Agent, err)
	}

	logger.Infof("Found endpoint: id=%d, client_id=%s, os_type=%s", endpoint.ID, endpoint.ClientID, endpoint.OsType)

	// Generate kill session query based on database type
	killQuery, err := s.generateKillSessionQuery(cntMgt.CntType, sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to generate kill session query: %w", err)
	}

	logger.Infof("Generated kill session query for %s: %s", cntMgt.CntType, killQuery)

	// Build query parameters using existing DTO and builder
	queryParam := dto.NewDBQueryParamBuilder().
		SetDBType(cntMgt.CntType).
		SetHost(cntMgt.IP).
		SetPort(cntMgt.Port).
		SetUser(cntMgt.Username).
		SetPassword(cntMgt.Password).
		SetDatabase(""). // Empty for kill session command
		SetQuery(killQuery).
		SetAction("execute").
		Build()

	// Create agent command JSON
	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		return "", fmt.Errorf("failed to create agent command JSON: %w", err)
	}

	logger.Infof("Executing agent API command for kill session - endpoint: %s, os: %s", endpoint.ClientID, endpoint.OsType)

	// Execute agent API
	osType := strings.ToLower(endpoint.OsType)
	stdout, err := agent.ExecuteSqlAgentAPI(endpoint.ClientID, osType, "execute", hexJSON, "", false)
	if err != nil {
		logger.Errorf("Agent API execution failed: %v", err)
		return "", fmt.Errorf("agent API execution failed: %w", err)
	}

	logger.Infof("Kill session command executed successfully - stdout: %s", stdout)

	return fmt.Sprintf("Kill session command executed successfully for session %s on connection %s", sessionID, cntMgt.CntName), nil
}

// generateKillSessionQuery creates the appropriate kill session query based on database type
func (s *sessionService) generateKillSessionQuery(dbType, sessionID string) (string, error) {
	switch strings.ToLower(dbType) {
	case "mysql":
		// MySQL KILL statement
		if sessionID == "" {
			return "", fmt.Errorf("session ID cannot be empty")
		}
		return fmt.Sprintf("KILL %s", sessionID), nil

	case "postgresql", "postgres":
		// PostgreSQL terminate session
		if sessionID == "" {
			return "", fmt.Errorf("session ID cannot be empty")
		}
		return fmt.Sprintf("SELECT pg_terminate_backend(%s)", sessionID), nil

	case "oracle":
		// Oracle kill session (requires SID,SERIAL#)
		if sessionID == "" {
			return "", fmt.Errorf("session ID cannot be empty")
		}
		return fmt.Sprintf("ALTER SYSTEM KILL SESSION '%s'", sessionID), nil

	case "sqlserver", "mssql":
		// SQL Server KILL statement
		if sessionID == "" {
			return "", fmt.Errorf("session ID cannot be empty")
		}
		return fmt.Sprintf("KILL %s", sessionID), nil

	default:
		return "", fmt.Errorf("unsupported database type: %s", dbType)
	}
}
