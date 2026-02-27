package entity

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"dbfartifactapi/config"
	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services/agent"
	"dbfartifactapi/services/job"
	"dbfartifactapi/utils"
)

// ObjectJobContext contains context data for object job completion
type ObjectJobContext struct {
	DBMgtID       uint                `json:"dbmgt_id"`
	ObjectQueries map[string][]string `json:"object_queries"`
	DBMgt         *models.DBMgt       `json:"dbmgt"`
	CMT           *models.CntMgt      `json:"cmt"`
	EndpointID    uint                `json:"endpoint_id"`
}

// ObjectResult represents a query result for object creation
type ObjectResult struct {
	QueryKey    string          `json:"query_key"`
	Query       string          `json:"query"`
	Status      string          `json:"status"`
	Result      [][]interface{} `json:"result"`
	ExecuteTime string          `json:"execute_time"`
	DurationMs  int             `json:"duration_ms"`
}

// objectGetResultsResponse represents the response from getresults command.
// Mirrors services.GetResultsResponse to avoid circular dependency.
type objectGetResultsResponse struct {
	Completed    int    `json:"completed"`
	Failed       int    `json:"failed"`
	FilePath     string `json:"file_path"`
	Message      string `json:"message"`
	Success      bool   `json:"success"`
	TotalQueries int    `json:"total_queries"`
}

// CreateObjectCompletionHandler creates a callback function for object job completion
func CreateObjectCompletionHandler() job.JobCompletionCallback {
	return func(jobID string, jobInfo *job.JobInfo, statusResp *job.StatusResponse) error {
		logger.Infof("Processing object completion for job %s, status: %s", jobID, statusResp.Status)

		// Extract context data from job
		contextData, ok := jobInfo.ContextData["object_context"]
		if !ok {
			return fmt.Errorf("missing object context data for job %s", jobID)
		}

		// Process completed jobs regardless of completion method (polling or notification)
		if statusResp.Status == "completed" {
			return processObjectResults(jobID, contextData, statusResp, jobInfo)
		} else {
			logger.Errorf("Object job %s failed, no objects will be created", jobID)
			return fmt.Errorf("object job failed: %s", statusResp.Message)
		}
	}
}

// processObjectResults processes the results of a completed object job
func processObjectResults(jobID string, contextData interface{}, statusResp *job.StatusResponse, jobInfo *job.JobInfo) error {
	logger.Infof("Processing object results for job %s - completed: %d, failed: %d",
		jobID, statusResp.Completed, statusResp.Failed)

	// Extract ObjectJobContext from contextData
	objectContext, ok := contextData.(*ObjectJobContext)
	if !ok {
		return fmt.Errorf("invalid object context data for job %s", jobID)
	}

	// Check if this is notification-based completion with file data
	if notificationData, exists := jobInfo.ContextData["notification_data"]; exists {
		return processObjectResultsFromNotification(jobID, objectContext, notificationData, statusResp)
	}

	// Legacy VeloArtifact polling flow
	return processObjectResultsFromVeloArtifact(jobID, objectContext, statusResp)
}

// getEndpointForObjectJob retrieves endpoint information for job processing
func getEndpointForObjectJob(jobID string, endpointID uint) (*models.Endpoint, error) {
	logger.Debugf("Getting endpoint for object job %s, endpoint ID: %d", jobID, endpointID)

	// Get endpoint from repository
	endpointRepo := repository.NewEndpointRepository()
	baseRepo := repository.NewBaseRepository()
	tx := baseRepo.Begin()
	defer tx.Rollback()

	ep, err := endpointRepo.GetByID(tx, endpointID)
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoint with id=%d for job %s: %w", endpointID, jobID, err)
	}

	logger.Debugf("Found endpoint for job %s: id=%d, client_id=%s, os_type=%s", jobID, ep.ID, ep.ClientID, ep.OsType)
	return ep, nil
}

