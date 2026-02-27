package fileops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"dbfartifactapi/config"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services/job"
)

// CreateBackupCompletionHandler creates a callback function for backup job completion
func CreateBackupCompletionHandler() job.JobCompletionCallback {
	return func(jobID string, jobInfo *job.JobInfo, statusResp *job.StatusResponse) error {
		logger.Infof("Processing backup completion for job %s, status: %s", jobID, statusResp.Status)

		contextData, ok := jobInfo.ContextData["backup_context"]
		if !ok {
			return fmt.Errorf("missing backup context data for job %s", jobID)
		}

		// CRITICAL: Process backup results regardless of completion status
		// This ensures failed jobs are properly handled and logged
		return processBackupResults(jobID, contextData, statusResp, jobInfo)
	}
}

// CreateBackupSubJobCompletionHandler creates callback for OS sub-job completion.
// Critical for accurate master job status - ensures failed sub-jobs cause master job failure.
// Returns JobCompletionCallback that handles sub-job status aggregation.
func CreateBackupSubJobCompletionHandler(masterJobID string) job.JobCompletionCallback {
	return func(subJobID string, jobInfo *job.JobInfo, statusResp *job.StatusResponse) error {
		logger.Infof("Processing backup sub-job completion: sub_job=%s, master=%s, status=%s",
			subJobID, masterJobID, statusResp.Status)

		jobMonitor := job.GetJobMonitorService()

		// Store OS execution results from agent response to sub-job
		if len(statusResp.Results) > 0 {
			jobMonitor.SetJobResults(subJobID, statusResp.Results)
			logger.Debugf("Stored %d results for sub-job %s", len(statusResp.Results), subJobID)
		}

		// CRITICAL: Finalize sub-job status from intermediate "processing" to actual final status.
		// updateJobStatus sets "processing" instead of "completed" when job has a CompletionCallback,
		// which prevents other sub-job callbacks from counting this sub-job correctly.
		if statusResp.Status == "completed" {
			if err := jobMonitor.CompleteJobAfterProcessing(subJobID, statusResp.Message); err != nil {
				logger.Warnf("Failed to finalize completed sub-job %s: %v", subJobID, err)
			}
		} else if statusResp.Status == "failed" {
			errMsg := statusResp.Message
			if statusResp.Error != "" {
				errMsg = statusResp.Error
			}
			if err := jobMonitor.FailJobAfterProcessing(subJobID, errMsg); err != nil {
				logger.Warnf("Failed to finalize failed sub-job %s: %v", subJobID, err)
			}
		}

		// CRITICAL: Master job must exist for progress tracking
		masterJob, exists := jobMonitor.GetJob(masterJobID)
		if !exists {
			return fmt.Errorf("failed to process sub-job %s: master job %s not found", subJobID, masterJobID)
		}

		// CRITICAL: OS job IDs required for progress calculation
		osJobIDs, ok := masterJob.ContextData["os_job_ids"].([]string)
		if !ok {
			return fmt.Errorf("failed to process sub-job %s: invalid os_job_ids in master job %s", subJobID, masterJobID)
		}

		// CRITICAL: Separate counting ensures accurate master job status
		// Failed sub-jobs must cause master job failure, not success
		// "processing" = VeloArtifact reported completion but callback not yet finalized (race condition)
		// "error" = status check command itself failed (treat as failure for aggregation)
		completedCount := 0
		failedCount := 0
		var errorMessages []string
		for _, osJobID := range osJobIDs {
			job, exists := jobMonitor.GetJob(osJobID)
			if exists {
				if job.Status == "completed" || job.Status == "processing" {
					completedCount++
				} else if job.Status == "failed" || job.Status == "error" {
					failedCount++
					// CRITICAL: Error messages required for troubleshooting by client
					if job.Error != "" {
						errorMessages = append(errorMessages, job.Error)
					} else if job.Message != "" && job.Message != "Job started" {
						errorMessages = append(errorMessages, job.Message)
					}
				}
			}
		}

		logger.Infof("Master job %s progress: %d/%d OS sub-jobs completed (%d succeeded, %d failed)",
			masterJobID, completedCount+failedCount, len(osJobIDs), completedCount, failedCount)

		// CRITICAL: Only update master status when ALL sub-jobs finished
		// This prevents premature completion notification to client
		if completedCount+failedCount == len(osJobIDs) {
			sqlStepResults, _ := masterJob.ContextData["sql_step_results"].([]map[string]interface{})
			totalSteps := len(osJobIDs) + len(sqlStepResults)

			var message string
			var masterStatus string

			if failedCount > 0 {
				// CRITICAL: Master job should be failed if any sub-jobs failed
				masterStatus = "failed"
				message = fmt.Sprintf("Backup partially failed: %d of %d steps completed (%d SQL completed, %d OS succeeded, %d OS failed)",
					completedCount+len(sqlStepResults), totalSteps, len(sqlStepResults), completedCount, failedCount)

				// CRITICAL: Include actual error messages for client display
				if len(errorMessages) > 0 {
					message += " | Errors: "
					for i, errMsg := range errorMessages {
						if i > 0 {
							message += "; "
						}
						// Truncate very long error messages for readability
						if len(errMsg) > 200 {
							message += errMsg[:200] + "..."
						} else {
							message += errMsg
						}
					}
				}
			} else {
				// All jobs succeeded
				masterStatus = "completed"
				message = fmt.Sprintf("Backup completed successfully with %d steps (%d SQL completed, %d OS completed)",
					totalSteps, len(sqlStepResults), len(osJobIDs))
			}

			// Aggregate OS results from all sub-jobs into master job
			var aggregatedResults []interface{}
			for _, osJobID := range osJobIDs {
				subJob, exists := jobMonitor.GetJob(osJobID)
				if exists && subJob.Results != nil {
					if resultsList, ok := subJob.Results.([]interface{}); ok {
						aggregatedResults = append(aggregatedResults, resultsList...)
					}
				}
			}
			if len(aggregatedResults) > 0 {
				jobMonitor.SetJobResults(masterJobID, aggregatedResults)
				logger.Debugf("Aggregated %d OS results into master job %s", len(aggregatedResults), masterJobID)
			}

			// CRITICAL: Update master job status with accurate failure information
			if err := jobMonitor.CompleteJobWithResults(masterJobID, masterStatus, message,
				completedCount+len(sqlStepResults), failedCount); err != nil {
				// Preserve sub-job failure context when master job update fails
				return fmt.Errorf("failed to update master job %s status after processing sub-job %s: %w",
					masterJobID, subJobID, err)
			}

			logger.Infof("Master job %s marked as %s after all sub-jobs finished (success: %d, failed: %d)",
				masterJobID, masterStatus, completedCount, failedCount)
		}

		return nil
	}
}

