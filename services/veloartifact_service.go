package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"dbfartifactapi/config"
	"dbfartifactapi/pkg/logger"
)

// VeloResponse: Response structure from VeloArtifact command execution
type VeloResponse map[string]struct {
	Complete   bool   `json:"Complete"`
	ReturnCode int    `json:"ReturnCode"`
	Stdout     string `json:"Stdout"`
	Stderr     string `json:"Stderr"`
}

// VeloDownloadResponse: response for file download operations
type VeloDownloadResponse struct {
	Accessor   string   `json:"Accessor"`
	Error      string   `json:"Error"`
	Md5        string   `json:"Md5"`
	Path       string   `json:"Path"`
	Sha256     string   `json:"Sha256"`
	Size       int64    `json:"Size"`
	StoredSize int64    `json:"StoredSize"`
	Components []string `json:"_Components"`
}

// ConnectionTestParams represents parameters for database connection testing
type ConnectionTestParams struct {
	Action   string `json:"action"`
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// ConnectionTestResult represents the result from v2dbfsqldetector
type ConnectionTestResult struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Response string `json:"response"`
}

func executeSqlVeloArtifact(clientID, artifactName, paramJSON string, requiredStdout, isPolicy bool) (string, error) {
	maxRetries := config.Cfg.VeloMaxRetries
	baseDelay := config.Cfg.VeloRetryBaseDelay

	logger.Debugf("Starting VeloArtifact execution with retry - clientID: %s, osType: %s, maxRetries: %d, baseDelay: %v",
		clientID, artifactName, maxRetries, baseDelay)

	switch strings.ToLower(artifactName) {
	case "linux":
		artifactName = "Linux.V2DBF.Sqlexecute"
	case "windows":
		artifactName = "Windows.V2DBF.Sqlexecute"
	default:
		logger.Errorf("Unknown OS type: %s", artifactName)
		return "", fmt.Errorf("unknown os_type: %s", artifactName)
	}

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			// Exponential backoff: 2s, 4s, 8s, 16s for attempts 2-5
			// Use lookup table to avoid gosec G115 false positive on bit shift
			delays := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second}
			delayIndex := attempt - 2
			if delayIndex >= len(delays) {
				delayIndex = len(delays) - 1
			}
			delay := delays[delayIndex]
			logger.Warnf("VeloArtifact attempt %d failed, retrying in %v...", attempt-1, delay)
			time.Sleep(delay)
		}

		logger.Debugf("VeloArtifact attempt %d/%d - clientID: %s", attempt, maxRetries, clientID)

		result, err := executeSqlVeloArtifactAttempt(clientID, artifactName, paramJSON, requiredStdout, isPolicy)
		if err == nil {
			if attempt > 1 {
				logger.Infof("VeloArtifact succeeded on attempt %d/%d", attempt, maxRetries)
			}
			return result, nil
		}

		lastErr = err

		// Check if this is a retryable error
		if !isRetryableError(err) {
			logger.Errorf("Non-retryable error on attempt %d: %v", attempt, err)
			return "", err
		}

		logger.Warnf("Retryable error on attempt %d/%d: %v", attempt, maxRetries, err)
	}

	logger.Errorf("VeloArtifact failed after %d attempts, last error: %v", maxRetries, lastErr)
	return "", fmt.Errorf("VeloArtifact failed after %d attempts: %w", maxRetries, lastErr)
}