// retrieveObjectJobResults retrieves and parses job results from VeloArtifact
func retrieveObjectJobResults(jobID string, endpoint *models.Endpoint) ([]ObjectResult, error) {
	logger.Debugf("Retrieving object results for job %s from endpoint %s", jobID, endpoint.ClientID)

	// Get results from agent using getresults command
	resultsOutput, err := agent.ExecuteAgentAPISimpleCommand(endpoint.ClientID, endpoint.OsType, "getresults", jobID, "", true)
	if err != nil {
		return nil, fmt.Errorf("failed to get results for job %s: %w", jobID, err)
	}

	// Parse getresults response
	var getResultsResp objectGetResultsResponse
	if err := json.Unmarshal([]byte(resultsOutput), &getResultsResp); err != nil {
		return nil, fmt.Errorf("failed to parse getresults response for job %s: %w", jobID, err)
	}

	if !getResultsResp.Success {
		return nil, fmt.Errorf("getresults failed for job %s: %s", jobID, getResultsResp.Message)
	}

	logger.Infof("Object job %s results: completed=%d, failed=%d, file=%s",
		jobID, getResultsResp.Completed, getResultsResp.Failed, getResultsResp.FilePath)

	// Download results file from agent
	downloadInfo, err := agent.DownloadFileAgentAPI(endpoint.ClientID, getResultsResp.FilePath, endpoint.OsType)
	if err != nil {
		return nil, fmt.Errorf("failed to download results file for job %s: %w", jobID, err)
	}

	// Use the local path from download response
	localFilePath := downloadInfo.LocalPath

	// Parse results file
	resultsData, err := parseObjectResultsFile(localFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse results file %s for job %s: %w", localFilePath, jobID, err)
	}

	logger.Infof("Successfully parsed %d object results from file %s", len(resultsData), localFilePath)
	return resultsData, nil
}

// parseObjectResultsFile reads and parses the downloaded JSON results file
func parseObjectResultsFile(filePath string) ([]ObjectResult, error) {
	logger.Debugf("Reading object results file: %s", filePath)

	// Read file content
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read results file %s: %w", filePath, err)
	}

	// Parse JSON array
	var results []ObjectResult
	if err := json.Unmarshal(fileData, &results); err != nil {
		return nil, fmt.Errorf("failed to unmarshal results from %s: %w", filePath, err)
	}

	logger.Debugf("Successfully parsed %d object results from file", len(results))
	return results, nil
}

