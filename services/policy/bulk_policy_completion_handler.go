package policy

import (
	"fmt"
	"log"
	"os"

	"dbfartifactapi/config"
	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services/dto"
	"dbfartifactapi/services/job"
	"dbfartifactapi/utils"
)

// CreateBulkPolicyUpdateCompletionHandler creates a callback function for bulk policy update job completion
// Processes VeloArtifact results and atomically updates database with policy changes
func CreateBulkPolicyUpdateCompletionHandler() job.JobCompletionCallback {
	return func(jobID string, jobInfo *job.JobInfo, statusResp *job.StatusResponse) error {
		logger.Infof("Processing bulk policy update completion for job %s, status: %s", jobID, statusResp.Status)

		// Extract context data from job
		contextData, ok := jobInfo.ContextData["bulk_policy_context"]
		if !ok {
			return fmt.Errorf("missing bulk policy context data for job %s", jobID)
		}

		// Process completed jobs regardless of completion method (polling or notification)
		if statusResp.Status == "completed" {
			return processBulkPolicyUpdateResults(jobID, contextData, statusResp, jobInfo)
		} else {
			logger.Errorf("Bulk policy update job %s failed, no database changes will be applied", jobID)
			return fmt.Errorf("bulk policy update job failed: %s", statusResp.Message)
		}
	}
}

// processBulkPolicyUpdateResults processes the results of a completed bulk policy update job
func processBulkPolicyUpdateResults(jobID string, contextData interface{}, statusResp *job.StatusResponse, jobInfo *job.JobInfo) error {
	logger.Infof("Processing bulk policy update results for job %s - completed: %d, failed: %d",
		jobID, statusResp.Completed, statusResp.Failed)

	// Extract BulkPolicyUpdateJobContext from contextData
	bulkContext, ok := contextData.(*dto.BulkPolicyUpdateJobContext)
	if !ok {
		return fmt.Errorf("invalid bulk policy context data for job %s", jobID)
	}

	// Check if this is notification-based completion with file data
	if notificationData, exists := jobInfo.ContextData["notification_data"]; exists {
		return processBulkPolicyUpdateResultsFromNotification(jobID, bulkContext, notificationData, statusResp)
	}

	// Legacy VeloArtifact polling flow
	return processBulkPolicyUpdateResultsFromVeloArtifact(jobID, bulkContext, statusResp)
}

// processBulkPolicyUpdateResultsFromNotification handles bulk policy update processing when triggered by external notification
func processBulkPolicyUpdateResultsFromNotification(jobID string, bulkContext *dto.BulkPolicyUpdateJobContext, notificationData interface{}, statusResp *job.StatusResponse) error {
	logger.Infof("Processing bulk policy update results from notification for job %s", jobID)

	// Extract notification data
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

	logger.Infof("Processing notification-based bulk policy update results: job_id=%s, file=%s, md5=%s", jobID, fileName, md5Hash)

	// Construct local file path using md5Hash from notification
	localFilePath := fmt.Sprintf("%s/%s/%s", config.Cfg.NotificationFileDir, jobID, md5Hash)
	logger.Infof("Processing bulk policy update results from notification file: %s", localFilePath)

	// Read and parse the already downloaded file
	resultsData, err := ParseResultsFile(localFilePath)
	if err != nil {
		return fmt.Errorf("failed to parse notification results file for job %s: %v", jobID, err)
	}

	logger.Infof("Successfully parsed %d command results from notification file for job %s", len(resultsData), jobID)

	// Validate results and apply database changes
	addedCount, removedCount, err := applyBulkPolicyUpdates(jobID, bulkContext, resultsData)
	if err != nil {
		return err
	}

	logger.Infof("Bulk policy update notification handler executed successfully for job %s - added %d policies, removed %d policies",
		jobID, addedCount, removedCount)
	return nil
}

