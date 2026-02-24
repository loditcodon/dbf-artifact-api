package services

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services/dto"
	"dbfartifactapi/utils"
)

// BackupJobContext contains context data for backup job completion callback
type BackupJobContext struct {
	JobID    uint
	CntID    uint
	Type     string
	FileName string
	Command  string
}

// BackupService provides database backup operations via VeloArtifact.
type BackupService interface {
	ExecuteBackup(ctx context.Context, req models.BackupRequest) (jobID string, message string, err error)
}

type backupService struct {
	baseRepo     repository.BaseRepository
	cntMgtRepo   repository.CntMgtRepository
	endpointRepo repository.EndpointRepository
}

// NewBackupService creates a new database backup service instance.
func NewBackupService() BackupService {
	return &backupService{
		baseRepo:     repository.NewBaseRepository(),
		cntMgtRepo:   repository.NewCntMgtRepository(),
		endpointRepo: repository.NewEndpointRepository(),
	}
}

// ExecuteBackup processes backup request and starts VeloArtifact background job
// Hex-decodes command, processes steps, and executes via VeloArtifact client
// Returns job_id, message, and error
func (s *backupService) ExecuteBackup(ctx context.Context, req models.BackupRequest) (string, string, error) {
	if ctx == nil {
		return "", "", fmt.Errorf("context cannot be nil")
	}
	if req.JobID == 0 {
		return "", "", fmt.Errorf("invalid job ID: must be greater than 0")
	}
	if req.CntID == 0 {
		return "", "", fmt.Errorf("invalid connection ID: must be greater than 0")
	}
	if strings.TrimSpace(req.Command) == "" {
		return "", "", fmt.Errorf("command cannot be empty")
	}
	if strings.TrimSpace(req.Type) == "" {
		return "", "", fmt.Errorf("type cannot be empty")
	}

	tx := s.baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	cmt, err := s.cntMgtRepo.GetCntMgtByID(tx, req.CntID)
	if err != nil {
		tx.Rollback()
		return "", "", fmt.Errorf("connection management with id=%d not found: %v", req.CntID, err)
	}
	logger.Infof("Found connection: id=%d, type=%s, agent=%d", cmt.ID, cmt.CntType, cmt.Agent)

	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		tx.Rollback()
		return "", "", fmt.Errorf("endpoint with id=%d not found: %v", cmt.Agent, err)
	}
	logger.Infof("Found endpoint: id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	// CRITICAL: Hex-decoding prevents command injection during storage
	// Commands must remain encoded until execution time for security
	commandBytes, err := hex.DecodeString(req.Command)
	if err != nil {
		tx.Rollback()
		return "", "", fmt.Errorf("failed to decode hex command: %v", err)
	}

	var backupCmd models.BackupCommand
	if err := json.Unmarshal(commandBytes, &backupCmd); err != nil {
		tx.Rollback()
		return "", "", fmt.Errorf("failed to parse backup command: %v", err)
	}

	if len(backupCmd.Steps) == 0 {
		tx.Rollback()
		return "", "", fmt.Errorf("backup command must contain at least one step")
	}

	logger.Infof("Parsed %d backup steps for job_id=%d", len(backupCmd.Steps), req.JobID)

	type JobResponse struct {
		JobID          string `json:"job_id"`
		FileName       string `json:"filename,omitempty"` // For os_execute response
		Message        string `json:"message"`
		MonitorCommand string `json:"monitor_command,omitempty"` // For os_execute response
		PID            int    `json:"pid"`
		ResultsCommand string `json:"results_command,omitempty"`
		Success        bool   `json:"success"`
	}

	// Create master backup job ID for tracking
	masterJobID := fmt.Sprintf("backup_%d", req.JobID)

	// Prepare context data for tracking
	backupContext := &BackupJobContext{
		JobID:    req.JobID,
		CntID:    req.CntID,
		Type:     req.Type,
		FileName: req.FileName,
		Command:  req.Command,
	}

	contextData := map[string]interface{}{
		"backup_context":   backupContext,
		"total_steps":      len(backupCmd.Steps),
		"completed_steps":  0,
		"os_job_ids":       []string{},                 // Only OS execute jobs need monitoring
		"sql_step_results": []map[string]interface{}{}, // SQL results available immediately
	}

	// Three main cases for backup execution:
	// Case 1: dump type with filename - execute each step (OS or SQL)
	// Case 2: os type - execute OS commands only (e.g., Oracle RMAN backup)
	// Case 3: other types - execute all SQL steps separately
	if req.Type == "dump" && req.FileName != "" {
		// TH1: Execute each step separately
		var osJobIDs []string
		sqlStepResults := contextData["sql_step_results"].([]map[string]interface{})

		for _, step := range backupCmd.Steps {
			var stdout string
			var err error

			if step.Type == "os" {
				// OS command: needs monitoring, result comes later via callback
				hexJSON, osOption, osErr := s.createOSExecuteArtifact(step.Command, req.FileName)
				if osErr != nil {
					tx.Rollback()
					return "", "", fmt.Errorf("failed to create OS artifact for step %d: %v", step.Order, osErr)
				}
				stdout, err = executeSqlAgentAPI(ep.ClientID, ep.OsType, "os_execute", hexJSON, osOption, true)
			} else if step.Type == "sql" {
				// SQL command: result available immediately in response
				hexJSON, sqlErr := s.createSQLExecuteArtifact(step.Command, cmt)
				if sqlErr != nil {
					tx.Rollback()
					return "", "", fmt.Errorf("failed to create SQL artifact for step %d: %v", step.Order, sqlErr)
				}
				stdout, err = executeSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", true)
			} else {
				tx.Rollback()
				return "", "", fmt.Errorf("unsupported step type: %s", step.Type)
			}

			if err != nil {
				logger.Errorf("executeSqlAgentAPI error for step %d: %v", step.Order, err)
				tx.Rollback()
				return "", "", fmt.Errorf("failed to execute step %d: %v", step.Order, err)
			}

			var jobResp JobResponse
			if err := json.Unmarshal([]byte(stdout), &jobResp); err != nil {
				logger.Errorf("Failed to parse job response for step %d: %v", step.Order, err)
				tx.Rollback()
				return "", "", fmt.Errorf("failed to parse job response for step %d: %v", step.Order, err)
			}

			if !jobResp.Success {
				logger.Errorf("Step %d job failed to start: %s", step.Order, jobResp.Message)
				tx.Rollback()
				return "", "", fmt.Errorf("step %d job failed to start: %s", step.Order, jobResp.Message)
			}

			if step.Type == "os" {
				// OS execute: save job_id for monitoring
				logger.Infof("Step %d OS execute job started: job_id=%s (will monitor for results)", step.Order, jobResp.JobID)
				osJobIDs = append(osJobIDs, jobResp.JobID)
			} else if step.Type == "sql" {
				// SQL execute: result available immediately - parse full query results
				// The stdout contains the actual query results with "results" array
				var queryResult map[string]interface{}
				if err := json.Unmarshal([]byte(stdout), &queryResult); err != nil {
					logger.Warnf("Failed to parse query result for step %d: %v", step.Order, err)
					queryResult = map[string]interface{}{
						"message": jobResp.Message,
					}
				}

				logger.Infof("Step %d SQL execute completed: result=%s", step.Order, jobResp.Message)
				sqlStepResults = append(sqlStepResults, map[string]interface{}{
					"step_order": step.Order,
					"step_type":  "sql",
					"command":    step.Command,
					"result":     queryResult, // Store full result including "results" array
					"job_id":     jobResp.JobID,
				})
			}
		}

		// Update master job context with OS job IDs (for monitoring) and SQL results (already complete)
		contextData["os_job_ids"] = osJobIDs
		contextData["sql_step_results"] = sqlStepResults
		contextData["completed_steps"] = len(sqlStepResults)
		contextData["master_job_id"] = masterJobID

		jobMonitor := GetJobMonitorService()

		// If only SQL steps (no OS jobs), mark job completed immediately
		if len(osJobIDs) == 0 {
			// Create master job entry for querying
			completionCallback := CreateBackupCompletionHandler()
			jobMonitor.AddJobWithCallback(masterJobID, req.JobID, ep.ClientID, ep.OsType, completionCallback, contextData)

			message := fmt.Sprintf("Backup completed with %d SQL steps (all executed immediately)", len(sqlStepResults))
			if err := jobMonitor.CompleteJobImmediately(masterJobID, message, len(sqlStepResults)); err != nil {
				logger.Errorf("Failed to mark job completed: %v", err)
			}
			logger.Infof("SQL-only backup completed immediately: master_job_id=%s", masterJobID)
		} else {
			// Add master job entry but mark as processed to skip VeloArtifact status check
			// Master job is just for tracking, real jobs are OS sub-jobs
			completionCallback := CreateBackupCompletionHandler()
			jobMonitor.AddJobWithCallback(masterJobID, req.JobID, ep.ClientID, ep.OsType, completionCallback, contextData)

			// Mark master job to skip VeloArtifact polling since it doesn't exist on VeloArtifact
			// We'll monitor OS sub-jobs instead
			if err := jobMonitor.MarkJobAsNoPolling(masterJobID); err != nil {
				logger.Warnf("Failed to mark master job as no-polling: %v", err)
			}

			// Add each OS sub-job to monitoring with callback that tracks master job progress
			subJobCallback := CreateBackupSubJobCompletionHandler(masterJobID)
			for _, osJobID := range osJobIDs {
				jobMonitor.AddJobWithCallback(osJobID, req.JobID, ep.ClientID, ep.OsType, subJobCallback, contextData)
				logger.Infof("Added OS sub-job to monitoring: job_id=%s, master=%s", osJobID, masterJobID)
			}

			logger.Infof("Added master job to tracking: master_job_id=%s, os_jobs=%v", masterJobID, osJobIDs)
		}

		txCommitted = true
		tx.Rollback()

		logger.Infof("Backup dump jobs started: master_job_id=%s, req_job_id=%d, total_steps=%d, os_jobs=%v, sql_completed=%d",
			masterJobID, req.JobID, len(backupCmd.Steps), osJobIDs, len(sqlStepResults))
		return masterJobID, fmt.Sprintf("Backup job started successfully with %d steps (%d SQL completed, %d OS monitoring)",
			len(backupCmd.Steps), len(sqlStepResults), len(osJobIDs)), nil

	} else if req.Type == "os" {
		// Case 2: OS type - execute OS commands only (e.g., Oracle RMAN backup)
		var osJobIDs []string

		for _, step := range backupCmd.Steps {
			if step.Type != "os" {
				logger.Warnf("Step %d has type=%s but request type=os, skipping", step.Order, step.Type)
				continue
			}

			// OS command execution via dbfAgentAPI
			hexJSON, osOption, osErr := s.createOSExecuteArtifact(step.Command, req.FileName)
			if osErr != nil {
				tx.Rollback()
				return "", "", fmt.Errorf("failed to create OS artifact for step %d: %v", step.Order, osErr)
			}

			stdout, err := executeSqlAgentAPI(ep.ClientID, ep.OsType, "os_execute", hexJSON, osOption, true)
			if err != nil {
				logger.Errorf("executeSqlAgentAPI error for OS step %d: %v", step.Order, err)
				tx.Rollback()
				return "", "", fmt.Errorf("failed to execute OS step %d: %v", step.Order, err)
			}

			var jobResp JobResponse
			if err := json.Unmarshal([]byte(stdout), &jobResp); err != nil {
				logger.Errorf("Failed to parse job response for OS step %d: %v", step.Order, err)
				tx.Rollback()
				return "", "", fmt.Errorf("failed to parse job response for OS step %d: %v", step.Order, err)
			}

			if !jobResp.Success {
				logger.Errorf("OS step %d job failed to start: %s", step.Order, jobResp.Message)
				tx.Rollback()
				return "", "", fmt.Errorf("OS step %d job failed to start: %s", step.Order, jobResp.Message)
			}

			logger.Infof("OS step %d job started: job_id=%s", step.Order, jobResp.JobID)
			osJobIDs = append(osJobIDs, jobResp.JobID)
		}

		if len(osJobIDs) == 0 {
			tx.Rollback()
			return "", "", fmt.Errorf("no OS jobs started for type=os backup request")
		}

		// Update context with OS job IDs for monitoring
		contextData["os_job_ids"] = osJobIDs
		contextData["master_job_id"] = masterJobID

		jobMonitor := GetJobMonitorService()
		completionCallback := CreateBackupCompletionHandler()
		jobMonitor.AddJobWithCallback(masterJobID, req.JobID, ep.ClientID, ep.OsType, completionCallback, contextData)

		// Mark master job to skip VeloArtifact polling - we monitor OS sub-jobs instead
		if err := jobMonitor.MarkJobAsNoPolling(masterJobID); err != nil {
			logger.Warnf("Failed to mark master job as no-polling: %v", err)
		}

		// Add each OS sub-job to monitoring
		subJobCallback := CreateBackupSubJobCompletionHandler(masterJobID)
		for _, osJobID := range osJobIDs {
			jobMonitor.AddJobWithCallback(osJobID, req.JobID, ep.ClientID, ep.OsType, subJobCallback, contextData)
			logger.Infof("Added OS sub-job to monitoring: job_id=%s, master=%s", osJobID, masterJobID)
		}

		txCommitted = true
		tx.Rollback()

		logger.Infof("OS backup jobs started: master_job_id=%s, req_job_id=%d, os_jobs=%v",
			masterJobID, req.JobID, osJobIDs)
		return masterJobID, fmt.Sprintf("OS backup job started successfully with %d OS steps", len(osJobIDs)), nil

	} else {
		// Case 3: Other backup types - execute each step separately (all SQL commands)
		sqlStepResults := contextData["sql_step_results"].([]map[string]interface{})

		for _, step := range backupCmd.Steps {
			// All steps are SQL commands for check_binlog type - results available immediately
			hexJSON, err := s.createSQLExecuteArtifact(step.Command, cmt)
			if err != nil {
				tx.Rollback()
				return "", "", fmt.Errorf("failed to create artifact for step %d: %v", step.Order, err)
			}

			// Execute this step immediately
			stdout, err := executeSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", true)
			if err != nil {
				logger.Errorf("executeSqlAgentAPI error for step %d: %v", step.Order, err)
				tx.Rollback()
				return "", "", fmt.Errorf("failed to execute step %d: %v", step.Order, err)
			}

			var jobResp JobResponse
			if err := json.Unmarshal([]byte(stdout), &jobResp); err != nil {
				logger.Errorf("Failed to parse job response for step %d: %v", step.Order, err)
				tx.Rollback()
				return "", "", fmt.Errorf("failed to parse job response for step %d: %v", step.Order, err)
			}

			if !jobResp.Success {
				logger.Errorf("Step %d job failed to start: %s", step.Order, jobResp.Message)
				tx.Rollback()
				return "", "", fmt.Errorf("step %d job failed to start: %s", step.Order, jobResp.Message)
			}

			// SQL execute: result available immediately - parse full query results
			// The stdout contains the actual query results with "results" array
			var queryResult map[string]interface{}
			if err := json.Unmarshal([]byte(stdout), &queryResult); err != nil {
				logger.Warnf("Failed to parse query result for step %d: %v", step.Order, err)
				queryResult = map[string]interface{}{
					"message": jobResp.Message,
				}
			}

			logger.Infof("Step %d SQL execute completed: result=%s", step.Order, jobResp.Message)
			sqlStepResults = append(sqlStepResults, map[string]interface{}{
				"step_order": step.Order,
				"step_type":  "sql",
				"command":    step.Command,
				"result":     queryResult, // Store full result including "results" array
				"job_id":     jobResp.JobID,
			})
		}

		// All SQL steps completed immediately, no OS jobs to monitor
		contextData["sql_step_results"] = sqlStepResults
		contextData["completed_steps"] = len(sqlStepResults)

		// Add job to monitoring and mark completed immediately
		jobMonitor := GetJobMonitorService()
		completionCallback := CreateBackupCompletionHandler()
		jobMonitor.AddJobWithCallback(masterJobID, req.JobID, ep.ClientID, ep.OsType, completionCallback, contextData)

		message := fmt.Sprintf("Backup completed with %d SQL steps (all executed immediately)", len(sqlStepResults))
		if err := jobMonitor.CompleteJobImmediately(masterJobID, message, len(sqlStepResults)); err != nil {
			logger.Errorf("Failed to mark job completed: %v", err)
		}

		logger.Infof("Backup SQL-only completed immediately: master_job_id=%s, req_job_id=%d, sql_steps_completed=%d",
			masterJobID, req.JobID, len(sqlStepResults))

		txCommitted = true
		tx.Rollback()

		return masterJobID, fmt.Sprintf("Backup job completed successfully with %d SQL steps", len(sqlStepResults)), nil
	}
}

