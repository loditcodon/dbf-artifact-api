package fileops

import (
	"fmt"
	"os"

	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services/job"
)

// DownloadJobContext holds context data for download job completion processing.
// Stored in JobInfo.ContextData["download_context"] at job creation time.
type DownloadJobContext struct {
	ArchivePath  string // Path to compressed archive file (empty if source was a file)
	IsCompressed bool   // Whether source was compressed
}

// CreateDownloadCompletionHandler creates a callback function for download job completion.
// Cleans up temporary compressed archive file after job completes and marks job as done.
func CreateDownloadCompletionHandler() job.JobCompletionCallback {
	return func(jobID string, jobInfo *job.JobInfo, statusResp *job.StatusResponse) error {
		logger.Infof("Processing download completion for job %s, status: %s", jobID, statusResp.Status)

		contextData, ok := jobInfo.ContextData["download_context"]
		if !ok {
			logger.Debugf("No download context data for job %s, skipping cleanup", jobID)
			return finalizeDownloadJob(jobID, statusResp)
		}

		downloadContext, ok := contextData.(*DownloadJobContext)
		if !ok {
			logger.Warnf("Invalid download context data type for job %s", jobID)
			return finalizeDownloadJob(jobID, statusResp)
		}

		// Clean up compressed archive if it exists
		if downloadContext.IsCompressed && downloadContext.ArchivePath != "" {
			if err := os.Remove(downloadContext.ArchivePath); err != nil {
				if os.IsNotExist(err) {
					logger.Debugf("Archive file already removed: %s", downloadContext.ArchivePath)
				} else {
					logger.Warnf("Failed to remove archive file %s: %v", downloadContext.ArchivePath, err)
				}
			} else {
				logger.Infof("Cleaned up archive file after download job %s: %s", jobID, downloadContext.ArchivePath)
			}
		}

		return finalizeDownloadJob(jobID, statusResp)
	}
}

// finalizeDownloadJob marks the download job as completed or failed based on client status.
func finalizeDownloadJob(jobID string, statusResp *job.StatusResponse) error {
	jobMonitor := job.GetJobMonitorService()

	if statusResp.Status == "completed" {
		message := fmt.Sprintf("Download completed successfully: %s", statusResp.Message)
		if err := jobMonitor.CompleteJobAfterProcessing(jobID, message); err != nil {
			logger.Errorf("Failed to mark download job %s as completed: %v", jobID, err)
			return err
		}
		logger.Infof("Download job %s marked as completed", jobID)
	} else if statusResp.Status == "failed" {
		errorMsg := statusResp.Error
		if errorMsg == "" {
			errorMsg = statusResp.Message
		}
		if err := jobMonitor.FailJobAfterProcessing(jobID, errorMsg); err != nil {
			logger.Errorf("Failed to mark download job %s as failed: %v", jobID, err)
			return err
		}
		logger.Errorf("Download job %s marked as failed: %s", jobID, errorMsg)
	}

	return nil
}
