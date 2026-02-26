package services

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
	"dbfartifactapi/utils"

	"gorm.io/gorm"
)

// PolicyJobContext and policyInput are defined in dbpolicy_service.go to avoid circular imports

// GetResultsResponse represents the response from getresults command
type GetResultsResponse struct {
	Completed    int    `json:"completed"`
	Failed       int    `json:"failed"`
	FilePath     string `json:"file_path"`
	Message      string `json:"message"`
	Success      bool   `json:"success"`
	TotalQueries int    `json:"total_queries"`
}

// CreatePolicyCompletionHandler creates a callback function for policy job completion
func CreatePolicyCompletionHandler() JobCompletionCallback {
	return func(jobID string, jobInfo *JobInfo, statusResp *StatusResponse) error {
		logger.Infof("Processing policy completion for job %s, status: %s", jobID, statusResp.Status)

		// Extract context data from job
		contextData, ok := jobInfo.ContextData["policy_context"]
		if !ok {
			return fmt.Errorf("missing policy context data for job %s", jobID)
		}

		// Process completed jobs regardless of completion method (polling or notification)
		if statusResp.Status == "completed" {
			return processPolicyResults(jobID, contextData, statusResp, jobInfo)
		} else {
			logger.Errorf("Policy job %s failed, no policies will be created", jobID)
			return fmt.Errorf("policy job failed: %s", statusResp.Message)
		}
	}
}

// processPolicyResults processes the results of a completed policy job
func processPolicyResults(jobID string, contextData interface{}, statusResp *StatusResponse, jobInfo *JobInfo) error {
	logger.Infof("Processing policy results for job %s - completed: %d, failed: %d",
		jobID, statusResp.Completed, statusResp.Failed)

	// Extract PolicyJobContext from contextData
	policyContext, ok := contextData.(*PolicyJobContext)
	if !ok {
		return fmt.Errorf("invalid policy context data for job %s", jobID)
	}

	// Check if this is notification-based completion with file data
	if notificationData, exists := jobInfo.ContextData["notification_data"]; exists {
		return processPolicyResultsFromNotification(jobID, policyContext, notificationData, statusResp)
	}

	// Legacy VeloArtifact polling flow
	return processPolicyResultsFromVeloArtifact(jobID, policyContext, statusResp)
}

// processPolicyResultsFromNotification handles policy processing when triggered by external notification
func processPolicyResultsFromNotification(jobID string, policyContext *PolicyJobContext, notificationData interface{}, statusResp *StatusResponse) error {
	logger.Infof("Processing policy results from notification for job %s", jobID)

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

	logger.Infof("Processing notification-based policy results: job_id=%s, file=%s, md5=%s", jobID, fileName, md5Hash)

	// Construct local file path using md5Hash from notification
	localFilePath := fmt.Sprintf("%s/%s/%s", config.Cfg.NotificationFileDir, jobID, md5Hash)
	logger.Infof("Processing policy results from notification file: %s", localFilePath)

	// Read and parse the already downloaded file
	resultsData, err := parseResultsFile(localFilePath)
	if err != nil {
		return fmt.Errorf("failed to parse notification results file for job %s: %v", jobID, err)
	}

	logger.Infof("Successfully parsed %d query results from notification file for job %s", len(resultsData), jobID)

	// Process results and create policies atomically
	insertCount, err := createPoliciesFromResults(jobID, policyContext, resultsData)
	if err != nil {
		return err
	}

	logger.Infof("Policy notification handler executed successfully for job %s - created %d policies", jobID, insertCount)
	return nil
}

// processPolicyResultsFromVeloArtifact handles policy processing via traditional VeloArtifact polling
func processPolicyResultsFromVeloArtifact(jobID string, policyContext *PolicyJobContext, statusResp *StatusResponse) error {
	logger.Infof("Processing policy results from VeloArtifact polling for job %s", jobID)

	// Get endpoint information
	ep, err := getEndpointForJob(jobID, policyContext.EndpointID)
	if err != nil {
		return err
	}

	// Retrieve and download results file via VeloArtifact
	resultsData, err := retrieveJobResults(jobID, ep)
	if err != nil {
		return err
	}

	logger.Infof("Successfully retrieved %d query results via VeloArtifact for job %s", len(resultsData), jobID)

	// Process results and create policies atomically
	insertCount, err := createPoliciesFromResults(jobID, policyContext, resultsData)
	if err != nil {
		return err
	}

	logger.Infof("Policy VeloArtifact handler executed successfully for job %s - created %d policies", jobID, insertCount)
	return nil
}

// QueryResult represents a single query execution result
type QueryResult struct {
	QueryKey    string          `json:"query_key"`
	Query       string          `json:"query"`
	Status      string          `json:"status"`
	Result      [][]interface{} `json:"result"`
	ExecuteTime string          `json:"execute_time"`
	DurationMs  int             `json:"duration_ms"`
}