// processBulkPolicyUpdateResultsFromVeloArtifact handles bulk policy update processing via traditional VeloArtifact polling
func processBulkPolicyUpdateResultsFromVeloArtifact(jobID string, bulkContext *dto.BulkPolicyUpdateJobContext, statusResp *job.StatusResponse) error {
	logger.Infof("Processing bulk policy update results from VeloArtifact polling for job %s", jobID)

	// Get endpoint information
	ep, err := GetEndpointForJob(jobID, bulkContext.EndpointID)
	if err != nil {
		return err
	}

	// Retrieve and download results file via VeloArtifact
	resultsData, err := RetrieveJobResults(jobID, ep)
	if err != nil {
		return err
	}

	logger.Infof("Successfully retrieved %d command results via VeloArtifact for job %s", len(resultsData), jobID)

	// Validate results and apply database changes
	addedCount, removedCount, err := applyBulkPolicyUpdates(jobID, bulkContext, resultsData)
	if err != nil {
		return err
	}

	logger.Infof("Bulk policy update VeloArtifact handler executed successfully for job %s - added %d policies, removed %d policies",
		jobID, addedCount, removedCount)
	return nil
}

// applyBulkPolicyUpdates validates command execution results and applies database changes atomically
func applyBulkPolicyUpdates(jobID string, bulkContext *dto.BulkPolicyUpdateJobContext, resultsData []QueryResult) (int, int, error) {
	logger.Infof("Applying bulk policy updates: job_id=%s, command_results=%d, policies_to_add=%d, policies_to_remove=%d",
		jobID, len(resultsData), len(bulkContext.PolicesToAdd), len(bulkContext.PolicesToRemove))

	// Create audit logger for tracking changes
	logFilePath := fmt.Sprintf("%s/bulk_policy_update_%s.log", config.Cfg.VeloResultsDir, jobID)
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		logger.Warnf("Failed to create bulk policy update log file: %v", err)
	}
	var auditLogger *log.Logger
	if logFile != nil {
		defer logFile.Close()
		auditLogger = log.New(logFile, "", log.LstdFlags)
		auditLogger.Printf("Starting bulk policy update for job %s: %d commands executed, %d to add, %d to remove",
			jobID, len(resultsData), len(bulkContext.PolicesToAdd), len(bulkContext.PolicesToRemove))
	}

	// Step 1: Validate all commands succeeded
	failedCommands := make(map[string]string)
	successfulCommands := make(map[string]bool)

	for _, result := range resultsData {
		cleanKey := result.QueryKey
		for bracketIndex := 0; bracketIndex < len(cleanKey); bracketIndex++ {
			if cleanKey[bracketIndex] == '[' {
				cleanKey = cleanKey[:bracketIndex]
				break
			}
		}

		if result.Status != "success" {
			failedCommands[cleanKey] = fmt.Sprintf("status=%s, query=%s", result.Status, result.Query)
			if auditLogger != nil {
				auditLogger.Printf("FAILED: %s - %s", cleanKey, result.Query)
			}
		} else {
			successfulCommands[cleanKey] = true
		}
	}

	// Critical business rule: All commands must succeed for atomic consistency
	if len(failedCommands) > 0 {
		errMsg := fmt.Sprintf("bulk policy update aborted - %d commands failed out of %d total", len(failedCommands), len(resultsData))
		logger.Errorf("%s for job %s", errMsg, jobID)

		if auditLogger != nil {
			auditLogger.Printf("ROLLBACK: %s", errMsg)
			for key, details := range failedCommands {
				auditLogger.Printf("FAILED COMMAND: %s - %s", key, details)
			}
		}

		return 0, 0, fmt.Errorf("%s - details logged to %s", errMsg, logFilePath)
	}

	logger.Infof("All %d commands executed successfully - proceeding with database updates", len(resultsData))

	// Step 2: Create new transaction for atomic database updates
	baseRepo := repository.NewBaseRepository()
	tx := baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	dbPolicyRepo := repository.NewDBPolicyRepository()

	// Step 3: Delete policies that were removed (revoke permissions)
	var removedCount int
	if len(bulkContext.PolicesToRemove) > 0 {
		policyIDsToDelete := make([]uint, 0, len(bulkContext.PolicesToRemove))
		for _, combo := range bulkContext.PolicesToRemove {
			if combo.DBPolicyID != 0 {
				policyIDsToDelete = append(policyIDsToDelete, combo.DBPolicyID)
			}
		}

		if len(policyIDsToDelete) > 0 {
			if err := dbPolicyRepo.BulkDelete(tx, policyIDsToDelete); err != nil {
				logger.Errorf("Failed to bulk delete policies for job %s: %v", jobID, err)
				if auditLogger != nil {
					auditLogger.Printf("ROLLBACK: Failed to delete %d policies - %v", len(policyIDsToDelete), err)
				}
				return 0, 0, fmt.Errorf("failed to delete policies: %v", err)
			}
			removedCount = len(policyIDsToDelete)

			if auditLogger != nil {
				auditLogger.Printf("DELETED: %d policies", removedCount)
				for _, policyID := range policyIDsToDelete {
					auditLogger.Printf("DELETE: policy_id=%d", policyID)
				}
			}

			logger.Infof("Successfully deleted %d policies for job %s", removedCount, jobID)
		}
	}

	// Step 4: Create new policies (grant permissions)
	var addedCount int
	if len(bulkContext.PolicesToAdd) > 0 {
		policiesToCreate := make([]models.DBPolicy, 0, len(bulkContext.PolicesToAdd))
		for _, combo := range bulkContext.PolicesToAdd {
			policy := models.DBPolicy{
				CntMgt:          bulkContext.CntMgtID,
				DBMgt:           utils.MustUintToInt(bulkContext.DBMgtID),
				DBActorMgt:      bulkContext.DBActorMgtID,
				DBPolicyDefault: combo.PolicyDefaultID,
				DBObjectMgt:     utils.MustUintToInt(combo.ObjectMgtID),
				Status:          "enabled",
				Description:     "Added via bulk policy update",
			}
			policiesToCreate = append(policiesToCreate, policy)
		}

		if len(policiesToCreate) > 0 {
			if err := dbPolicyRepo.BulkCreate(tx, policiesToCreate); err != nil {
				logger.Errorf("Failed to bulk create policies for job %s: %v", jobID, err)
				if auditLogger != nil {
					auditLogger.Printf("ROLLBACK: Failed to create %d policies - %v", len(policiesToCreate), err)
				}
				return 0, 0, fmt.Errorf("failed to create policies: %v", err)
			}
			addedCount = len(policiesToCreate)

			if auditLogger != nil {
				auditLogger.Printf("CREATED: %d policies", addedCount)
				for i := range policiesToCreate {
					combo := bulkContext.PolicesToAdd[i]
					auditLogger.Printf("CREATE: policy_default_id=%d, object_id=%d, actor_id=%d",
						combo.PolicyDefaultID, combo.ObjectMgtID, bulkContext.DBActorMgtID)
				}
			}

			logger.Infof("Successfully created %d policies for job %s", addedCount, jobID)
		}
	}

	// Step 5: Commit transaction atomically
	if err := tx.Commit().Error; err != nil {
		logger.Errorf("Failed to commit bulk policy updates for job %s: %v", jobID, err)
		if auditLogger != nil {
			auditLogger.Printf("ROLLBACK: Commit failed - %v", err)
		}
		return 0, 0, fmt.Errorf("failed to commit bulk policy updates: %v", err)
	}
	txCommitted = true

	logger.Infof("Bulk policy update completed: job_id=%s, added=%d, removed=%d, actor_id=%d",
		jobID, addedCount, removedCount, bulkContext.DBActorMgtID)

	if auditLogger != nil {
		auditLogger.Printf("SUCCESS: Bulk policy update completed - added %d, removed %d policies for actor_id=%d",
			addedCount, removedCount, bulkContext.DBActorMgtID)
	}

	return addedCount, removedCount, nil
}