// executeSqlVeloArtifactAttempt performs a single attempt of VeloArtifact execution with timeout
func executeSqlVeloArtifactAttempt(clientID, artifactName, paramJSON string, requiredStdout, isPolicy bool) (string, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), config.Cfg.VeloExecutionTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		"sudo",
		config.Cfg.VeloClientPath, // path from config
		"-c", clientID,            // client (endpoint) ID
		"-a", artifactName, // artifact name
		"-p", paramJSON, // JSON param
	)
	logger.Debugf("Executing command with timeout %v: sudo %s -c %s -a %s -p '%s'",
		config.Cfg.VeloExecutionTimeout, config.Cfg.VeloClientPath, clientID, artifactName, paramJSON)

	outputBytes, err := cmd.Output()
	if err != nil {
		// Check if error is due to timeout
		if ctx.Err() == context.DeadlineExceeded {
			logger.Debugf("Command execution timed out after %v", config.Cfg.VeloExecutionTimeout)
			return "", fmt.Errorf("veloapiclient execution timed out after %v", config.Cfg.VeloExecutionTimeout)
		}
		logger.Debugf("Command execution failed: %v", err)
		return "", fmt.Errorf("failed to run veloapiclient: %v", err)
	}

	logger.Debugf("executeSqlVeloArtifact output: %s", string(outputBytes))

	var resp VeloResponse
	if err := json.Unmarshal(outputBytes, &resp); err != nil {
		logger.Debugf("JSON parsing failed: %v, Output: %s", err, string(outputBytes))
		return "", fmt.Errorf("failed to parse JSON: %v\nOutput was: %s", err, string(outputBytes))
	}

	val, ok := resp["0"]
	logger.Debugf("VeloResponse: %+v", val)
	if !ok {
		logger.Debugf("Missing '0' key in VeloResponse")
		return "", errors.New(`no "0" key in response`)
	}

	// Validate VeloArtifact execution completed successfully
	if !val.Complete {
		logger.Debugf("VeloArtifact execution not completed")
		return val.Stdout, fmt.Errorf("VeloArtifact execution not completed")
	}

	if val.ReturnCode != 0 {
		logger.Debugf("VeloArtifact command failed with code=%d, stderr=%s", val.ReturnCode, val.Stderr)
		return val.Stdout, fmt.Errorf("command failed with code=%d, stderr=%s", val.ReturnCode, val.Stderr)
	}

	// For non-policy operations requiring stdout, validate response content
	if !isPolicy && requiredStdout && val.Stdout == "" {
		logger.Debugf("VeloArtifact command returned empty stdout")
		return "", fmt.Errorf("command returned empty stdout")
	}

	logger.Debugf("VeloArtifact execution completed successfully")
	return val.Stdout, nil
}

// isRetryableError determines if an error is worth retrying
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Common retryable error patterns
	retryablePatterns := []string{
		"no \"0\" key in response",      // JSON parsing issue
		"failed to parse json",          // JSON parsing issue
		"command returned empty stdout", // Empty response
		"connection refused",            // Network issue
		"timeout",                       // Timeout issue
		"timed out after",               // Context timeout issue
		"temporary failure",             // Temporary failure
		"service unavailable",           // Service issue
		"context deadline exceeded",     // Context timeout
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	// Don't retry authentication or permission errors
	nonRetryablePatterns := []string{
		"permission denied",
		"unauthorized",
		"access denied",
		"unknown os_type",
		"invalid artifact",
	}

	for _, pattern := range nonRetryablePatterns {
		if strings.Contains(errStr, pattern) {
			return false
		}
	}

	// By default, treat command execution failures as retryable
	// This covers cases like "failed to run veloapiclient"
	return true
}

// downloadFileVeloArtifact: download file from remote endpoint using VeloArtifact with retry logic
func downloadFileVeloArtifact(clientID, filePath, osType string) (*VeloDownloadResponse, error) {
	maxRetries := config.Cfg.VeloDownloadRetries
	baseDelay := config.Cfg.VeloRetryBaseDelay

	// Convert Windows path backslashes to forward slashes
	if strings.ToLower(osType) == "windows" {
		filePath = strings.ReplaceAll(filePath, "\\", "/")
	}

	logger.Debugf("Starting file download with retry - clientID: %s, filePath: %s, osType: %s", clientID, filePath, osType)

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			delay := time.Duration(attempt-1) * baseDelay
			logger.Warnf("File download attempt %d failed, retrying in %v...", attempt-1, delay)
			time.Sleep(delay)
		}

		logger.Debugf("File download attempt %d/%d - clientID: %s, filePath: %s", attempt, maxRetries, clientID, filePath)

		result, err := downloadFileVeloArtifactAttempt(clientID, filePath)
		if err == nil {
			if attempt > 1 {
				logger.Infof("File download succeeded on attempt %d/%d", attempt, maxRetries)
			}
			return result, nil
		}

		lastErr = err

		// Check if this is a retryable error
		if !isRetryableError(err) {
			logger.Errorf("Non-retryable file download error on attempt %d: %v", attempt, err)
			return nil, err
		}

		logger.Warnf("Retryable file download error on attempt %d/%d: %v", attempt, maxRetries, err)
	}

	logger.Errorf("File download failed after %d attempts, last error: %v", maxRetries, lastErr)
	return nil, fmt.Errorf("file download failed after %d attempts: %w", maxRetries, lastErr)
}