// parseResultsFile reads and parses the downloaded JSON results file
func parseResultsFile(filePath string) ([]QueryResult, error) {
	logger.Debugf("Reading results file: %s", filePath)

	// Read file content
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read results file %s: %v", filePath, err)
	}

	// Parse JSON content as array of QueryResult
	var resultsData []QueryResult
	if err := json.Unmarshal(fileData, &resultsData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON from file %s: %v", filePath, err)
	}

	logger.Debugf("Successfully parsed results file with %d query results", len(resultsData))
	return resultsData, nil
}

// getEndpointForJob retrieves endpoint information for a specific job
func getEndpointForJob(jobID string, endpointID uint) (*models.Endpoint, error) {
	endpointRepo := repository.NewEndpointRepository()
	ep, err := endpointRepo.GetByID(nil, endpointID)
	if err != nil {
		logger.Errorf("Failed to get endpoint for job %s: %v", jobID, err)
		return nil, fmt.Errorf("failed to get endpoint: %v", err)
	}
	return ep, nil
}

// retrieveJobResults gets results from VeloArtifact and downloads the results file
func retrieveJobResults(jobID string, ep *models.Endpoint) ([]QueryResult, error) {
	// Get results from agent using getresults command
	resultsOutput, err := agent.ExecuteAgentAPISimpleCommand(ep.ClientID, ep.OsType, "getresults", jobID, "", true)
	if err != nil {
		logger.Errorf("Failed to get results for job %s: %v", jobID, err)
		return nil, fmt.Errorf("failed to get results: %v", err)
	}

	// Parse getresults response
	var getResultsResp GetResultsResponse
	if err := json.Unmarshal([]byte(resultsOutput), &getResultsResp); err != nil {
		logger.Errorf("Failed to parse getresults response for job %s: %v", jobID, err)
		return nil, fmt.Errorf("failed to parse getresults response: %v", err)
	}

	if !getResultsResp.Success {
		logger.Errorf("GetResults failed for job %s: %s", jobID, getResultsResp.Message)
		return nil, fmt.Errorf("getresults failed: %s", getResultsResp.Message)
	}

	logger.Infof("Policy job results exported: job_id=%s, file=%s, total_queries=%d, completed=%d, failed=%d",
		jobID, getResultsResp.FilePath, getResultsResp.TotalQueries, getResultsResp.Completed, getResultsResp.Failed)

	// Download the results file from agent
	downloadResp, err := agent.DownloadFileAgentAPI(ep.ClientID, getResultsResp.FilePath, ep.OsType)
	if err != nil {
		logger.Errorf("Failed to download results file for job %s: %v", jobID, err)
		return nil, fmt.Errorf("failed to download results file: %v", err)
	}

	// Use the local path from download response
	localFilePath := downloadResp.LocalPath
	logger.Infof("Policy results file downloaded: path=%s, size=%d bytes, md5=%s", localFilePath, downloadResp.Size, downloadResp.Md5)

	// Read and parse the downloaded JSON results file
	resultsData, err := parseResultsFile(localFilePath)
	if err != nil {
		logger.Errorf("Failed to parse results file for job %s: %v", jobID, err)
		return nil, fmt.Errorf("failed to parse results file: %v", err)
	}

	return resultsData, nil
}

