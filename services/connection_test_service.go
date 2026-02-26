package services

import (
	"context"
	"fmt"
	"strings"

	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services/agent"
	"dbfartifactapi/utils"
)

// ConnectionTestResponse represents the response from VeloArtifact command
type ConnectionTestResponse struct {
	Complete   bool   `json:"Complete"`
	ReturnCode int    `json:"ReturnCode"`
	Stderr     string `json:"Stderr"`
	Stdout     string `json:"Stdout"`
}

// ConnectionStatus represents the parsed connection status from stdout
type ConnectionStatus struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Response string `json:"response"`
}

// ConnectionTestService interface defines connection testing operations
type ConnectionTestService interface {
	TestConnection(ctx context.Context, id uint) (string, error)
}

type connectionTestService struct {
	cntMgtRepo   repository.CntMgtRepository
	endpointRepo repository.EndpointRepository
}

// NewConnectionTestService creates a new connection test service instance
func NewConnectionTestService() ConnectionTestService {
	return &connectionTestService{
		cntMgtRepo:   repository.NewCntMgtRepository(),
		endpointRepo: repository.NewEndpointRepository(),
	}
}

// TestConnection tests database connection and updates status in database.
// Returns connection test result message from VeloArtifact.
func (s *connectionTestService) TestConnection(ctx context.Context, id uint) (string, error) {
	// Input validation at service boundary
	if ctx == nil {
		return "", fmt.Errorf("context cannot be nil")
	}
	if id == 0 {
		return "", fmt.Errorf("invalid connection ID: must be greater than 0")
	}

	// Get connection management record
	cntMgt, err := s.cntMgtRepo.GetCntMgtByID(nil, id)
	if err != nil {
		return "", fmt.Errorf("connection with id=%d not found: %v", id, err)
	}

	logger.Infof("Testing connection: id=%d, type=%s, host=%s:%d, service_name=%s",
		id, cntMgt.CntType, cntMgt.IP, cntMgt.Port, cntMgt.ServiceName)

	// Get endpoint information for VeloArtifact execution
	endpoint, err := s.endpointRepo.GetByID(nil, utils.MustIntToUint(cntMgt.Agent))
	if err != nil {
		return "", fmt.Errorf("endpoint with id=%d not found: %v", cntMgt.Agent, err)
	}

	logger.Infof("Found endpoint: client_id=%s, os_type=%s", endpoint.ClientID, endpoint.OsType)

	// Prepare connection test parameters
	testParams := agent.ConnectionTestAgentParams{
		Action:      "test_connection",
		Type:        strings.ToLower(cntMgt.CntType),
		Host:        cntMgt.IP,
		Port:        cntMgt.Port,
		Username:    cntMgt.Username,
		Password:    cntMgt.Password,
		ServiceName: cntMgt.ServiceName,
	}

	// Execute connection test via agent API
	agentResponse, err := agent.ExecuteConnectionTestAgentAPI(endpoint.ClientID, testParams, endpoint.OsType)
	if err != nil {
		logger.Errorf("Connection test execution failed: %v", err)
		return "", fmt.Errorf("failed to execute connection test: %v", err)
	}

	logger.Debugf("Agent API response: Status=%s, ExitCode=%d, Output=%s",
		agentResponse.Status, agentResponse.ExitCode, agentResponse.Output)

	// Determine connection status based on response
	var connectionStatus string
	var resultMessage string

	if agentResponse.Status == "success" && agentResponse.ExitCode == 0 && agentResponse.Output != "" {
		// Parse output using helper function from veloartifact_service
		connectionResults, err := parseConnectionTestResult(agentResponse.Output)
		if err == nil && len(connectionResults) > 0 {
			if connectionResults[0].Status == "running" &&
				strings.Contains(connectionResults[0].Response, "connected successfully") {
				connectionStatus = "enabled"
				resultMessage = agentResponse.Output
				logger.Infof("Connection test successful for id=%d", id)
			} else {
				connectionStatus = "disabled"
				resultMessage = agentResponse.Output
				logger.Warnf("Connection test failed for id=%d: %s", id, agentResponse.Output)
			}
		} else {
			// If output parsing fails, use raw output
			connectionStatus = "disabled"
			resultMessage = agentResponse.Output
			logger.Warnf("Failed to parse connection test output for id=%d: %v", id, err)
		}
	} else {
		connectionStatus = "disabled"
		resultMessage = fmt.Sprintf("Connection test failed - Status: %s, ExitCode: %d, ErrorOutput: %s",
			agentResponse.Status, agentResponse.ExitCode, agentResponse.ErrorOutput)
		logger.Errorf("Connection test failed for id=%d: %s", id, resultMessage)
	}

	// Update connection status in database
	cntMgt.Status = connectionStatus
	if err := s.cntMgtRepo.UpdateStatus(nil, id, connectionStatus); err != nil {
		logger.Errorf("Failed to update connection status for id=%d: %v", id, err)
		return "", fmt.Errorf("failed to update connection status: %v", err)
	}

	logger.Infof("Updated connection status for id=%d to %s", id, connectionStatus)
	return resultMessage, nil
}