// downloadFileVeloArtifactAttempt performs a single attempt of file download with timeout
func downloadFileVeloArtifactAttempt(clientID, filePath string) (*VeloDownloadResponse, error) {
	// Create parameters for System.VFS.DownloadFile artifact
	paramJSON := fmt.Sprintf(`{"Path": "%s", "Accessor": "file"}`, filePath)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), config.Cfg.VeloDownloadTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		"sudo",
		config.Cfg.VeloClientPath, // path from config
		"-c", clientID,            // client (endpoint) ID
		"-a", "System.VFS.DownloadFile", // artifact name for file download
		"-p", paramJSON, // JSON param
	)

	logger.Debugf("Executing command with timeout %v: sudo %s -c %s -a System.VFS.DownloadFile -p '%s'",
		config.Cfg.VeloDownloadTimeout, config.Cfg.VeloClientPath, clientID, paramJSON)

	outputBytes, err := cmd.Output()
	if err != nil {
		// Check if error is due to timeout
		if ctx.Err() == context.DeadlineExceeded {
			logger.Debugf("File download timed out after %v", config.Cfg.VeloDownloadTimeout)
			return nil, fmt.Errorf("file download timed out after %v", config.Cfg.VeloDownloadTimeout)
		}
		logger.Debugf("Failed to run veloapiclient for file download: %v", err)
		return nil, fmt.Errorf("failed to run veloapiclient for file download: %w", err)
	}

	logger.Debugf("downloadFileVeloArtifact output: %s", string(outputBytes))

	// Try to parse as raw JSON first to check structure
	var rawResp map[string]interface{}
	if err := json.Unmarshal(outputBytes, &rawResp); err != nil {
		logger.Debugf("Failed to parse JSON: %v, Output: %s", err, string(outputBytes))
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	downloadDataRaw, exists := rawResp["0"]
	if !exists {
		return nil, errors.New("no '0' key in response")
	}

	// Convert the raw data to JSON bytes
	downloadBytes, err := json.Marshal(downloadDataRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal download data: %w", err)
	}

	// Check if this has VeloResponse structure (with ReturnCode) or direct download data
	var tempCheck map[string]interface{}
	if err := json.Unmarshal(downloadBytes, &tempCheck); err != nil {
		return nil, fmt.Errorf("failed to parse download data structure: %w", err)
	}

	// If it has ReturnCode, it's standard VeloResponse format
	if retCode, hasRetCode := tempCheck["returncode"]; hasRetCode {
		if retCodeInt, ok := retCode.(float64); ok && retCodeInt != 0 {
			stderr, _ := tempCheck["stderr"].(string)
			logger.Debugf("File download failed with code=%.0f, stderr=%s", retCodeInt, stderr)
			return nil, fmt.Errorf("file download failed with code=%.0f, stderr=%s", retCodeInt, stderr)
		}
		// Parse stdout as download response
		if stdout, hasStdout := tempCheck["stdout"].(string); hasStdout {
			var downloadResp VeloDownloadResponse
			if err := json.Unmarshal([]byte(stdout), &downloadResp); err != nil {
				return nil, fmt.Errorf("failed to parse download response from stdout: %w", err)
			}

			if downloadResp.Error != "" {
				logger.Debugf("File download error: %s", downloadResp.Error)
				return nil, fmt.Errorf("file download error: %s", downloadResp.Error)
			}

			logger.Infof("File downloaded successfully - Path: %s, Size: %d bytes, MD5: %s",
				downloadResp.Path, downloadResp.Size, downloadResp.Md5)

			return &downloadResp, nil
		}
	}

	// Direct download response format - parse as VeloDownloadResponse
	var downloadResp VeloDownloadResponse
	if err := json.Unmarshal(downloadBytes, &downloadResp); err != nil {
		return nil, fmt.Errorf("failed to parse download response: %w", err)
	}

	// Check for download errors
	if downloadResp.Error != "" {
		logger.Debugf("File download error: %s", downloadResp.Error)
		return nil, fmt.Errorf("file download error: %s", downloadResp.Error)
	}

	logger.Infof("File downloaded successfully - Path: %s, Size: %d bytes, MD5: %s",
		downloadResp.Path, downloadResp.Size, downloadResp.Md5)

	return &downloadResp, nil
}