// createObjectsFromResults processes query results and synchronizes objects atomically (insert + delete)
func createObjectsFromResults(jobID string, objectContext *ObjectJobContext, results []ObjectResult) (int, error) {
	logger.Infof("Synchronizing objects from %d results for job %s, dbmgt_id=%d", len(results), jobID, objectContext.DBMgtID)

	// Initialize repositories
	baseRepo := repository.NewBaseRepository()
	dbObjectMgtRepo := repository.NewDBObjectMgtRepository()

	// Start atomic transaction
	tx := baseRepo.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	insertCount := 0
	deleteCount := 0

	// Create object exception logger for unexpected outputs
	logFilePath := fmt.Sprintf("%s/object_sync_%s.log", config.Cfg.VeloResultsDir, jobID)
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		logger.Warnf("Failed to create object sync log file: %v", err)
	}
	var objectLogger *log.Logger
	if logFile != nil {
		defer logFile.Close()
		objectLogger = log.New(logFile, "", log.LstdFlags)
		objectLogger.Printf("Starting object synchronization for job %s with %d results", jobID, len(results))
	}

	// Step 1: Collect all objects from VeloArtifact results by object type
	remoteObjectsByType := make(map[uint]map[string]bool) // objectType -> objectName -> exists

	for _, result := range results {
		if result.Status != "success" {
			logger.Warnf("Skipping failed query %s: %s", result.QueryKey, result.Status)
			continue
		}

		// Parse object type from query key (format: "ObjectType:ID_QueryIndex")
		objectType, err := parseObjectTypeFromKey(result.QueryKey)
		if err != nil {
			logger.Errorf("Failed to parse object type from key %s: %v", result.QueryKey, err)
			continue
		}

		objectTypeUint := utils.MustIntToUint(objectType)
		if remoteObjectsByType[objectTypeUint] == nil {
			remoteObjectsByType[objectTypeUint] = make(map[string]bool)
		}

		// Process each row in the result
		for _, row := range result.Result {
			if len(row) > 0 {
				// Extract object name from first column
				objectName := fmt.Sprintf("%v", row[0])
				if objectName == "" || objectName == "<nil>" {
					continue
				}
				remoteObjectsByType[objectTypeUint][objectName] = true
			}
		}
	}

	logger.Infof("Collected remote objects from %d object types for synchronization", len(remoteObjectsByType))

	// Step 2: Process each object type for synchronization
	for objectType, remoteObjects := range remoteObjectsByType {
		logger.Debugf("Synchronizing object type %d: %d remote objects", objectType, len(remoteObjects))

		// Get existing objects of this type from database
		existingObjects, err := dbObjectMgtRepo.GetByDbMgtAndObjectId(tx, objectContext.DBMgtID, objectType)
		if err != nil {
			logger.Errorf("Failed to get existing objects for type %d: %v", objectType, err)
			continue
		}

		logger.Debugf("Found %d existing objects of type %d in database", len(existingObjects), objectType)

		// Step 2a: Insert missing objects (exist in remote but not in database)
		for objectName := range remoteObjects {
			// Check if object already exists in database
			found := false
			for _, existing := range existingObjects {
				if existing.ObjectName == objectName {
					found = true
					break
				}
			}

			if !found {
				// Create new DBObjectMgt record
				dbObjectMgt := models.DBObjectMgt{
					DBMgt:       objectContext.DBMgtID,
					ObjectName:  objectName,
					ObjectId:    objectType,
					Description: "Auto-collected from V2-DBF Agent",
					Status:      "enabled",
					SqlParam:    "", // Empty for Auto-collected objects
				}

				if err := tx.Create(&dbObjectMgt).Error; err != nil {
					tx.Rollback()
					return 0, fmt.Errorf("failed to create object record for %s (type %d): %w", objectName, objectType, err)
				}

				insertCount++
				logger.Debugf("Inserted object: dbmgt=%d, type=%d, name=%s, id=%d", objectContext.DBMgtID, objectType, objectName, dbObjectMgt.ID)

				if objectLogger != nil {
					objectLogger.Printf("INSERT: dbmgt=%d, type=%d, name=%s", objectContext.DBMgtID, objectType, objectName)
				}
			}
		}

		// Step 2b: Delete obsolete objects (exist in database but not in remote)
		for _, existing := range existingObjects {
			// Check if object still exists in remote
			if !remoteObjects[existing.ObjectName] {
				// Delete from database
				if err := tx.Delete(&existing).Error; err != nil {
					tx.Rollback()
					return 0, fmt.Errorf("failed to delete obsolete object %s (type %d): %w", existing.ObjectName, objectType, err)
				}

				deleteCount++
				logger.Debugf("Deleted obsolete object: dbmgt=%d, type=%d, name=%s, id=%d", objectContext.DBMgtID, objectType, existing.ObjectName, existing.ID)

				if objectLogger != nil {
					objectLogger.Printf("DELETE: dbmgt=%d, type=%d, name=%s (no longer exists in remote database)", objectContext.DBMgtID, objectType, existing.ObjectName)
				}
			}
		}
	}

	// Commit transaction atomically
	if err := tx.Commit().Error; err != nil {
		return 0, fmt.Errorf("failed to commit object synchronization transaction for job %s: %w", jobID, err)
	}

	logger.Infof("Successfully synchronized objects for job %s: +%d inserted, -%d deleted", jobID, insertCount, deleteCount)
	return insertCount + deleteCount, nil
}