// createPoliciesFromResults processes query results and creates policies atomically
func createPoliciesFromResults(jobID string, policyContext *PolicyJobContext, resultsData []QueryResult) (int, error) {
	// Create new transaction for atomic policy creation
	baseRepo := repository.NewBaseRepository()
	tx := baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	// Create policy sync logger for audit trail
	logFilePath := fmt.Sprintf("%s/policy_sync_%s.log", config.Cfg.VeloResultsDir, jobID)
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		logger.Warnf("Failed to create policy sync log file: %v", err)
	}
	var policyAuditLogger *log.Logger
	if logFile != nil {
		defer logFile.Close()
		policyAuditLogger = log.New(logFile, "", log.LstdFlags)
		policyAuditLogger.Printf("Starting policy synchronization for job %s with %d results", jobID, len(resultsData))
	}

	policyLogger := utils.GetPolicyLogger()
	if policyLogger == nil {
		logger.Warnf("Cannot init exception log for job %s", jobID)
	}

	logger.Infof("Processing policy results: job_id=%s, query_results=%d, dbmgt_id=%d", jobID, len(resultsData), policyContext.DBMgtID)

	// Track allowed actors/policies to skip specific object queries
	allowedActorPolicies := make(map[string]bool)
	var insertCount int

	// Get CntMgt ID from DBMgt for database record creation
	dbmgtRepo := repository.NewDBMgtRepository()
	dbmgt, err := dbmgtRepo.GetByID(nil, policyContext.DBMgtID)
	if err != nil {
		logger.Errorf("Failed to get dbmgt for job %s: %v", jobID, err)
		return 0, fmt.Errorf("failed to get dbmgt: %v", err)
	}

	// First pass: Process wildcard objects (Object:-1) to identify allowed policies
	for _, result := range resultsData {
		if result.Status != "success" {
			// logger.Debugf("Skipping failed query result: %s", result.QueryKey)
			continue
		}

		// Parse query_key: Actor:X - Object:Y - PolicyDf:Z[n]
		actorID, objectID, policyDfID, err := parseQueryKey(result.QueryKey)
		if err != nil {
			logger.Errorf("Failed to parse query key %s: %v", result.QueryKey, err)
			continue
		}

		// Only process wildcard objects in first pass
		if objectID != -1 {
			continue
		}

		// Get policy input from context using unique key
		policyData, exists := getPolicyDataFromContext(result.QueryKey, policyContext.SqlFinalMap)
		if !exists {
			logger.Warnf("Policy data not found for query key: %s", result.QueryKey)
			continue
		}

		// Process query result and determine policy action
		output := processQueryResult(result.Result)
		policyDefault := policyData.policydf
		resAllow := policyDefault.SqlGetAllow
		resDeny := policyDefault.SqlGetDeny

		// Enhanced logging for debugging empty results
		logger.Debugf("Policy evaluation for %s: query_result=%v, output='%s', resAllow='%s', resDeny='%s'",
			result.QueryKey, result.Result, output, resAllow, resDeny)

		if len(result.Result) == 0 {
			logger.Debugf("Empty result for %s - output will be 'NULL'", result.QueryKey)
		}

		if isPolicyAllowed(output, resAllow, resDeny, policyLogger, result) {
			// Check for existing policy to prevent duplicates
			var existingPolicy models.DBPolicy
			err := tx.Where("dbmgt_id = ? AND actor_id = ? AND object_id = ? AND dbpolicydefault_id = ?",
				policyContext.DBMgtID, policyData.actorId, policyData.objectId, policyDefault.ID).First(&existingPolicy).Error

			if err == nil {
				// Policy already exists, skip creation
				logger.Debugf("Policy already exists for %s, skipping duplicate", result.QueryKey)
				continue
			} else if err != gorm.ErrRecordNotFound {
				// Database error
				logger.Errorf("Failed to check existing policy for %s: %v", result.QueryKey, err)
				return 0, fmt.Errorf("failed to check existing policy: %v", err)
			}

			// Create new policy (no duplicate found)
			dbpolicy := models.DBPolicy{
				CntMgt:          dbmgt.CntID,
				DBPolicyDefault: policyDefault.ID,
				DBMgt:           utils.MustUintToInt(policyContext.DBMgtID),
				DBActorMgt:      policyData.actorId,
				DBObjectMgt:     policyData.objectId,
				Status:          "enabled",
				Description:     "Auto-collected from V2-DBF Agent",
			}

			if err := tx.Create(&dbpolicy).Error; err != nil {
				logger.Errorf("Failed to create policy for %s: %v", result.QueryKey, err)
				return 0, fmt.Errorf("failed to create policy: %v", err)
			}

			insertCount++

			// Mark this actor+policy as allowed to skip specific objects
			actorPolicyKey := fmt.Sprintf("Actor:%d-PolicyDf:%d", actorID, policyDfID)
			allowedActorPolicies[actorPolicyKey] = true

			logger.Debugf("Created wildcard policy: query_key=%s, policy_id=%d, actor_id=%d", result.QueryKey, dbpolicy.ID, actorID)

			if policyAuditLogger != nil {
				policyAuditLogger.Printf("INSERT WILDCARD: policy_id=%d, actor_id=%d, policy_df_id=%d, dbmgt_id=%d",
					dbpolicy.ID, actorID, policyDfID, policyContext.DBMgtID)
			}
		}
	}

	// Second pass: Process specific objects (Object:>0) if not already allowed
	for _, result := range resultsData {
		if result.Status != "success" {
			continue
		}

		actorID, objectID, policyDfID, err := parseQueryKey(result.QueryKey)
		if err != nil {
			continue
		}

		// Only process specific objects in second pass
		if objectID == -1 {
			continue
		}

		// Check if this actor+policy already has wildcard permission
		actorPolicyKey := fmt.Sprintf("Actor:%d-PolicyDf:%d", actorID, policyDfID)
		if allowedActorPolicies[actorPolicyKey] {
			// logger.Debugf("Skipping specific object for %s - wildcard already allowed", result.QueryKey)
			continue
		}

		// Get policy input from context using unique key
		policyData, exists := getPolicyDataFromContext(result.QueryKey, policyContext.SqlFinalMap)
		if !exists {
			// logger.Warnf("Policy data not found for query key: %s", result.QueryKey)
			continue
		}

		// Process query result
		output := processQueryResult(result.Result)
		policyDefault := policyData.policydf
		resAllow := policyDefault.SqlGetAllow
		resDeny := policyDefault.SqlGetDeny

		if isPolicyAllowed(output, resAllow, resDeny, policyLogger, result) {
			// Check for existing policy to prevent duplicates
			var existingPolicy models.DBPolicy
			err := tx.Where("dbmgt_id = ? AND actor_id = ? AND object_id = ? AND dbpolicydefault_id = ?",
				policyContext.DBMgtID, policyData.actorId, policyData.objectId, policyDefault.ID).First(&existingPolicy).Error

			if err == nil {
				// Policy already exists, skip creation
				logger.Debugf("Policy already exists for %s, skipping duplicate", result.QueryKey)
				continue
			} else if err != gorm.ErrRecordNotFound {
				// Database error
				logger.Errorf("Failed to check existing policy for %s: %v", result.QueryKey, err)
				return 0, fmt.Errorf("failed to check existing policy: %v", err)
			}

			// Create new policy (no duplicate found)
			dbpolicy := models.DBPolicy{
				CntMgt:          dbmgt.CntID,
				DBPolicyDefault: policyDefault.ID,
				DBMgt:           utils.MustUintToInt(policyContext.DBMgtID),
				DBActorMgt:      policyData.actorId,
				DBObjectMgt:     policyData.objectId,
				Status:          "enabled",
				Description:     "Auto-collected from V2-DBF Agent",
			}

			if err := tx.Create(&dbpolicy).Error; err != nil {
				logger.Errorf("Failed to create policy for %s: %v", result.QueryKey, err)
				return 0, fmt.Errorf("failed to create policy: %v", err)
			}

			insertCount++
			logger.Debugf("Created specific policy: query_key=%s, policy_id=%d, actor_id=%d, object_id=%d", result.QueryKey, dbpolicy.ID, actorID, objectID)

			if policyAuditLogger != nil {
				policyAuditLogger.Printf("INSERT SPECIFIC: policy_id=%d, actor_id=%d, policy_df_id=%d, object_id=%d, dbmgt_id=%d",
					dbpolicy.ID, actorID, policyDfID, objectID, policyContext.DBMgtID)
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		logger.Errorf("Failed to commit policies for job %s: %v", jobID, err)
		return 0, fmt.Errorf("failed to commit policies: %v", err)
	}
	txCommitted = true

	logger.Infof("Policy creation completed: job_id=%s, policies_created=%d, dbmgt_id=%d", jobID, insertCount, policyContext.DBMgtID)

	if policyAuditLogger != nil {
		policyAuditLogger.Printf("Policy synchronization completed: %d policies created for dbmgt_id=%d", insertCount, policyContext.DBMgtID)
	}

	return insertCount, nil
}

// parseQueryKey parses enhanced query_key formats:
// - New format: "Actor:X_PolicyDf:Y_General[n]" or "Actor:X_PolicyDf:Y_Object:Z[n]"
// - Legacy format: "Actor:X - Object:Y - PolicyDf:Z[n]" (backward compatibility)
func parseQueryKey(queryKey string) (actorID uint, objectID int, policyDfID uint, err error) {
	// Check for new unique key format first
	if strings.Contains(queryKey, "_PolicyDf:") {
		return parseUniqueQueryKey(queryKey)
	}

	// Legacy format: "Actor:X - Object:Y - PolicyDf:Z[n]"
	parts := strings.Split(queryKey, " - ")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("invalid legacy query key format: %s", queryKey)
	}

	// Parse Actor:X
	actorPart := strings.TrimPrefix(parts[0], "Actor:")
	actorIDInt, err := strconv.Atoi(actorPart)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid actor ID in legacy query key: %s", queryKey)
	}

	// Parse Object:Y
	objectPart := strings.TrimPrefix(parts[1], "Object:")
	objectIDInt, err := strconv.Atoi(objectPart)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid object ID in legacy query key: %s", queryKey)
	}

	// Parse PolicyDf:Z[n] - remove [n] suffix
	policyPart := strings.TrimPrefix(parts[2], "PolicyDf:")
	if bracketIndex := strings.Index(policyPart, "["); bracketIndex != -1 {
		policyPart = policyPart[:bracketIndex]
	}
	policyDfIDInt, err := strconv.Atoi(policyPart)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid policy default ID in legacy query key: %s", queryKey)
	}

	return utils.MustIntToUint(actorIDInt), objectIDInt, utils.MustIntToUint(policyDfIDInt), nil
}