// processBackupResults processes the results of a completed backup job
func processBackupResults(jobID string, contextData interface{}, statusResp *job.StatusResponse, jobInfo *job.JobInfo) error {
	logger.Infof("Processing backup results for job %s - completed: %d, failed: %d",
		jobID, statusResp.Completed, statusResp.Failed)

	backupContext, ok := contextData.(*BackupJobContext)
	if !ok {
		return fmt.Errorf("invalid backup context data for job %s", jobID)
	}

	// CRITICAL: Log job status for debugging and monitoring
	if statusResp.Status == "failed" {
		logger.Errorf("Processing failed backup job %s: completed=%d, failed=%d, message=%s",
			jobID, statusResp.Completed, statusResp.Failed, statusResp.Message)
	} else {
		logger.Infof("Processing completed backup job %s: completed=%d, failed=%d, message=%s",
			jobID, statusResp.Completed, statusResp.Failed, statusResp.Message)
	}

	if notificationData, exists := jobInfo.ContextData["notification_data"]; exists {
		return processBackupResultsFromNotification(jobID, backupContext, notificationData, statusResp, jobInfo)
	}

	return processBackupResultsFromVeloArtifact(jobID, backupContext, statusResp, jobInfo)
}

// processBackupResultsFromNotification handles backup processing when triggered by external notification
func processBackupResultsFromNotification(jobID string, backupContext *BackupJobContext, notificationData interface{}, statusResp *job.StatusResponse, jobInfo *job.JobInfo) error {
	logger.Infof("Processing backup results from notification for job %s", jobID)

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

	logger.Infof("Processing notification-based backup results: job_id=%s, file=%s, md5=%s", jobID, fileName, md5Hash)

	// Get OS job IDs from context to determine the sub-job directory
	osJobIDs, ok := jobInfo.ContextData["os_job_ids"].([]string)
	if !ok || len(osJobIDs) == 0 {
		return fmt.Errorf("no OS job IDs found in context for master job %s", jobID)
	}

	// File path structure: NotificationFileDir/osJobID/md5Hash
	// For multiple OS jobs, try each until file is found
	var localFilePath string
	var foundJob string
	for _, osJobID := range osJobIDs {
		testPath := filepath.Join(config.Cfg.NotificationFileDir, osJobID, md5Hash)
		if _, err := os.Stat(testPath); err == nil {
			localFilePath = testPath
			foundJob = osJobID
			break
		}
	}

	if localFilePath == "" {
		return fmt.Errorf("backup file not found for any OS job in NotificationFileDir, md5=%s", md5Hash)
	}

	logger.Infof("Successfully confirmed backup file at %s for fileName=%s (os_job=%s)", localFilePath, fileName, foundJob)

	// Update master job results with file path
	jobMonitor := job.GetJobMonitorService()
	results := map[string]interface{}{
		"backup_file": map[string]interface{}{
			"fileName": fileName,
			"filePath": localFilePath,
			"md5Hash":  md5Hash,
			"osJobID":  foundJob,
			"status":   "completed",
		},
	}
	jobMonitor.UpdateJobResults(jobID, results)

	logger.Infof("Backup file path added to job %s results: %s", jobID, localFilePath)

	return nil
}

