package fileops

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services"
	"dbfartifactapi/services/agent"
	"dbfartifactapi/utils"
)

// DownloadService provides file download operations from server to agent via dbfAgentAPI.
type DownloadService interface {
	ExecuteDownload(ctx context.Context, req models.DownloadRequest) (jobID string, message string, err error)
}

type downloadService struct {
	baseRepo     repository.BaseRepository
	cntMgtRepo   repository.CntMgtRepository
	endpointRepo repository.EndpointRepository
}

// NewDownloadService creates a new file download service instance.
func NewDownloadService() DownloadService {
	return &downloadService{
		baseRepo:     repository.NewBaseRepository(),
		cntMgtRepo:   repository.NewCntMgtRepository(),
		endpointRepo: repository.NewEndpointRepository(),
	}
}

// ExecuteDownload processes download request and starts dbfAgentAPI background job.
// If SourcePath is a directory, compresses it to tar.gz before sending.
// Calculates MD5 hash and sends filedownload command to agent.
// Returns job_id, message, and error.
func (s *downloadService) ExecuteDownload(ctx context.Context, req models.DownloadRequest) (string, string, error) {
	if ctx == nil {
		return "", "", fmt.Errorf("context cannot be nil")
	}
	if req.CntID == 0 {
		return "", "", fmt.Errorf("invalid connection ID: must be greater than 0")
	}
	if strings.TrimSpace(req.SourcePath) == "" {
		return "", "", fmt.Errorf("source path cannot be empty")
	}
	// if strings.TrimSpace(req.SavePath) == "" {
	// 	return "", "", fmt.Errorf("save path cannot be empty")
	// }

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

	hexJSON, fileName, archivePath, isCompressed, err := s.prepareDownloadPayload(req.SourcePath, req.SavePath)
	if err != nil {
		tx.Rollback()
		return "", "", fmt.Errorf("failed to prepare download payload: %v", err)
	}

	logger.Infof("Executing download: source=%s, save_path=%s, file_name=%s, compressed=%v",
		req.SourcePath, req.SavePath, fileName, isCompressed)

	stdout, err := agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "filedownload", hexJSON, "--background", true)
	if err != nil {
		logger.Errorf("executeSqlAgentAPI error for filedownload: %v", err)
		// Clean up archive if command failed
		if isCompressed && archivePath != "" {
			os.Remove(archivePath)
		}
		tx.Rollback()
		return "", "", fmt.Errorf("failed to execute filedownload: %v", err)
	}

	type JobResponse struct {
		JobID          string `json:"job_id"`
		FileName       string `json:"filename,omitempty"`
		Message        string `json:"message"`
		MonitorCommand string `json:"monitor_command,omitempty"`
		PID            int    `json:"pid"`
		Success        bool   `json:"success"`
	}

	var jobResp JobResponse
	if err := json.Unmarshal([]byte(stdout), &jobResp); err != nil {
		logger.Errorf("Failed to parse job response: %v", err)
		// Clean up archive if parsing failed
		if isCompressed && archivePath != "" {
			os.Remove(archivePath)
		}
		tx.Rollback()
		return "", "", fmt.Errorf("failed to parse job response: %v", err)
	}

	if !jobResp.Success {
		logger.Errorf("Download job failed to start: %s", jobResp.Message)
		// Clean up archive if job failed to start
		if isCompressed && archivePath != "" {
			os.Remove(archivePath)
		}
		tx.Rollback()
		return "", "", fmt.Errorf("download job failed to start: %s", jobResp.Message)
	}

	// Add download job to monitoring with completion handler for cleanup
	completionCallback := CreateDownloadCompletionHandler()
	contextData := map[string]interface{}{
		"download_context": &DownloadJobContext{
			ArchivePath:  archivePath,
			IsCompressed: isCompressed,
		},
	}
	jobMonitor := services.GetJobMonitorService()
	jobMonitor.AddJobWithCallback(jobResp.JobID, 0, ep.ClientID, ep.OsType, completionCallback, contextData)

	logger.Infof("Download job started successfully: job_id=%s", jobResp.JobID)

	txCommitted = true
	tx.Rollback()

	return jobResp.JobID, fmt.Sprintf("Download job started successfully for file: %s", fileName), nil
}

// prepareDownloadPayload prepares the hex-encoded JSON payload for filedownload command.
// If sourcePath is a directory, compresses it to tar.gz first.
// Returns hexJSON, fileName, archivePath (empty if not compressed), isCompressed, and error.
func (s *downloadService) prepareDownloadPayload(sourcePath, savePath string) (string, string, string, bool, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", "", "", false, fmt.Errorf("failed to access source path %s: %w", sourcePath, err)
	}

	var filePath string
	var fileName string
	var archivePath string
	var isCompressed bool
	var compressionType string

	if info.IsDir() {
		// Directory: compress to tar.gz
		archivePath, err = utils.CompressDirectoryToTarGz(sourcePath)
		if err != nil {
			return "", "", "", false, fmt.Errorf("failed to compress directory %s: %w", sourcePath, err)
		}
		filePath = archivePath
		fileName = filepath.Base(archivePath)
		isCompressed = true
		compressionType = "tar.gz"
		logger.Infof("Compressed directory %s to %s", sourcePath, archivePath)
	} else {
		// File: use directly
		filePath = sourcePath
		fileName = filepath.Base(sourcePath)
		archivePath = ""
		isCompressed = false
		compressionType = ""
	}

	md5Hash, err := utils.CalculateFileMD5(filePath)
	if err != nil {
		return "", "", "", false, fmt.Errorf("failed to calculate MD5 for %s: %w", filePath, err)
	}
	logger.Debugf("Calculated MD5 for %s: %s", filePath, md5Hash)

	hexJSON, err := utils.CreateAgentDownloadJSON(fileName, savePath, md5Hash, isCompressed, compressionType)
	if err != nil {
		return "", "", "", false, fmt.Errorf("failed to create download hex JSON: %w", err)
	}

	logger.Debugf("Created download hex JSON: fileName=%s, savePath=%s, md5Hash=%s, isCompressed=%v",
		fileName, savePath, md5Hash, isCompressed)

	return hexJSON, fileName, archivePath, isCompressed, nil
}
