package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services/dto"
	"dbfartifactapi/utils"
)

// PolicyComplianceService provides business logic for policy compliance verification operations.
type PolicyComplianceService interface {
	StartCheck(ctx context.Context, cntMgtID uint) (string, error)
}

type policyComplianceService struct {
	baseRepo     repository.BaseRepository
	cntMgtRepo   repository.CntMgtRepository
	endpointRepo repository.EndpointRepository
}

// NewPolicyComplianceService creates a new policy compliance service instance.
func NewPolicyComplianceService() PolicyComplianceService {
	return &policyComplianceService{
		baseRepo:     repository.NewBaseRepository(),
		cntMgtRepo:   repository.NewCntMgtRepository(),
		endpointRepo: repository.NewEndpointRepository(),
	}
}

func (s *policyComplianceService) StartCheck(ctx context.Context, cntMgtID uint) (string, error) {
	tx := s.baseRepo.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	logger.Debugf("Starting policy compliance check for cntmgt ID: %d", cntMgtID)

	// Get connection management info
	cmt, err := s.cntMgtRepo.GetCntMgtByID(tx, cntMgtID)
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to get cntmgt with id=%d: %w", cntMgtID, err)
	}
	logger.Infof("Found cmt with id=%d, cnt_type=%s, username=%s", cmt.ID, cmt.CntType, cmt.Username)

	// Get endpoint by cmt.Agent
	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to get endpoint with id=%d: %w", cmt.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	// Oracle connections use ServiceName instead of a database name
	database := ""
	if strings.EqualFold(cmt.CntType, "oracle") {
		database = cmt.ServiceName
	}

	// Build connection parameters for policy compliance
	queryParam := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cmt.CntType)).
		SetHost(cmt.IP).
		SetPort(cmt.Port).
		SetUser(cmt.Username).
		SetPassword(cmt.Password).
		SetDatabase(database).
		SetQuery("").
		SetFileConfig(cmt.ConfigFilePath).
		Build()

	queryParam.Action = "policycompliance"
	queryParam.Option = "--background"

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to create agent command JSON: %w", err)
	}

	type JobResponse struct {
		JobID          string `json:"job_id"`
		Message        string `json:"message"`
		MonitorCommand string `json:"monitor_command"`
		PID            int    `json:"pid"`
		Success        bool   `json:"success"`
		DatabaseName   string `json:"database_name"`
		DBType         string `json:"db_type"`
	}

	// Start background job with --background option
	stdout, err := executeSqlAgentAPI(ep.ClientID, ep.OsType, "policycompliance", hexJSON, "--background", true)
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to start agent API job: %w", err)
	}

	var jobResp JobResponse
	if err := json.Unmarshal([]byte(stdout), &jobResp); err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to parse job response: %w", err)
	}

	if !jobResp.Success {
		tx.Rollback()
		return "", fmt.Errorf("job failed to start: %s", jobResp.Message)
	}

	logger.Infof("Policy compliance check started successfully: %s", jobResp.JobID)

	// Prepare context data for job completion callback
	complianceContext := &PolicyComplianceJobContext{
		CntMgtID:   cntMgtID,
		CMT:        cmt,
		EndpointID: ep.ID,
	}

	contextData := map[string]interface{}{
		"policy_compliance_context": complianceContext,
	}

	// Add job to monitoring system with completion callback
	jobMonitor := GetJobMonitorService()
	completionCallback := CreatePolicyComplianceCompletionHandler()
	jobMonitor.AddJobWithCallback(jobResp.JobID, cntMgtID, ep.ClientID, ep.OsType, completionCallback, contextData)

	tx.Rollback()

	logger.Infof("Policy compliance job %s started for cntmgt_id=%d", jobResp.JobID, cntMgtID)
	return fmt.Sprintf("Background policy compliance check started: %s. Use job monitoring to track completion.", jobResp.JobID), nil
}
