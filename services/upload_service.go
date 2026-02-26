package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services/agent"
	"dbfartifactapi/utils"
)

// UploadService provides file upload operations via VeloArtifact.
type UploadService interface {
	ExecuteUpload(ctx context.Context, req models.UploadRequest) (jobID string, message string, err error)
}

type uploadService struct {
	baseRepo     repository.BaseRepository
	cntMgtRepo   repository.CntMgtRepository
	endpointRepo repository.EndpointRepository
}

// NewUploadService creates a new file upload service instance.
func NewUploadService() UploadService {
	return &uploadService{
		baseRepo:     repository.NewBaseRepository(),
		cntMgtRepo:   repository.NewCntMgtRepository(),
		endpointRepo: repository.NewEndpointRepository(),
	}
}

// ExecuteUpload processes upload request and starts VeloArtifact background job
// Creates upload artifact with sourceJobId, fileName, filePath and executes via VeloArtifact client
// Returns job_id, message, and error
func (s *uploadService) ExecuteUpload(ctx context.Context, req models.UploadRequest) (string, string, error) {
	if ctx == nil {
		return "", "", fmt.Errorf("context cannot be nil")
	}
	if req.CntID == 0 {
		return "", "", fmt.Errorf("invalid connection ID: must be greater than 0")
	}
	if strings.TrimSpace(req.SourceJobID) == "" {
		return "", "", fmt.Errorf("source job ID cannot be empty")
	}
	if strings.TrimSpace(req.FileName) == "" {
		return "", "", fmt.Errorf("file name cannot be empty")
	}
	if strings.TrimSpace(req.FilePath) == "" {
		return "", "", fmt.Errorf("file path cannot be empty")
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

	hexJSON, uploadOption, err := s.createUploadArtifact(req.SourceJobID, req.FileName, req.FilePath)
	if err != nil {
		tx.Rollback()
		return "", "", fmt.Errorf("failed to create upload artifact: %v", err)
	}

	logger.Infof("Executing upload for sourceJobId=%s, fileName=%s, filePath=%s", req.SourceJobID, req.FileName, req.FilePath)

	stdout, err := agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "upload", hexJSON, uploadOption, true)
	if err != nil {
		logger.Errorf("executeSqlAgentAPI error for upload: %v", err)
		tx.Rollback()
		return "", "", fmt.Errorf("failed to execute upload: %v", err)
	}

	type JobResponse struct {
		JobID          string `json:"upload_job_id"`
		FileName       string `json:"filename,omitempty"`
		Message        string `json:"message"`
		MonitorCommand string `json:"monitor_command,omitempty"`
		PID            int    `json:"pid"`
		ResultsCommand string `json:"results_command,omitempty"`
		Success        bool   `json:"success"`
	}

	var jobResp JobResponse
	if err := json.Unmarshal([]byte(stdout), &jobResp); err != nil {
		logger.Errorf("Failed to parse job response: %v", err)
		tx.Rollback()
		return "", "", fmt.Errorf("failed to parse job response: %v", err)
	}

	if !jobResp.Success {
		logger.Errorf("Upload job failed to start: %s", jobResp.Message)
		tx.Rollback()
		return "", "", fmt.Errorf("upload job failed to start: %s", jobResp.Message)
	}

	// Add upload job to monitoring with completion handler for storing file metadata
	completionCallback := CreateUploadCompletionHandler()
	contextData := map[string]interface{}{
		"upload_context": &UploadJobContext{
			FileName:    req.FileName,
			FilePath:    req.FilePath,
			SourceJobID: req.SourceJobID,
		},
	}
	jobMonitor := GetJobMonitorService()
	jobMonitor.AddJobWithCallback(jobResp.JobID, 0, ep.ClientID, ep.OsType, completionCallback, contextData)

	logger.Infof("Upload job started successfully: job_id=%s", jobResp.JobID)

	txCommitted = true
	tx.Rollback()

	return jobResp.JobID, fmt.Sprintf("Upload job started successfully for file: %s", req.FileName), nil
}

// createUploadArtifact creates hex-encoded JSON for upload operation via dbfAgentAPI.
// Returns hexJSON containing {fileName, filePath, sourceJobId} and option string.
func (s *uploadService) createUploadArtifact(sourceJobID, fileName, filePath string) (hexJSON string, option string, err error) {
	hexJSON, err = utils.CreateAgentUploadJSON(fileName, filePath, sourceJobID)
	if err != nil {
		return "", "", fmt.Errorf("failed to create upload hex JSON: %v", err)
	}

	logger.Debugf("Created upload hex JSON: sourceJobId=%s, fileName=%s, filePath=%s", sourceJobID, fileName, filePath)
	return hexJSON, "--background", nil
}
