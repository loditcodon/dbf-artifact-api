package fileops

import (
	"fmt"
	"os"
	"path/filepath"

	"dbfartifactapi/config"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services/job"
)

// UploadJobContext holds context data for upload job completion processing.
// Stored in JobInfo.ContextData["upload_context"] at job creation time.
type UploadJobContext struct {
	FileName    string
	FilePath    string
	SourceJobID string
}

// CreateUploadCompletionHandler creates a callback function for upload job completion.
// Stores upload file metadata (fileName, filePath, md5Hash) in job results
// so that status check responses include full upload information.
func CreateUploadCompletionHandler() job.JobCompletionCallback {
	return func(jobID string, jobInfo *job.JobInfo, statusResp *job.StatusResponse) error {
		logger.Infof("Processing upload completion for job %s, status: %s", jobID, statusResp.Status)

		contextData, ok := jobInfo.ContextData["upload_context"]
		if !ok {
			return fmt.Errorf("missing upload context data for job %s", jobID)
		}

		uploadContext, ok := contextData.(*UploadJobContext)
		if !ok {
			return fmt.Errorf("invalid upload context data type for job %s", jobID)
		}

		// CRITICAL: Process upload results regardless of completion status
		// This ensures failed jobs are properly logged and results are available for status checks
		return processUploadResults(jobID, uploadContext, statusResp, jobInfo)
	}
}

// processUploadResults processes upload job results and stores file metadata in job results.
// Handles both notification-based and polling-based completion paths.
func processUploadResults(jobID string, uploadContext *UploadJobContext, statusResp *job.StatusResponse, jobInfo *job.JobInfo) error {
	if statusResp.Status == "failed" {
		logger.Errorf("Processing failed upload job %s: message=%s, error=%s",
			jobID, statusResp.Message, statusResp.Error)
	} else {
		logger.Infof("Processing completed upload job %s: message=%s",
			jobID, statusResp.Message)
	}

	if notificationData, exists := jobInfo.ContextData["notification_data"]; exists {
		return processUploadResultsFromNotification(jobID, notificationData)
	}

	return processUploadResultsFromPolling(jobID, uploadContext, statusResp)
}

// processUploadResultsFromNotification handles upload completion when triggered by external notification.
// Resolves actual local file path from NotificationFileDir/jobID/md5Hash and stores full result set.
func processUploadResultsFromNotification(jobID string, notificationData interface{}) error {
	logger.Infof("Processing upload results from notification for job %s", jobID)

	notification, ok := notificationData.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid notification data format for job %s", jobID)
	}

	fileName, ok := notification["fileName"].(string)
	if !ok {
		return fmt.Errorf("missing fileName in notification data for job %s", jobID)
	}

	md5Hash, ok := notification["md5Hash"].(string)
	if !ok {
		return fmt.Errorf("missing md5Hash in notification data for job %s", jobID)
	}

	success, ok := notification["success"].(bool)
	if !ok || !success {
		return fmt.Errorf("job %s was not successful according to notification", jobID)
	}

	// File path structure: NotificationFileDir/jobID/md5Hash
	localFilePath := filepath.Join(config.Cfg.NotificationFileDir, jobID, md5Hash)
	if _, err := os.Stat(localFilePath); err != nil {
		return fmt.Errorf("upload file not found at %s for job %s: %w", localFilePath, jobID, err)
	}

	logger.Infof("Successfully confirmed upload file at %s for fileName=%s", localFilePath, fileName)

	results := map[string]interface{}{
		"upload_file": map[string]interface{}{
			"fileName": fileName,
			"filePath": localFilePath,
			"md5Hash":  md5Hash,
			"status":   "completed",
		},
	}

	jobMonitor := job.GetJobMonitorService()
	jobMonitor.UpdateJobResults(jobID, results)

	logger.Infof("Upload results stored for job %s: fileName=%s, filePath=%s, md5Hash=%s",
		jobID, fileName, localFilePath, md5Hash)

	return nil
}

// processUploadResultsFromPolling handles upload completion detected via VeloArtifact polling.
// Stores available context data without md5Hash (only available via notification).
func processUploadResultsFromPolling(jobID string, uploadContext *UploadJobContext, statusResp *job.StatusResponse) error {
	logger.Infof("Processing upload results from polling for job %s", jobID)

	results := map[string]interface{}{
		"upload_file": map[string]interface{}{
			"fileName":    uploadContext.FileName,
			"filePath":    uploadContext.FilePath,
			"sourceJobId": uploadContext.SourceJobID,
			"status":      statusResp.Status,
		},
	}

	jobMonitor := job.GetJobMonitorService()
	jobMonitor.UpdateJobResults(jobID, results)

	logger.Infof("Upload results stored for job %s (via polling): fileName=%s, filePath=%s",
		jobID, uploadContext.FileName, uploadContext.FilePath)

	return nil
}