// processBackupResultsFromVeloArtifact handles backup processing using VeloArtifact polling flow
func processBackupResultsFromVeloArtifact(jobID string, backupContext *BackupJobContext, statusResp *job.StatusResponse, jobInfo *job.JobInfo) error {
	logger.Infof("Processing backup results from VeloArtifact polling for job %s", jobID)

	logger.Infof("Backup job completed via VeloArtifact polling: job_id=%s, status=%s, completed=%d, failed=%d",
		jobID, statusResp.Status, statusResp.Completed, statusResp.Failed)

	// Extract sql_step_results from context data and structure by step number
	if sqlStepResults, ok := jobInfo.ContextData["sql_step_results"].([]map[string]interface{}); ok && len(sqlStepResults) > 0 {
		results := make(map[string]interface{})

		for _, stepResult := range sqlStepResults {
			if stepOrder, ok := stepResult["step_order"].(int); ok {
				stepKey := fmt.Sprintf("step_%d", stepOrder)
				results[stepKey] = stepResult["result"]
			}
		}

		// Update job results with proper locking
		jobMonitor := job.GetJobMonitorService()
		jobMonitor.UpdateJobResults(jobID, results)

		logger.Infof("Populated %d step results for job %s", len(sqlStepResults), jobID)
	}

	resultsData := fmt.Sprintf("Job completed: status=%s, completed=%d, failed=%d, total=%d",
		statusResp.Status, statusResp.Completed, statusResp.Failed, statusResp.TotalQueries)

	return saveBackupResults(backupContext, resultsData)
}

// saveBackupResults saves backup job results to log
func saveBackupResults(backupContext *BackupJobContext, resultsData string) error {
	// CRITICAL: Parse resultsData to determine actual job status
	var status string
	if _, failed, _ := parseJobStatusFromResults(resultsData); failed > 0 {
		status = "PARTIALLY FAILED"
	} else {
		status = "COMPLETED"
	}

	logger.Infof("Backup job %s for job_id=%d, type=%s, file_name=%s",
		status, backupContext.JobID, backupContext.Type, backupContext.FileName)

	var results map[string][]map[string]interface{}
	if err := json.Unmarshal([]byte(resultsData), &results); err != nil {
		logger.Warnf("Backup results are not in JSON format, logging as plain text: %s", resultsData)
	} else {
		resultJSON, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			logger.Errorf("Failed to format backup results: %v", err)
		} else {
			logger.Infof("Backup results:\n%s", string(resultJSON))
		}
	}

	return nil
}

// parseJobStatusFromResults extracts completed, failed, and total counts from results data string
// Expected format: "Job completed: status=X, completed=Y, failed=Z, total=W"
func parseJobStatusFromResults(resultsData string) (completed, failed, total int) {
	// Default values
	completed, failed, total = 0, 0, 0

	// Parse using regex to extract numeric values
	re := regexp.MustCompile(`completed=(\d+),\s*failed=(\d+),\s*total=(\d+)`)
	matches := re.FindStringSubmatch(resultsData)
	if len(matches) == 4 {
		if val, err := strconv.Atoi(matches[1]); err == nil {
			completed = val
		}
		if val, err := strconv.Atoi(matches[2]); err == nil {
			failed = val
		}
		if val, err := strconv.Atoi(matches[3]); err == nil {
			total = val
		}
	}

	return completed, failed, total
}