// executeConnectionTestVeloArtifact executes database connection test via VeloArtifact
// Supports both Linux and Windows platforms with appropriate artifacts
func executeConnectionTestVeloArtifact(clientID string, params ConnectionTestParams, osType string) (*VeloResponse, error) {
	maxRetries := config.Cfg.VeloMaxRetries

	logger.Debugf("Starting connection test with retry - clientID: %s, osType: %s, type: %s, host: %s:%d",
		clientID, osType, params.Type, params.Host, params.Port)

	// Determine artifact name and executable path based on OS
	var artifactName string
	var executablePath string

	switch strings.ToLower(osType) {
	case "linux":
		artifactName = "Linux.Sys.BashShell"
		executablePath = "/etc/v2/dbf/bin/v2dbfsqldetector"
	case "windows":
		artifactName = "Windows.System.CmdShell"
		// Use 8.3 short path to avoid spaces and quote escaping issues
		// PROGRA~1 is the short name for "Program Files"
		executablePath = "C:/PROGRA~1/V2/DBF/bin/sqldetector.exe"
	default:
		logger.Errorf("Unknown OS type: %s", osType)
		return nil, fmt.Errorf("unknown os_type: %s", osType)
	}

	// Build sqldetector command
	// Note: Do not add quotes around parameters - the executable handles them internally
	command := fmt.Sprintf("%s --action %s --type %s --host %s --port %d --username %s --password %s",
		executablePath, params.Action, params.Type, params.Host, params.Port, params.Username, params.Password)

	// Create JSON parameters for shell artifact
	// Use json.Marshal to properly escape all special characters
	cmdParams := map[string]string{"Command": command}
	paramBytes, err := json.Marshal(cmdParams)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal VeloArtifact parameters: %w", err)
	}
	paramJSON := string(paramBytes)

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			// Use lookup table to avoid gosec G115 false positive on bit shift
			delays := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second}
			delayIndex := attempt - 2
			if delayIndex >= len(delays) {
				delayIndex = len(delays) - 1
			}
			delay := delays[delayIndex]
			logger.Warnf("Connection test attempt %d failed, retrying in %v...", attempt-1, delay)
			time.Sleep(delay)
		}

		logger.Debugf("Connection test attempt %d/%d - clientID: %s", attempt, maxRetries, clientID)

		result, err := executeConnectionTestAttempt(clientID, paramJSON, artifactName)
		if err == nil {
			if attempt > 1 {
				logger.Infof("Connection test succeeded on attempt %d/%d", attempt, maxRetries)
			}
			return result, nil
		}

		lastErr = err

		// Check if this is a retryable error
		if !isRetryableError(err) {
			logger.Errorf("Non-retryable connection test error on attempt %d: %v", attempt, err)
			return nil, err
		}

		logger.Warnf("Retryable connection test error on attempt %d/%d: %v", attempt, maxRetries, err)
	}

	logger.Errorf("Connection test failed after %d attempts, last error: %v", maxRetries, lastErr)
	return nil, fmt.Errorf("connection test failed after %d attempts: %w", maxRetries, lastErr)
}

// executeConnectionTestAttempt performs a single attempt of connection test execution
func executeConnectionTestAttempt(clientID, paramJSON, artifactName string) (*VeloResponse, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), config.Cfg.VeloExecutionTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		"sudo",
		config.Cfg.VeloClientPath, // path from config
		"-c", clientID,            // client (endpoint) ID
		"-a", artifactName, // artifact name for shell execution
		"-p", paramJSON, // JSON param with command
	)

	logger.Debugf("Executing connection test command with timeout %v: sudo %s -c %s -a %s -p '%s'",
		config.Cfg.VeloExecutionTimeout, config.Cfg.VeloClientPath, clientID, artifactName, paramJSON)

	outputBytes, err := cmd.Output()
	if err != nil {
		// Check if error is due to timeout
		if ctx.Err() == context.DeadlineExceeded {
			logger.Debugf("Connection test timed out after %v", config.Cfg.VeloExecutionTimeout)
			return nil, fmt.Errorf("connection test timed out after %v", config.Cfg.VeloExecutionTimeout)
		}
		logger.Debugf("Connection test command execution failed: %v", err)
		return nil, fmt.Errorf("failed to run connection test command: %v", err)
	}

	logger.Debugf("Connection test output: %s", string(outputBytes))

	// Parse VeloArtifact response
	var resp VeloResponse
	if err := json.Unmarshal(outputBytes, &resp); err != nil {
		logger.Debugf("JSON parsing failed: %v, Output: %s", err, string(outputBytes))
		return nil, fmt.Errorf("failed to parse connection test response: %v\nOutput was: %s", err, string(outputBytes))
	}

	// Validate response structure
	val, ok := resp["0"]
	if !ok {
		logger.Debugf("Missing '0' key in connection test response")
		return nil, errors.New(`no "0" key in connection test response`)
	}

	// Log the complete response for debugging
	logger.Debugf("Connection test VeloResponse: Complete=%t, ReturnCode=%d, Stdout=%s, Stderr=%s",
		val.Complete, val.ReturnCode, val.Stdout, val.Stderr)

	// Return the full response for further processing by the caller
	return &resp, nil
}

// parseConnectionTestResult parses the stdout from connection test to extract results
func parseConnectionTestResult(stdout string) ([]ConnectionTestResult, error) {
	if strings.TrimSpace(stdout) == "" {
		return nil, fmt.Errorf("connection test returned empty output")
	}

	var results []ConnectionTestResult
	if err := json.Unmarshal([]byte(stdout), &results); err != nil {
		logger.Debugf("Failed to parse connection test results as JSON: %v", err)
		return nil, fmt.Errorf("failed to parse connection test results: %v", err)
	}

	return results, nil
}