// parseObjectTypeFromKey extracts object type from various query key formats:
// - Simple: "ObjectType:1[0]" or "ObjectType:15[0]"
// - Complex: "ObjectType:6_DependentIndex:1[0]" or "ObjectType:6_DependentIndex:4[0]"
// - Underscore: "ObjectType:1_2" or "ObjectType:15_3"
func parseObjectTypeFromKey(queryKey string) (int, error) {
	// Split by colon first
	parts := strings.Split(queryKey, ":")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid query key format: %s", queryKey)
	}

	var objectIDPart string

	// Handle different formats:
	// 1. Simple: "ObjectType:1[0]" → parts = ["ObjectType", "1[0]"]
	// 2. Complex: "ObjectType:6_DependentIndex:1[0]" → parts = ["ObjectType", "6_DependentIndex", "1[0]"]
	if len(parts) == 2 {
		// Simple format: "ObjectType:1[0]" or "ObjectType:1_2"
		objectIDPart = parts[1]
	} else if len(parts) >= 3 {
		// Complex format: "ObjectType:6_DependentIndex:1[0]"
		// The object ID is in parts[1] before "_DependentIndex"
		if strings.Contains(parts[1], "_DependentIndex") {
			// Extract object ID from "6_DependentIndex"
			dependentParts := strings.Split(parts[1], "_")
			objectIDPart = dependentParts[0] // Get "6"
		} else {
			// Fallback: use parts[1]
			objectIDPart = parts[1]
		}
	}

	// Check if there's a bracket notation like "1[0]"
	if strings.Contains(objectIDPart, "[") {
		// Extract everything before the first bracket
		bracketIndex := strings.Index(objectIDPart, "[")
		objectIDPart = objectIDPart[:bracketIndex]
	} else {
		// Handle underscore format: "ObjectType:ID_QueryIndex"
		idParts := strings.Split(objectIDPart, "_")
		if len(idParts) >= 1 {
			objectIDPart = idParts[0]
		}
	}

	// Parse object type as integer
	objectType, err := strconv.Atoi(objectIDPart)
	if err != nil {
		return 0, fmt.Errorf("failed to parse object type from key %s: %w", queryKey, err)
	}

	return objectType, nil
}

