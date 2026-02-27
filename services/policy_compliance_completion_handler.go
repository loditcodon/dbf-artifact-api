package services

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"dbfartifactapi/config"
	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services/agent"
	"dbfartifactapi/services/job"
)

// PolicyComplianceJobContext contains context data for policy compliance job completion
type PolicyComplianceJobContext struct {
	CntMgtID   uint           `json:"cntmgt_id"`
	CMT        *models.CntMgt `json:"cmt"`
	EndpointID uint           `json:"endpoint_id"`
}

// PolicyComplianceResult represents a compliance check result with new format
type PolicyComplianceResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Value       string `json:"value"`
	Message     string `json:"message"`
	TimeCheck   string `json:"time_check"`
}

// PolicyComplianceGetResultsResponse represents VeloArtifact getresults response for policy compliance
type PolicyComplianceGetResultsResponse struct {
	Completed    int    `json:"completed"`
	Failed       int    `json:"failed"`
	FilePath     string `json:"file_path"`
	Message      string `json:"message"`
	Success      bool   `json:"success"`
	TotalQueries int    `json:"total_queries"`
}

// CreatePolicyComplianceCompletionHandler creates a callback function for policy compliance job completion
func CreatePolicyComplianceCompletionHandler() job.JobCompletionCallback {
	return func(jobID string, jobInfo *job.JobInfo, statusResp *job.StatusResponse) error {
		logger.Infof("Processing policy compliance completion for job %s, status: %s", jobID, statusResp.Status)

		// Extract context data from job
		contextData, ok := jobInfo.ContextData["policy_compliance_context"]
		if !ok {
			return fmt.Errorf("missing policy compliance context data for job %s", jobID)
		}

		if statusResp.Status == "completed" {
			return processPolicyComplianceResults(jobID, contextData, statusResp, jobInfo)
		} else {
			logger.Errorf("Policy compliance job %s failed, no results will be processed", jobID)
			return fmt.Errorf("policy compliance job failed: %s", statusResp.Message)
		}
	}
}