// parseUniqueQueryKey parses new unique key formats:
// - "Actor:X_PolicyDf:Y_General[n]" → actorID=X, objectID=-1, policyDfID=Y
// - "Actor:X_PolicyDf:Y_Object:Z[n]" → actorID=X, objectID=Z, policyDfID=Y
// - "Actor:X_PolicyDf:Y_Object:Z_General[n]" → actorID=X, objectID=Z, policyDfID=Y
// - "Actor:X_PolicyDf:Y_Object:Z_Specific[n]" → actorID=X, objectID=Z, policyDfID=Y
// - "Actor:X_PolicyDf:Y_NoObject[n]" → actorID=X, objectID=0, policyDfID=Y
func parseUniqueQueryKey(queryKey string) (actorID uint, objectID int, policyDfID uint, err error) {
	// Remove [n] suffix if present
	cleanKey := queryKey
	if bracketIndex := strings.Index(cleanKey, "["); bracketIndex != -1 {
		cleanKey = cleanKey[:bracketIndex]
	}

	// Split by underscore
	parts := strings.Split(cleanKey, "_")
	if len(parts) < 2 {
		return 0, 0, 0, fmt.Errorf("invalid unique query key format: %s", queryKey)
	}

	// Parse Actor:X
	if !strings.HasPrefix(parts[0], "Actor:") {
		return 0, 0, 0, fmt.Errorf("missing Actor prefix in unique query key: %s", queryKey)
	}
	actorPart := strings.TrimPrefix(parts[0], "Actor:")
	actorIDInt, err := strconv.Atoi(actorPart)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid actor ID in unique query key: %s", queryKey)
	}

	// Parse PolicyDf:Y
	if !strings.HasPrefix(parts[1], "PolicyDf:") {
		return 0, 0, 0, fmt.Errorf("missing PolicyDf prefix in unique query key: %s", queryKey)
	}
	policyPart := strings.TrimPrefix(parts[1], "PolicyDf:")
	policyDfIDInt, err := strconv.Atoi(policyPart)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid policy default ID in unique query key: %s", queryKey)
	}

	// Handle different formats based on number of parts
	switch len(parts) {
	case 2:
		// "Actor:X_PolicyDf:Y" → no object context
		return utils.MustIntToUint(actorIDInt), 0, utils.MustIntToUint(policyDfIDInt), nil
	case 3:
		thirdPart := parts[2]
		switch {
		case strings.HasPrefix(thirdPart, "Object:"):
			// "Actor:X_PolicyDf:Y_Object:Z" → specific object
			objectPart := strings.TrimPrefix(thirdPart, "Object:")
			objectIDInt, err := strconv.Atoi(objectPart)
			if err != nil {
				return 0, 0, 0, fmt.Errorf("invalid object ID in unique query key: %s", queryKey)
			}
			return utils.MustIntToUint(actorIDInt), objectIDInt, utils.MustIntToUint(policyDfIDInt), nil
		case thirdPart == "General":
			// "Actor:X_PolicyDf:Y_General" → wildcard object
			return utils.MustIntToUint(actorIDInt), -1, utils.MustIntToUint(policyDfIDInt), nil
		case thirdPart == "NoObject":
			// "Actor:X_PolicyDf:Y_NoObject" → no object context
			return utils.MustIntToUint(actorIDInt), 0, utils.MustIntToUint(policyDfIDInt), nil
		default:
			return 0, 0, 0, fmt.Errorf("invalid third part in unique query key: %s", queryKey)
		}
	case 4:
		// "Actor:X_PolicyDf:Y_Object:Z_General" or "Actor:X_PolicyDf:Y_Object:Z_Specific"
		thirdPart := parts[2]
		fourthPart := parts[3]

		if !strings.HasPrefix(thirdPart, "Object:") {
			return 0, 0, 0, fmt.Errorf("invalid third part format in 4-part unique query key: %s", queryKey)
		}

		objectPart := strings.TrimPrefix(thirdPart, "Object:")
		objectIDInt, err := strconv.Atoi(objectPart)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid object ID in 4-part unique query key: %s", queryKey)
		}

		// Fourth part should be "General" or "Specific" - both are treated the same for parsing
		if fourthPart != "General" && fourthPart != "Specific" {
			return 0, 0, 0, fmt.Errorf("invalid fourth part in unique query key: %s", queryKey)
		}

		return utils.MustIntToUint(actorIDInt), objectIDInt, utils.MustIntToUint(policyDfIDInt), nil
	default:
		return 0, 0, 0, fmt.Errorf("unsupported unique query key format with %d parts: %s", len(parts), queryKey)
	}
}