// createOSExecuteArtifact creates hex-encoded JSON and option for OS command execution via dbfAgentAPI.
// Returns hexJSON (for Value parameter) and option string (e.g., "--background").
func (s *backupService) createOSExecuteArtifact(command, fileName string) (hexJSON string, option string, err error) {
	builder := dto.NewDBQueryParamBuilder().
		SetCommandExec(command).
		SetAction("os_execute")

	if fileName != "" {
		builder.SetFileName(fileName)
	}

	queryParam := builder.Build()

	hexJSON, err = utils.CreateAgentOSExecuteJSON(queryParam)
	if err != nil {
		return "", "", fmt.Errorf("failed to create OS execute hex JSON: %v", err)
	}

	logger.Debugf("Created os_execute hex JSON: commandExec=%s, fileName=%s", command, fileName)
	return hexJSON, "--background", nil
}

// createSQLExecuteArtifact creates artifact JSON for SQL command execution.
// Uses full queryParam with database connection info.
// For Oracle databases, sets the database field to ServiceName for proper connection.
func (s *backupService) createSQLExecuteArtifact(command string, cmt *models.CntMgt) (string, error) {
	builder := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cmt.CntType)).
		SetHost(cmt.IP).
		SetPort(cmt.Port).
		SetUser(cmt.Username).
		SetPassword(cmt.Password)

	// Oracle requires ServiceName for connection instead of database name
	if strings.ToLower(cmt.CntType) == "oracle" {
		builder.SetDatabase(cmt.ServiceName)
	}

	queryParam := builder.Build()
	queryParam.Query = command
	queryParam.Action = "execute"

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		return "", fmt.Errorf("failed to create agent command JSON: %v", err)
	}

	logger.Debugf("Created execute artifact: query=%s", command)
	return hexJSON, nil
}