// processPolicyComplianceResults processes the results of a completed policy compliance job.
// Routes to notification-based or VeloArtifact polling flow based on available data.
func processPolicyComplianceResults(jobID string, contextData interface{}, statusResp *job.StatusResponse, jobInfo *job.JobInfo) error {
	logger.Infof("Processing policy compliance results for job %s - completed: %d, failed: %d",
		jobID, statusResp.Completed, statusResp.Failed)

	// Extract PolicyComplianceJobContext from contextData
	complianceContext, ok := contextData.(*PolicyComplianceJobContext)
	if !ok {
		err := fmt.Errorf("invalid policy compliance context data for job %s", jobID)
		jobMonitor := job.GetJobMonitorService()
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	// Check if this is notification-based completion with file data
	if notificationData, exists := jobInfo.ContextData["notification_data"]; exists {
		return processPolicyComplianceResultsFromNotification(jobID, complianceContext, notificationData, statusResp)
	}

	// Legacy VeloArtifact polling flow
	return processPolicyComplianceResultsFromVeloArtifact(jobID, complianceContext, statusResp)
}

// processPolicyComplianceResultsFromNotification handles policy compliance processing when triggered by external notification.
func processPolicyComplianceResultsFromNotification(jobID string, complianceContext *PolicyComplianceJobContext, notificationData interface{}, statusResp *job.StatusResponse) error {
	logger.Infof("Processing policy compliance results from notification for job %s", jobID)

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

	logger.Infof("Processing notification-based policy compliance results: job_id=%s, file=%s, md5=%s", jobID, fileName, md5Hash)

	// Construct local file path using md5Hash from notification
	localFilePath := fmt.Sprintf("%s/%s/%s", config.Cfg.NotificationFileDir, jobID, md5Hash)
	logger.Infof("Processing policy compliance results from notification file: %s", localFilePath)

	// Read and parse the already downloaded file
	resultsData, err := parsePolicyComplianceResultsFile(localFilePath)
	if err != nil {
		errMsg := fmt.Sprintf("failed to parse notification results file for job %s: %v", jobID, err)
		jobMonitor.FailJobAfterProcessing(jobID, errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	logger.Infof("Successfully parsed %d compliance results from notification file for job %s", len(resultsData), jobID)

	// Process results using external tool
	err = processPolicyComplianceData(jobID, complianceContext, resultsData)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	// Mark job as completed after all server-side processing is done
	successMsg := fmt.Sprintf("Policy compliance check completed successfully - processed %d results", len(resultsData))
	if err := jobMonitor.CompleteJobAfterProcessing(jobID, successMsg); err != nil {
		logger.Errorf("Failed to mark job as completed: %v", err)
	}

	logger.Infof("Policy compliance notification handler executed successfully for job %s - processed %d results", jobID, len(resultsData))
	return nil
}

// processPolicyComplianceResultsFromVeloArtifact handles policy compliance processing via traditional VeloArtifact polling.
func processPolicyComplianceResultsFromVeloArtifact(jobID string, complianceContext *PolicyComplianceJobContext, statusResp *job.StatusResponse) error {
	logger.Infof("Processing policy compliance results from VeloArtifact polling for job %s", jobID)

	jobMonitor := job.GetJobMonitorService()

	// Get endpoint information
	ep, err := getEndpointForComplianceJob(jobID, complianceContext.EndpointID)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	// Retrieve and download results file
	resultsData, err := retrievePolicyComplianceJobResults(jobID, ep)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	logger.Infof("Successfully retrieved %d compliance results via VeloArtifact for job %s", len(resultsData), jobID)

	// Process results using external tool
	err = processPolicyComplianceData(jobID, complianceContext, resultsData)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	// Mark job as completed after all server-side processing is done
	successMsg := fmt.Sprintf("Policy compliance check completed successfully - processed %d results", len(resultsData))
	if err := jobMonitor.CompleteJobAfterProcessing(jobID, successMsg); err != nil {
		logger.Errorf("Failed to mark job as completed: %v", err)
	}

	logger.Infof("Policy compliance VeloArtifact handler executed successfully for job %s - processed %d results", jobID, len(resultsData))
	return nil
}

// getEndpointForComplianceJob retrieves endpoint information for compliance job processing
func getEndpointForComplianceJob(jobID string, endpointID uint) (*models.Endpoint, error) {
	logger.Debugf("Getting endpoint for compliance job %s, endpoint ID: %d", jobID, endpointID)

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

// retrievePolicyComplianceJobResults retrieves and parses compliance job results from VeloArtifact
func retrievePolicyComplianceJobResults(jobID string, endpoint *models.Endpoint) ([]PolicyComplianceResult, error) {
	logger.Debugf("Retrieving policy compliance results for job %s from endpoint %s", jobID, endpoint.ClientID)

	// Get results from agent using getresults command
	resultsOutput, err := agent.ExecuteAgentAPISimpleCommand(endpoint.ClientID, endpoint.OsType, "getresults", jobID, "", true)
	if err != nil {
		return nil, fmt.Errorf("failed to get results for job %s: %w", jobID, err)
	}

	// Parse getresults response
	var getResultsResp PolicyComplianceGetResultsResponse
	if err := json.Unmarshal([]byte(resultsOutput), &getResultsResp); err != nil {
		return nil, fmt.Errorf("failed to parse getresults response for job %s: %w", jobID, err)
	}

	if !getResultsResp.Success {
		return nil, fmt.Errorf("getresults failed for job %s: %s", jobID, getResultsResp.Message)
	}

	logger.Infof("Policy compliance job %s results: completed=%d, failed=%d, file=%s",
		jobID, getResultsResp.Completed, getResultsResp.Failed, getResultsResp.FilePath)

	// Download results file from agent
	downloadInfo, err := agent.DownloadFileAgentAPI(endpoint.ClientID, getResultsResp.FilePath, endpoint.OsType)
	if err != nil {
		return nil, fmt.Errorf("failed to download results file for job %s: %w", jobID, err)
	}

	// Use the local path from download response
	localFilePath := downloadInfo.LocalPath

	// Parse results file
	resultsData, err := parsePolicyComplianceResultsFile(localFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse results file %s for job %s: %w", localFilePath, jobID, err)
	}

	logger.Infof("Successfully parsed %d policy compliance results from file %s", len(resultsData), localFilePath)
	return resultsData, nil
}

// parsePolicyComplianceResultsFile reads and parses the downloaded JSON results file
func parsePolicyComplianceResultsFile(filePath string) ([]PolicyComplianceResult, error) {
	logger.Debugf("Reading policy compliance results file: %s", filePath)

	// Read file content
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read results file %s: %w", filePath, err)
	}

	// Parse JSON array
	var results []PolicyComplianceResult
	if err := json.Unmarshal(fileData, &results); err != nil {
		return nil, fmt.Errorf("failed to unmarshal results from %s: %w", filePath, err)
	}

	logger.Debugf("Successfully parsed %d policy compliance results from file", len(results))
	return results, nil
}

// processPolicyComplianceData processes compliance results by calling external tool
func processPolicyComplianceData(jobID string, complianceContext *PolicyComplianceJobContext, results []PolicyComplianceResult) error {
	logger.Infof("Processing %d policy compliance results for job %s, cntmgt_id=%d",
		len(results), jobID, complianceContext.CntMgtID)

	// Validate JSON format by checking structure
	if len(results) > 0 {
		logger.Infof("Sample compliance result: name=%s, description=%s, value=%s, message=%s, time_check=%s",
			results[0].Name, results[0].Description, results[0].Value, results[0].Message, results[0].TimeCheck)
	}

	// Save results to a temporary file path for external tool
	tempResultPath := fmt.Sprintf("%s/policy_compliance_results_%s.json", config.Cfg.VeloResultsDir, jobID)

	// Write results to JSON file
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal compliance results: %w", err)
	}

	if err := os.WriteFile(tempResultPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write compliance results to file: %w", err)
	}

	logger.Infof("Policy compliance results written to: %s", tempResultPath)

	// Call external dbfcheckpolicycompliance tool
	cmd := exec.Command(config.Cfg.DBFCheckPolicyCompliancePath,
		"--id", fmt.Sprintf("%d", complianceContext.CntMgtID),
		"--path", tempResultPath)

	logger.Infof("Executing: %s --id %d --path %s",
		config.Cfg.DBFCheckPolicyCompliancePath, complianceContext.CntMgtID, tempResultPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Errorf("dbfcheckpolicycompliance execution failed: %v, output: %s", err, string(output))
		return fmt.Errorf("failed to execute dbfcheckpolicycompliance: %w", err)
	}

	logger.Infof("dbfcheckpolicycompliance completed successfully for job %s, output: %s", jobID, string(output))

	// Optionally clean up temporary file
	if err := os.Remove(tempResultPath); err != nil {
		logger.Warnf("Failed to remove temporary results file %s: %v", tempResultPath, err)
	}

	return nil
}