// processObjectResultsFromNotification handles object processing when triggered by external notification
func processObjectResultsFromNotification(jobID string, objectContext *ObjectJobContext, notificationData interface{}, statusResp *job.StatusResponse) error {
	logger.Infof("Processing object results from notification for job %s", jobID)

	jobMonitor := job.GetJobMonitorService()

	// Extract notification data
	notification, ok := notificationData.(map[string]interface{})
	if !ok {
		err := fmt.Errorf("invalid notification data format for job %s", jobID)
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	fileName, ok := notification["fileName"].(string)
	if !ok {
		err := fmt.Errorf("missing fileName in notification data for job %s", jobID)
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	md5Hash, ok := notification["md5Hash"].(string)
	if !ok {
		err := fmt.Errorf("missing md5Hash in notification data for job %s", jobID)
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	success, ok := notification["success"].(bool)
	if !ok || !success {
		err := fmt.Errorf("job %s was not successful according to notification", jobID)
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	logger.Infof("Processing notification-based object results: job_id=%s, file=%s, md5=%s", jobID, fileName, md5Hash)

	// Construct local file path using md5Hash from notification
	localFilePath := fmt.Sprintf("%s/%s/%s", config.Cfg.NotificationFileDir, jobID, md5Hash)
	logger.Infof("Processing object results from notification file: %s", localFilePath)

	// Read and parse the already downloaded file
	resultsData, err := parseObjectResultsFile(localFilePath)
	if err != nil {
		errMsg := fmt.Sprintf("failed to parse notification results file for job %s: %v", jobID, err)
		jobMonitor.FailJobAfterProcessing(jobID, errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	logger.Infof("Successfully parsed %d query results from notification file for job %s", len(resultsData), jobID)

	// Process results and create objects atomically
	insertCount, err := createObjectsFromResults(jobID, objectContext, resultsData)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	// Mark job as completed after all server-side processing is done
	successMsg := fmt.Sprintf("Object synchronization completed successfully - changed %d objects", insertCount)
	if err := jobMonitor.CompleteJobAfterProcessing(jobID, successMsg); err != nil {
		logger.Errorf("Failed to mark job as completed: %v", err)
	}

	logger.Infof("Object notification handler executed successfully for job %s - changed %d objects", jobID, insertCount)
	return nil
}

// processObjectResultsFromVeloArtifact handles object processing via traditional VeloArtifact polling
func processObjectResultsFromVeloArtifact(jobID string, objectContext *ObjectJobContext, statusResp *job.StatusResponse) error {
	logger.Infof("Processing object results from VeloArtifact polling for job %s", jobID)

	jobMonitor := job.GetJobMonitorService()

	// Get endpoint information
	ep, err := getEndpointForObjectJob(jobID, objectContext.EndpointID)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	// Retrieve and download results file via VeloArtifact
	resultsData, err := retrieveObjectJobResults(jobID, ep)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	logger.Infof("Successfully retrieved %d query results via VeloArtifact for job %s", len(resultsData), jobID)

	// Process results and create objects atomically
	insertCount, err := createObjectsFromResults(jobID, objectContext, resultsData)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	// Mark job as completed after all server-side processing is done
	successMsg := fmt.Sprintf("Object synchronization completed successfully - changed %d objects", insertCount)
	if err := jobMonitor.CompleteJobAfterProcessing(jobID, successMsg); err != nil {
		logger.Errorf("Failed to mark job as completed: %v", err)
	}

	logger.Infof("Object VeloArtifact handler executed successfully for job %s - changed %d objects", jobID, insertCount)
	return nil
}

// CreateCombinedObjectCompletionHandler creates a callback function for combined object job completion
func CreateCombinedObjectCompletionHandler() job.JobCompletionCallback {
	return func(jobID string, jobInfo *job.JobInfo, statusResp *job.StatusResponse) error {
		logger.Infof("Processing combined object completion for job %s, status: %s", jobID, statusResp.Status)

		// Extract context data from job
		contextData, ok := jobInfo.ContextData["combined_object_context"]
		if !ok {
			return fmt.Errorf("missing combined object context data for job %s", jobID)
		}

		if statusResp.Status == "completed" {
			return processCombinedObjectResults(jobID, contextData, statusResp, jobInfo)
		} else {
			logger.Errorf("Combined object job %s failed, no objects will be created", jobID)
			return fmt.Errorf("combined object job failed: %s", statusResp.Message)
		}
	}
}

// processCombinedObjectResults processes the results of a completed combined object job
func processCombinedObjectResults(jobID string, contextData interface{}, statusResp *job.StatusResponse, jobInfo *job.JobInfo) error {
	logger.Infof("Processing combined object results for job %s - completed: %d, failed: %d",
		jobID, statusResp.Completed, statusResp.Failed)

	// Extract CombinedObjectJobContext from contextData
	combinedContext, ok := contextData.(*CombinedObjectJobContext)
	if !ok {
		return fmt.Errorf("invalid combined object context data for job %s", jobID)
	}

	// Check if this is notification-based completion with file data
	if notificationData, exists := jobInfo.ContextData["notification_data"]; exists {
		return processCombinedObjectResultsFromNotification(jobID, combinedContext, notificationData, statusResp)
	}

	// Legacy VeloArtifact polling flow
	return processCombinedObjectResultsFromVeloArtifact(jobID, combinedContext, statusResp)
}

// createCombinedObjectsFromResults processes combined query results and synchronizes objects atomically across all databases
func createCombinedObjectsFromResults(jobID string, combinedContext *CombinedObjectJobContext, results []ObjectResult) (int, error) {
	logger.Infof("Synchronizing combined objects from %d results for job %s, cntmgt_id=%d, databases=%d",
		len(results), jobID, combinedContext.CntMgtID, len(combinedContext.DbMgts))

	// Initialize repositories
	baseRepo := repository.NewBaseRepository()
	dbObjectMgtRepo := repository.NewDBObjectMgtRepository()

	// Start atomic transaction for all databases
	tx := baseRepo.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	totalInserts := 0
	totalDeletes := 0

	// Create object sync logger for tracking changes
	logFilePath := fmt.Sprintf("%s/combined_object_sync_%s.log", config.Cfg.VeloResultsDir, jobID)
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		logger.Warnf("Failed to create combined object sync log file: %v", err)
	}
	var objectLogger *log.Logger
	if logFile != nil {
		defer logFile.Close()
		objectLogger = log.New(logFile, "", log.LstdFlags)
		objectLogger.Printf("Starting combined object synchronization for job %s with %d results across %d databases",
			jobID, len(results), len(combinedContext.DbMgts))
	}

	// Step 1: Collect all objects from VeloArtifact results by database and object type
	remoteObjectsByDB := make(map[uint]map[uint]map[string]bool) // dbmgt_id -> objectType -> objectName -> exists

	for _, result := range results {
		if result.Status != "success" {
			logger.Warnf("Skipping failed query %s: %s", result.QueryKey, result.Status)
			continue
		}

		// Parse database ID and object type from combined query key (format: "DB:dbmgt_id_ObjectType:object_id_QueryIndex")
		dbMgtID, objectType, err := parseCombinedQueryKey(result.QueryKey)
		if err != nil {
			logger.Errorf("Failed to parse combined query key %s: %v", result.QueryKey, err)
			continue
		}

		objectTypeUint := utils.MustIntToUint(objectType)

		// Initialize nested maps if needed
		if remoteObjectsByDB[dbMgtID] == nil {
			remoteObjectsByDB[dbMgtID] = make(map[uint]map[string]bool)
		}
		if remoteObjectsByDB[dbMgtID][objectTypeUint] == nil {
			remoteObjectsByDB[dbMgtID][objectTypeUint] = make(map[string]bool)
		}

		// Process each row in the result
		for _, row := range result.Result {
			if len(row) > 0 {
				// Extract object name from first column
				objectName := fmt.Sprintf("%v", row[0])
				if objectName == "" || objectName == "<nil>" {
					continue
				}
				remoteObjectsByDB[dbMgtID][objectTypeUint][objectName] = true
			}
		}
	}

	logger.Infof("Collected remote objects from %d databases for combined synchronization", len(remoteObjectsByDB))

	// Step 2: Process each database for synchronization
	for dbMgtID, remoteObjectsByType := range remoteObjectsByDB {
		logger.Debugf("Synchronizing database %d with %d object types", dbMgtID, len(remoteObjectsByType))

		// Process each object type within this database
		for objectType, remoteObjects := range remoteObjectsByType {
			logger.Debugf("Synchronizing db=%d, object type %d: %d remote objects", dbMgtID, objectType, len(remoteObjects))

			// Get existing objects of this type from database
			existingObjects, err := dbObjectMgtRepo.GetByDbMgtAndObjectId(tx, dbMgtID, objectType)
			if err != nil {
				logger.Errorf("Failed to get existing objects for db=%d, type=%d: %v", dbMgtID, objectType, err)
				continue
			}

			logger.Debugf("Found %d existing objects of type %d in database %d", len(existingObjects), objectType, dbMgtID)

			// Step 2a: Insert missing objects (exist in remote but not in database)
			for objectName := range remoteObjects {
				// Check if object already exists in database
				found := false
				for _, existing := range existingObjects {
					if existing.ObjectName == objectName {
						found = true
						break
					}
				}

				if !found {
					// Create new DBObjectMgt record for this specific database
					dbObjectMgt := models.DBObjectMgt{
						DBMgt:       dbMgtID,
						ObjectName:  objectName,
						ObjectId:    objectType,
						Description: "Auto-collected from V2-DBF Agent",
						Status:      "enabled",
						SqlParam:    "", // Empty for Auto-collected objects
					}

					if err := tx.Create(&dbObjectMgt).Error; err != nil {
						tx.Rollback()
						return 0, fmt.Errorf("failed to create combined object record for %s (db=%d, type=%d): %w",
							objectName, dbMgtID, objectType, err)
					}

					totalInserts++
					logger.Debugf("Inserted combined object: dbmgt=%d, type=%d, name=%s, id=%d",
						dbMgtID, objectType, objectName, dbObjectMgt.ID)

					if objectLogger != nil {
						objectLogger.Printf("INSERT: dbmgt=%d, type=%d, name=%s", dbMgtID, objectType, objectName)
					}
				}
			}

			// Step 2b: Delete obsolete objects (exist in database but not in remote)
			for _, existing := range existingObjects {
				// Check if object still exists in remote
				if !remoteObjects[existing.ObjectName] {
					// Delete from database
					if err := tx.Delete(&existing).Error; err != nil {
						tx.Rollback()
						return 0, fmt.Errorf("failed to delete obsolete combined object %s (db=%d, type=%d): %w",
							existing.ObjectName, dbMgtID, objectType, err)
					}

					totalDeletes++
					logger.Debugf("Deleted obsolete combined object: dbmgt=%d, type=%d, name=%s, id=%d",
						dbMgtID, objectType, existing.ObjectName, existing.ID)

					if objectLogger != nil {
						objectLogger.Printf("DELETE: dbmgt=%d, type=%d, name=%s (no longer exists in remote database)",
							dbMgtID, objectType, existing.ObjectName)
					}
				}
			}
		}
	}

	// Commit transaction atomically for all databases
	if err := tx.Commit().Error; err != nil {
		return 0, fmt.Errorf("failed to commit combined object synchronization transaction for job %s: %w", jobID, err)
	}

	logger.Infof("Successfully synchronized objects across %d databases for job %s: +%d inserted, -%d deleted",
		len(combinedContext.DbMgts), jobID, totalInserts, totalDeletes)
	return totalInserts + totalDeletes, nil
}

// parseCombinedQueryKey extracts database ID and object type from combined query key format "DB:dbmgt_id_ObjectType:object_id_QueryIndex"
func parseCombinedQueryKey(queryKey string) (uint, int, error) {
	// Split by underscore to separate DB part and ObjectType part
	parts := strings.Split(queryKey, "_")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("invalid combined query key format: %s", queryKey)
	}

	// Extract dbmgt_id from "DB:dbmgt_id" part
	dbPart := parts[0]
	if !strings.HasPrefix(dbPart, "DB:") {
		return 0, 0, fmt.Errorf("invalid DB prefix in query key: %s", queryKey)
	}

	dbMgtIDStr := strings.TrimPrefix(dbPart, "DB:")
	dbMgtID, err := strconv.ParseUint(dbMgtIDStr, 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse dbmgt_id from key %s: %w", queryKey, err)
	}

	// Extract object_id from "ObjectType:object_id" part
	objectPart := parts[1]
	if !strings.HasPrefix(objectPart, "ObjectType:") {
		return 0, 0, fmt.Errorf("invalid ObjectType prefix in query key: %s", queryKey)
	}

	objectIDStr := strings.TrimPrefix(objectPart, "ObjectType:")

	// Handle VeloArtifact format with [index]: "ObjectType:1[0]"
	if strings.Contains(objectIDStr, "[") {
		// Extract everything before the first bracket
		bracketIndex := strings.Index(objectIDStr, "[")
		objectIDStr = objectIDStr[:bracketIndex]
	}

	objectID, err := strconv.Atoi(objectIDStr)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse object_id from key %s: %w", queryKey, err)
	}

	return uint(dbMgtID), objectID, nil
}

// processCombinedObjectResultsFromNotification handles combined object processing when triggered by external notification
func processCombinedObjectResultsFromNotification(jobID string, combinedContext *CombinedObjectJobContext, notificationData interface{}, statusResp *job.StatusResponse) error {
	logger.Infof("Processing combined object results from notification for job %s", jobID)

	jobMonitor := job.GetJobMonitorService()

	// Extract notification data
	notification, ok := notificationData.(map[string]interface{})
	if !ok {
		err := fmt.Errorf("invalid notification data format for job %s", jobID)
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	fileName, ok := notification["fileName"].(string)
	if !ok {
		err := fmt.Errorf("missing fileName in notification data for job %s", jobID)
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	md5Hash, ok := notification["md5Hash"].(string)
	if !ok {
		err := fmt.Errorf("missing md5Hash in notification data for job %s", jobID)
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	success, ok := notification["success"].(bool)
	if !ok || !success {
		err := fmt.Errorf("job %s was not successful according to notification", jobID)
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	logger.Infof("Processing notification-based combined object results: job_id=%s, file=%s, md5=%s", jobID, fileName, md5Hash)

	// Construct local file path using md5Hash from notification
	localFilePath := fmt.Sprintf("%s/%s/%s", config.Cfg.NotificationFileDir, jobID, md5Hash)
	logger.Infof("Processing combined object results from notification file: %s", localFilePath)

	// Read and parse the already downloaded file
	resultsData, err := parseObjectResultsFile(localFilePath)
	if err != nil {
		errMsg := fmt.Sprintf("failed to parse notification results file for job %s: %v", jobID, err)
		jobMonitor.FailJobAfterProcessing(jobID, errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	logger.Infof("Successfully parsed %d combined query results from notification file for job %s", len(resultsData), jobID)

	// Process results and create objects atomically for all databases
	totalInserts, err := createCombinedObjectsFromResults(jobID, combinedContext, resultsData)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	// Mark job as completed after all server-side processing is done
	successMsg := fmt.Sprintf("Combined object synchronization completed successfully - changed %d objects across %d databases",
		totalInserts, len(combinedContext.DbMgts))
	if err := jobMonitor.CompleteJobAfterProcessing(jobID, successMsg); err != nil {
		logger.Errorf("Failed to mark job as completed: %v", err)
	}

	logger.Infof("Combined object notification handler executed successfully for job %s - changed %d objects across %d databases",
		jobID, totalInserts, len(combinedContext.DbMgts))
	return nil
}

// processCombinedObjectResultsFromVeloArtifact handles combined object processing via traditional VeloArtifact polling
func processCombinedObjectResultsFromVeloArtifact(jobID string, combinedContext *CombinedObjectJobContext, statusResp *job.StatusResponse) error {
	logger.Infof("Processing combined object results from VeloArtifact polling for job %s", jobID)

	jobMonitor := job.GetJobMonitorService()

	// Get endpoint information
	ep, err := getEndpointForObjectJob(jobID, combinedContext.EndpointID)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	// Retrieve and download results file via VeloArtifact
	resultsData, err := retrieveObjectJobResults(jobID, ep)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	logger.Infof("Successfully retrieved %d combined query results via VeloArtifact for job %s", len(resultsData), jobID)

	// Process results and create objects atomically for all databases
	totalInserts, err := createCombinedObjectsFromResults(jobID, combinedContext, resultsData)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	// Mark job as completed after all server-side processing is done
	successMsg := fmt.Sprintf("Combined object synchronization completed successfully - changed %d objects across %d databases",
		totalInserts, len(combinedContext.DbMgts))
	if err := jobMonitor.CompleteJobAfterProcessing(jobID, successMsg); err != nil {
		logger.Errorf("Failed to mark job as completed: %v", err)
	}

	logger.Infof("Combined object VeloArtifact handler executed successfully for job %s - changed %d objects across %d databases",
		jobID, totalInserts, len(combinedContext.DbMgts))
	return nil
}