// getPolicyDataFromContext finds policy data using unique key lookup instead of SQL matching
// This prevents issues with SQL collision and improves lookup performance
func getPolicyDataFromContext(queryKey string, sqlFinalMap map[string]policyInput) (policyInput, bool) {
	// Remove [n] suffix from query key if present
	cleanKey := queryKey
	if bracketIndex := strings.Index(cleanKey, "["); bracketIndex != -1 {
		cleanKey = cleanKey[:bracketIndex]
	}

	// Direct lookup by unique key (much faster than SQL string comparison)
	if policyData, exists := sqlFinalMap[cleanKey]; exists {
		return policyData, true
	}

	// Fallback to SQL matching for backward compatibility (legacy format)
	for _, policyData := range sqlFinalMap {
		if strings.TrimSpace(policyData.finalSQL) == strings.TrimSpace(queryKey) {
			logger.Debugf("Found policy data using SQL fallback for key: %s", queryKey)
			return policyData, true
		}
	}

	return policyInput{}, false
}

func isPolicyAllowed(output, resAllow, resDeny string, policyLogger *log.Logger, result QueryResult) bool {
	// Check explicit deny first (higher priority)
	if output == resDeny {
		return false
	}

	// Exact match with allowed result
	if output == resAllow {
		return true
	}

	// Special case: NOT NULL means any non-null result is allowed (but still check deny first)
	if resAllow == "NOT NULL" && output != "NULL" {
		return true
	}

	// If no match, log exception and deny
	if policyLogger != nil {
		policyLogger.Printf("Policy exception output for %s: %s, result: %s",
			result.QueryKey, result.Query, output)
	}

	return false
}

// processQueryResult converts query result to output string for policy comparison
func processQueryResult(result [][]interface{}) string {
	// Handle empty result or empty first row
	if len(result) == 0 {
		return "NULL"
	}
	if len(result[0]) == 0 {
		return "NULL"
	}

	// Get first row, first column
	firstValue := result[0][0]
	if firstValue == nil {
		return "NULL"
	}

	// Convert to string
	output := fmt.Sprintf("%v", firstValue)

	// Simplified logic: Only handle NULL cases, return original output otherwise
	if output == "" || output == "NULL" {
		return "NULL"
	}

	// Return original output for all other cases (Y, N, etc.)
	return output
}

// CreateCombinedPolicyCompletionHandler creates a callback function for combined policy job completion
func CreateCombinedPolicyCompletionHandler() JobCompletionCallback {
	return func(jobID string, jobInfo *JobInfo, statusResp *StatusResponse) error {
		logger.Infof("Processing combined policy completion for job %s, status: %s", jobID, statusResp.Status)

		// Extract context data from job
		contextData, ok := jobInfo.ContextData["combined_policy_context"]
		if !ok {
			return fmt.Errorf("missing combined policy context data for job %s", jobID)
		}

		if statusResp.Status == "completed" {
			return processCombinedPolicyResults(jobID, contextData, statusResp, jobInfo)
		} else {
			logger.Errorf("Combined policy job %s failed, no policies will be created", jobID)
			return fmt.Errorf("combined policy job failed: %s", statusResp.Message)
		}
	}
}

// processCombinedPolicyResults processes the results of a completed combined policy job
func processCombinedPolicyResults(jobID string, contextData interface{}, statusResp *StatusResponse, jobInfo *JobInfo) error {
	logger.Infof("Processing combined policy results for job %s - completed: %d, failed: %d",
		jobID, statusResp.Completed, statusResp.Failed)

	// Extract CombinedPolicyJobContext from contextData
	combinedContext, ok := contextData.(*CombinedPolicyJobContext)
	if !ok {
		return fmt.Errorf("invalid combined policy context data for job %s", jobID)
	}

	// Check if this is notification-based completion with file data
	if notificationData, exists := jobInfo.ContextData["notification_data"]; exists {
		return processCombinedPolicyResultsFromNotification(jobID, combinedContext, notificationData, statusResp)
	}

	// Legacy VeloArtifact polling flow
	return processCombinedPolicyResultsFromVeloArtifact(jobID, combinedContext, statusResp)
}

// processCombinedPolicyResultsFromNotification handles combined policy processing when triggered by external notification
func processCombinedPolicyResultsFromNotification(jobID string, combinedContext *CombinedPolicyJobContext, notificationData interface{}, statusResp *StatusResponse) error {
	logger.Infof("Processing combined policy results from notification for job %s", jobID)

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

	logger.Infof("Processing notification-based combined policy results: job_id=%s, file=%s, md5=%s", jobID, fileName, md5Hash)

	// Construct local file path using md5Hash from notification
	localFilePath := fmt.Sprintf("%s/%s/%s", config.Cfg.NotificationFileDir, jobID, md5Hash)
	logger.Infof("Processing combined policy results from notification file: %s", localFilePath)

	// Read and parse the already downloaded file
	resultsData, err := parseResultsFile(localFilePath)
	if err != nil {
		return fmt.Errorf("failed to parse notification results file for job %s: %v", jobID, err)
	}

	logger.Infof("Successfully parsed %d combined query results from notification file for job %s", len(resultsData), jobID)

	// Process results and create policies atomically for all databases
	totalInserts, err := createCombinedPoliciesFromResults(jobID, combinedContext, resultsData)
	if err != nil {
		return err
	}

	logger.Infof("Combined policy notification handler executed successfully for job %s - created %d policies across %d databases",
		jobID, totalInserts, len(combinedContext.DbMgts))
	return nil
}

// processCombinedPolicyResultsFromVeloArtifact handles combined policy processing via traditional VeloArtifact polling
func processCombinedPolicyResultsFromVeloArtifact(jobID string, combinedContext *CombinedPolicyJobContext, statusResp *StatusResponse) error {
	logger.Infof("Processing combined policy results from VeloArtifact polling for job %s", jobID)

	// Get endpoint information
	ep, err := getEndpointForJob(jobID, combinedContext.EndpointID)
	if err != nil {
		return err
	}

	// Retrieve and download results file via VeloArtifact
	resultsData, err := retrieveJobResults(jobID, ep)
	if err != nil {
		return err
	}

	logger.Infof("Successfully retrieved %d combined query results via VeloArtifact for job %s", len(resultsData), jobID)

	// Process results and create policies atomically for all databases
	totalInserts, err := createCombinedPoliciesFromResults(jobID, combinedContext, resultsData)
	if err != nil {
		return err
	}

	logger.Infof("Combined policy VeloArtifact handler executed successfully for job %s - created %d policies across %d databases",
		jobID, totalInserts, len(combinedContext.DbMgts))
	return nil
}

// createCombinedPoliciesFromResults processes combined query results and creates policies atomically across all databases
func createCombinedPoliciesFromResults(jobID string, combinedContext *CombinedPolicyJobContext, resultsData []QueryResult) (int, error) {
	logger.Infof("Processing combined policy results: job_id=%s, query_results=%d, cntmgt_id=%d, databases=%d",
		jobID, len(resultsData), combinedContext.CntMgtID, len(combinedContext.DbMgts))

	// Create new transaction for atomic policy creation across all databases
	baseRepo := repository.NewBaseRepository()
	tx := baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	// Create combined policy sync logger for audit trail
	logFilePath := fmt.Sprintf("%s/combined_policy_sync_%s.log", config.Cfg.VeloResultsDir, jobID)
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		logger.Warnf("Failed to create combined policy sync log file: %v", err)
	}
	var policyAuditLogger *log.Logger
	if logFile != nil {
		defer logFile.Close()
		policyAuditLogger = log.New(logFile, "", log.LstdFlags)
		policyAuditLogger.Printf("Starting combined policy synchronization for job %s with %d results across %d databases",
			jobID, len(resultsData), len(combinedContext.DbMgts))
	}

	policyLogger := utils.GetPolicyLogger()
	if policyLogger == nil {
		logger.Warnf("Cannot init exception log for job %s", jobID)
	}

	// Process results by database
	totalInserts := 0
	processedDBs := make(map[uint]int) // dbmgt_id -> policy count

	for _, result := range resultsData {
		if result.Status != "success" {
			// logger.Debugf("Skipping failed query result: %s", result.QueryKey)
			continue
		}

		// Parse combined query key format: "DbMgt:X_Actor:Y_PolicyDf:Z_..."
		dbMgtID, actorID, objectID, policyDfID, err := parseCombinedPolicyQueryKey(result.QueryKey)
		if err != nil {
			logger.Errorf("Failed to parse combined query key %s: %v", result.QueryKey, err)
			continue
		}

		// Get policy input from context using database-specific lookup
		dbQueries, exists := combinedContext.DbQueries[dbMgtID]
		if !exists {
			logger.Warnf("No queries found for database %d in context", dbMgtID)
			continue
		}

		// Extract original key by removing DbMgt prefix
		originalKey := strings.TrimPrefix(result.QueryKey, fmt.Sprintf("DbMgt:%d_", dbMgtID))
		if bracketIndex := strings.Index(originalKey, "["); bracketIndex != -1 {
			originalKey = originalKey[:bracketIndex]
		}

		policyData, exists := dbQueries[originalKey]
		if !exists {
			logger.Warnf("Policy data not found for original key: %s in database %d", originalKey, dbMgtID)
			continue
		}

		// Process query result and determine policy action
		output := processQueryResult(result.Result)
		policyDefault := policyData.policydf
		resAllow := policyDefault.SqlGetAllow
		resDeny := policyDefault.SqlGetDeny

		// logger.Debugf("Combined policy comparison for %s: output='%s', resAllow='%s', resDeny='%s'", result.QueryKey, output, resAllow, resDeny)

		if isPolicyAllowed(output, resAllow, resDeny, policyLogger, result) {
			// Check for existing policy to prevent duplicates
			var existingPolicy models.DBPolicy
			err := tx.Where("dbmgt_id = ? AND actor_id = ? AND object_id = ? AND dbpolicydefault_id = ?",
				dbMgtID, policyData.actorId, policyData.objectId, policyDefault.ID).First(&existingPolicy).Error

			if err == nil {
				// Policy already exists, skip creation
				// logger.Debugf("Combined policy already exists for %s, skipping duplicate", result.QueryKey)
				continue
			} else if err != gorm.ErrRecordNotFound {
				// Database error
				logger.Errorf("Failed to check existing combined policy for %s: %v", result.QueryKey, err)
				return 0, fmt.Errorf("failed to check existing combined policy: %v", err)
			}

			// Create new policy (no duplicate found)
			dbpolicy := models.DBPolicy{
				CntMgt:          combinedContext.CntMgtID,
				DBPolicyDefault: policyDefault.ID,
				DBMgt:           utils.MustUintToInt(dbMgtID),
				DBActorMgt:      policyData.actorId,
				DBObjectMgt:     policyData.objectId,
				Status:          "enabled",
				Description:     "Auto-collected from V2-DBF Agent",
			}

			if err := tx.Create(&dbpolicy).Error; err != nil {
				logger.Errorf("Failed to create combined policy for %s: %v", result.QueryKey, err)
				return 0, fmt.Errorf("failed to create combined policy: %v", err)
			}

			totalInserts++
			processedDBs[dbMgtID]++

			// logger.Debugf("Created combined policy: query_key=%s, policy_id=%d, dbmgt_id=%d, actor_id=%d", result.QueryKey, dbpolicy.ID, dbMgtID, actorID)

			if policyAuditLogger != nil {
				policyAuditLogger.Printf("INSERT COMBINED: policy_id=%d, dbmgt_id=%d, actor_id=%d, policy_df_id=%d, object_id=%d",
					dbpolicy.ID, dbMgtID, actorID, policyDfID, objectID)
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		logger.Errorf("Failed to commit combined policies for job %s: %v", jobID, err)
		return 0, fmt.Errorf("failed to commit combined policies: %v", err)
	}
	txCommitted = true

	logger.Infof("Combined policy creation completed: job_id=%s, total_policies=%d, databases_processed=%d",
		jobID, totalInserts, len(processedDBs))

	if policyAuditLogger != nil {
		policyAuditLogger.Printf("Combined policy synchronization completed: %d policies created across %d databases",
			totalInserts, len(processedDBs))
		for dbMgtID, count := range processedDBs {
			policyAuditLogger.Printf("Database %d: %d policies created", dbMgtID, count)
		}
	}

	return totalInserts, nil
}

// parseCombinedPolicyQueryKey extracts database ID, actor ID, object ID, and policy default ID from combined query key
// Format: "DbMgt:X_Actor:Y_PolicyDf:Z_General[n]" or "DbMgt:X_Actor:Y_PolicyDf:Z_Object:W[n]"
func parseCombinedPolicyQueryKey(queryKey string) (dbMgtID uint, actorID uint, objectID int, policyDfID uint, err error) {
	// Remove [n] suffix if present
	cleanKey := queryKey
	if bracketIndex := strings.Index(cleanKey, "["); bracketIndex != -1 {
		cleanKey = cleanKey[:bracketIndex]
	}

	// Split by underscore to separate parts
	parts := strings.Split(cleanKey, "_")
	if len(parts) < 3 {
		return 0, 0, 0, 0, fmt.Errorf("invalid combined query key format: %s", queryKey)
	}

	// Extract dbmgt_id from "DbMgt:X" part
	dbPart := parts[0]
	if !strings.HasPrefix(dbPart, "DbMgt:") {
		return 0, 0, 0, 0, fmt.Errorf("invalid DbMgt prefix in query key: %s", queryKey)
	}

	dbMgtIDStr := strings.TrimPrefix(dbPart, "DbMgt:")
	dbMgtIDInt, err := strconv.ParseUint(dbMgtIDStr, 10, 32)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to parse dbmgt_id from key %s: %w", queryKey, err)
	}

	// Parse the rest using existing unique key parser (skip DbMgt part)
	remainingKey := strings.Join(parts[1:], "_")
	actorIDParsed, objectIDParsed, policyDfIDParsed, err := parseUniqueQueryKey(remainingKey)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to parse remaining key parts from %s: %w", queryKey, err)
	}

	return uint(dbMgtIDInt), actorIDParsed, objectIDParsed, policyDfIDParsed, nil
}
