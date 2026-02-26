package agent

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"dbfartifactapi/config"
	"dbfartifactapi/pkg/logger"
)

// AgentDownloadResponse represents response from file download operation via dbfAgentAPI
type AgentDownloadResponse struct {
	LocalPath  string `json:"local_path"`  // Path where file was downloaded locally
	RemotePath string `json:"remote_path"` // Original remote path on agent
	Md5        string `json:"md5"`         // MD5 hash of downloaded file (computed after download)
	Size       int64  `json:"size"`        // File size in bytes
}

// AgentAPIResponse represents response from dbfAgentAPI command execution
type AgentAPIResponse struct {
	Status      string `json:"status"`       // success, error, timeout
	ClientID    string `json:"client_id"`    // Agent ID (e.g., L.005056b02e60)
	Command     string `json:"command"`      // Full command executed
	ExitCode    int    `json:"exit_code"`    // Command exit code
	ExecutedAt  string `json:"executed_at"`  // Execution timestamp
	Output      string `json:"output"`       // Command stdout
	ErrorOutput string `json:"error_output"` // Command stderr
	Message     string `json:"message"`      // Additional message (e.g., error details)
}

// ExecuteSqlAgentAPI executes SQL commands via dbfAgentAPI with retry mechanism.
// Replaces executeSqlVeloArtifact for direct agent communication without Velociraptor artifacts.
// Supports execute, download, policycompliance, and os_execute actions for dbfsqlexecute binary.
// Option parameter is passed directly to the binary (e.g., "--background" for os_execute).
func ExecuteSqlAgentAPI(agentID, osType, action, hexEncodedJSON, option string, requiredStdout bool) (string, error) {
	maxRetries := config.Cfg.AgentMaxRetries

	logger.Debugf("Starting dbfAgentAPI SQL execution - agentID: %s, osType: %s, action: %s, maxRetries: %d",
		agentID, osType, action, maxRetries)

	// Validate action for SQL operations
	validActions := map[string]bool{
		"execute":          true,
		"download":         true,
		"policycompliance": true,
		"os_execute":       true,
		"upload":           true,
		"filedownload":     true,
	}
	if !validActions[action] {
		return "", fmt.Errorf("invalid SQL action: %s (must be one of: execute, download, policycompliance, os_execute, upload, filedownload)", action)
	}

	// Build executable path based on OS type
	var executablePath string
	switch strings.ToLower(osType) {
	case "linux":
		executablePath = "/etc/v2/dbf/bin/dbfsqlexecute"
	case "windows":
		executablePath = "C:/PROGRA~1/V2/DBF/bin/dbfsqlexecute"
	default:
		logger.Errorf("Unknown OS type: %s", osType)
		return "", fmt.Errorf("unknown os_type: %s", osType)
	}

	// Build full command string for dbfsqlexecute
	var command string
	if option != "" {
		command = fmt.Sprintf("%s %s %s %s", executablePath, action, hexEncodedJSON, option)
		logger.Debugf("Constructed command: %s %s <hex_json> %s", executablePath, action, option)
	} else {
		command = fmt.Sprintf("%s %s %s", executablePath, action, hexEncodedJSON)
		logger.Debugf("Constructed command: %s %s <hex_json>", executablePath, action)
	}

	// Determine if empty output is acceptable (download, policycompliance, and os_execute with --background may not return output)
	allowEmptyOutput := (action == "download" || action == "policycompliance" || action == "os_execute")

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			// Exponential backoff using lookup table to avoid gosec G115 false positive
			delays := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second}
			delayIndex := attempt - 2
			if delayIndex >= len(delays) {
				delayIndex = len(delays) - 1
			}
			delay := delays[delayIndex]
			logger.Warnf("dbfAgentAPI attempt %d failed, retrying in %v...", attempt-1, delay)
			time.Sleep(delay)
		}

		logger.Debugf("dbfAgentAPI attempt %d/%d - agentID: %s", attempt, maxRetries, agentID)

		result, err := executeAgentAPIAttempt(agentID, command, requiredStdout, allowEmptyOutput)
		if err == nil {
			if attempt > 1 {
				logger.Infof("dbfAgentAPI succeeded on attempt %d/%d", attempt, maxRetries)
			}
			return result, nil
		}

		lastErr = err

		// Check if this is a retryable error
		if !isRetryableAgentError(err) {
			logger.Errorf("Non-retryable error on attempt %d: %v", attempt, err)
			return "", err
		}

		logger.Warnf("Retryable error on attempt %d/%d: %v", attempt, maxRetries, err)
	}

	logger.Errorf("dbfAgentAPI failed after %d attempts, last error: %v", maxRetries, lastErr)
	return "", fmt.Errorf("dbfAgentAPI failed after %d attempts: %w", maxRetries, lastErr)
}

// executeAgentAPIAttempt performs a single attempt of dbfAgentAPI command execution with timeout.
// Generic function that can be reused for different command types (SQL, file operations, etc.)
func executeAgentAPIAttempt(agentID, command string, requiredStdout, allowEmptyOutput bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.Cfg.AgentExecutionTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		"sudo",
		config.Cfg.AgentAPIPath,
		"--json",
		"cmd",
		agentID,
		command,
	)

	logger.Debugf("Executing command with timeout %v: sudo %s --json cmd %s '%s'",
		config.Cfg.AgentExecutionTimeout, config.Cfg.AgentAPIPath, agentID, command)

	outputBytes, err := cmd.Output()
	if err != nil {
		// Check if error is due to timeout
		if ctx.Err() == context.DeadlineExceeded {
			logger.Debugf("dbfAgentAPI execution timed out after %v", config.Cfg.AgentExecutionTimeout)
			return "", fmt.Errorf("dbfAgentAPI execution timed out after %v", config.Cfg.AgentExecutionTimeout)
		}
		logger.Debugf("dbfAgentAPI command execution failed: %v", err)
		return "", fmt.Errorf("failed to run dbfAgentAPI: %v", err)
	}

	logger.Debugf("dbfAgentAPI output: %s", string(outputBytes))

	var resp AgentAPIResponse
	if err := json.Unmarshal(outputBytes, &resp); err != nil {
		logger.Debugf("JSON parsing failed: %v, Output: %s", err, string(outputBytes))
		return "", fmt.Errorf("failed to parse dbfAgentAPI response: %v\nOutput was: %s", err, string(outputBytes))
	}

	logger.Debugf("AgentAPIResponse: status=%s, exit_code=%d, client_id=%s",
		resp.Status, resp.ExitCode, resp.ClientID)

	// Validate response status
	if resp.Status == "timeout" {
		logger.Debugf("Agent command timed out: %s", resp.Message)
		return "", fmt.Errorf("agent command timed out: %s", resp.Message)
	}

	if resp.Status == "error" {
		logger.Debugf("Agent command failed: exit_code=%d, message=%s, error_output=%s",
			resp.ExitCode, resp.Message, resp.ErrorOutput)
		return resp.Output, fmt.Errorf("agent command failed with exit_code=%d: %s", resp.ExitCode, resp.Message)
	}

	// Status is "success"
	if resp.ExitCode != 0 {
		logger.Warnf("Agent command returned non-zero exit code %d but status is success", resp.ExitCode)
	}

	// Validate output requirements
	if requiredStdout && !allowEmptyOutput && resp.Output == "" {
		logger.Debugf("Agent command returned empty output when output was required")
		return "", fmt.Errorf("agent command returned empty output")
	}

	logger.Debugf("dbfAgentAPI execution completed successfully")
	return resp.Output, nil
}

// isRetryableAgentError determines if an agent API error is worth retrying
func isRetryableAgentError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Common retryable error patterns for agent communication
	retryablePatterns := []string{
		"failed to parse",               // JSON parsing issue
		"command returned empty output", // Empty response
		"connection refused",            // Network issue
		"timeout",                       // Timeout issue
		"timed out after",               // Context timeout
		"temporary failure",             // Temporary failure
		"service unavailable",           // Service issue
		"context deadline exceeded",     // Context timeout
		"dial tcp",                      // Network dial error
		"no route to host",              // Network routing issue
		"network is unreachable",        // Network unreachable
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	// Non-retryable errors
	nonRetryablePatterns := []string{
		"permission denied",
		"unauthorized",
		"access denied",
		"unknown os_type",
		"invalid",
		"authentication failed",
	}

	for _, pattern := range nonRetryablePatterns {
		if strings.Contains(errStr, pattern) {
			return false
		}
	}

	// By default, treat command execution failures as retryable
	return true
}

// ExecuteAgentAPISimpleCommand executes simple dbfsqlexecute commands that don't use hex-encoded JSON.
// Used for operations like checkstatus, getresults, listjobs, cleanup that pass plain values.
// Command format: /etc/v2/dbf/bin/dbfsqlexecute <action> <value> [option]
func ExecuteAgentAPISimpleCommand(agentID, osType, action, value, option string, requiredStdout bool) (string, error) {
	maxRetries := config.Cfg.AgentMaxRetries

	logger.Debugf("Starting dbfAgentAPI simple command - agentID: %s, osType: %s, action: %s, value: %s",
		agentID, osType, action, value)

	// Validate action for simple operations
	validActions := map[string]bool{
		"checkstatus": true,
		"getresults":  true,
		"listjobs":    true,
		"cleanup":     true,
		"info":        true,
	}
	if !validActions[action] {
		return "", fmt.Errorf("invalid simple action: %s (must be one of: checkstatus, getresults, listjobs, cleanup, info)", action)
	}

	// Build executable path based on OS type
	var executablePath string
	switch strings.ToLower(osType) {
	case "linux":
		executablePath = "/etc/v2/dbf/bin/dbfsqlexecute"
	case "windows":
		executablePath = "C:/PROGRA~1/V2/DBF/bin/dbfsqlexecute"
	default:
		logger.Errorf("Unknown OS type: %s", osType)
		return "", fmt.Errorf("unknown os_type: %s", osType)
	}

	// Build full command string for dbfsqlexecute
	var command string
	if option != "" {
		command = fmt.Sprintf("%s %s %s %s", executablePath, action, value, option)
	} else {
		command = fmt.Sprintf("%s %s %s", executablePath, action, value)
	}

	logger.Debugf("Constructed simple command: %s", command)

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			delays := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second}
			delayIndex := attempt - 2
			if delayIndex >= len(delays) {
				delayIndex = len(delays) - 1
			}
			delay := delays[delayIndex]
			logger.Warnf("dbfAgentAPI simple command attempt %d failed, retrying in %v...", attempt-1, delay)
			time.Sleep(delay)
		}

		logger.Debugf("dbfAgentAPI simple command attempt %d/%d - agentID: %s", attempt, maxRetries, agentID)

		result, err := executeAgentAPIAttempt(agentID, command, requiredStdout, false)
		if err == nil {
			if attempt > 1 {
				logger.Infof("dbfAgentAPI simple command succeeded on attempt %d/%d", attempt, maxRetries)
			}
			return result, nil
		}

		lastErr = err

		if !isRetryableAgentError(err) {
			logger.Errorf("Non-retryable error on attempt %d: %v", attempt, err)
			return "", err
		}

		logger.Warnf("Retryable error on attempt %d/%d: %v", attempt, maxRetries, err)
	}

	logger.Errorf("dbfAgentAPI simple command failed after %d attempts, last error: %v", maxRetries, lastErr)
	return "", fmt.Errorf("dbfAgentAPI simple command failed after %d attempts: %w", maxRetries, lastErr)
}

// executeAgentAPIGetFile downloads a file from agent to local path.
// Command format: dbfAgentAPI getfile <agentID> <remote_path> <local_path>
func executeAgentAPIGetFile(agentID, remotePath, localPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.Cfg.AgentExecutionTimeout)
	defer cancel()

	logger.Debugf("Starting dbfAgentAPI getfile - agentID: %s, remotePath: %s, localPath: %s",
		agentID, remotePath, localPath)

	cmd := exec.CommandContext(ctx,
		"sudo",
		config.Cfg.AgentAPIPath,
		"getfile",
		agentID,
		remotePath,
		localPath,
	)

	logger.Debugf("Executing getfile command: sudo %s getfile %s %s %s",
		config.Cfg.AgentAPIPath, agentID, remotePath, localPath)

	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			logger.Errorf("dbfAgentAPI getfile timed out after %v", config.Cfg.AgentExecutionTimeout)
			return fmt.Errorf("dbfAgentAPI getfile timed out after %v", config.Cfg.AgentExecutionTimeout)
		}
		logger.Errorf("dbfAgentAPI getfile failed: %v, output: %s", err, string(outputBytes))
		return fmt.Errorf("failed to download file from agent: %v, output: %s", err, string(outputBytes))
	}

	logger.Infof("dbfAgentAPI getfile completed successfully: %s -> %s", remotePath, localPath)
	return nil
}

// DownloadFileAgentAPI downloads a file from agent to local storage and returns metadata.
// Similar to downloadFileVeloArtifact but uses dbfAgentAPI getfile command.
// Files are downloaded to {VeloResultsDir}/{agentID}/ with MD5-based filename for compatibility.
func DownloadFileAgentAPI(agentID, remotePath, osType string) (*AgentDownloadResponse, error) {
	maxRetries := config.Cfg.AgentMaxRetries

	// Convert Windows path backslashes to forward slashes
	if strings.ToLower(osType) == "windows" {
		remotePath = strings.ReplaceAll(remotePath, "\\", "/")
	}

	logger.Debugf("Starting agent file download - agentID: %s, remotePath: %s, osType: %s", agentID, remotePath, osType)

	// Generate unique local path using timestamp and remote filename
	timestamp := time.Now().UnixNano()
	remoteFilename := filepath.Base(remotePath)
	tempFilename := fmt.Sprintf("%d_%s", timestamp, remoteFilename)

	// Create download directory: {VeloResultsDir}/{agentID}/
	downloadDir := filepath.Join(config.Cfg.VeloResultsDir, agentID)
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create download directory %s: %w", downloadDir, err)
	}

	tempLocalPath := filepath.Join(downloadDir, tempFilename)

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			delays := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second}
			delayIndex := attempt - 2
			if delayIndex >= len(delays) {
				delayIndex = len(delays) - 1
			}
			delay := delays[delayIndex]
			logger.Warnf("Agent file download attempt %d failed, retrying in %v...", attempt-1, delay)
			time.Sleep(delay)
		}

		logger.Debugf("Agent file download attempt %d/%d - agentID: %s, remotePath: %s", attempt, maxRetries, agentID, remotePath)

		err := executeAgentAPIGetFile(agentID, remotePath, tempLocalPath)
		if err == nil {
			if attempt > 1 {
				logger.Infof("Agent file download succeeded on attempt %d/%d", attempt, maxRetries)
			}

			// Calculate MD5 hash of downloaded file
			fileData, readErr := os.ReadFile(tempLocalPath)
			if readErr != nil {
				return nil, fmt.Errorf("failed to read downloaded file for MD5 calculation: %w", readErr)
			}

			md5Hash := md5.Sum(fileData)
			md5Hex := hex.EncodeToString(md5Hash[:])

			// Rename file to MD5-based name for compatibility with existing code
			finalLocalPath := filepath.Join(downloadDir, md5Hex)
			if err := os.Rename(tempLocalPath, finalLocalPath); err != nil {
				// If rename fails (e.g., file already exists), use the temp path
				logger.Warnf("Failed to rename downloaded file to MD5 name: %v, using temp path", err)
				finalLocalPath = tempLocalPath
			}

			// Get file info for size
			fileInfo, statErr := os.Stat(finalLocalPath)
			var fileSize int64
			if statErr == nil {
				fileSize = fileInfo.Size()
			}

			return &AgentDownloadResponse{
				LocalPath:  finalLocalPath,
				RemotePath: remotePath,
				Md5:        md5Hex,
				Size:       fileSize,
			}, nil
		}

		lastErr = err

		if !isRetryableAgentError(err) {
			logger.Errorf("Non-retryable agent file download error on attempt %d: %v", attempt, err)
			return nil, err
		}

		logger.Warnf("Retryable agent file download error on attempt %d/%d: %v", attempt, maxRetries, err)
	}

	logger.Errorf("Agent file download failed after %d attempts, last error: %v", maxRetries, lastErr)
	return nil, fmt.Errorf("agent file download failed after %d attempts: %w", maxRetries, lastErr)
}

// ConnectionTestAgentParams represents parameters for database connection testing via agent API
type ConnectionTestAgentParams struct {
	Action      string `json:"action"`
	Type        string `json:"type"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	ServiceName string `json:"service_name"`
}

// ExecuteConnectionTestAgentAPI executes database connection test via dbfAgentAPI.
// Uses v2dbfsqldetector/sqldetector.exe on the remote agent.
func ExecuteConnectionTestAgentAPI(clientID string, params ConnectionTestAgentParams, osType string) (*AgentAPIResponse, error) {
	maxRetries := config.Cfg.AgentMaxRetries

	logger.Debugf("Starting connection test via agent API - clientID: %s, osType: %s, type: %s, host: %s:%d",
		clientID, osType, params.Type, params.Host, params.Port)

	// Determine executable path based on OS
	var executablePath string
	switch strings.ToLower(osType) {
	case "linux":
		executablePath = "/etc/v2/dbf/bin/v2dbfsqldetector"
	case "windows":
		executablePath = "C:/PROGRA~1/V2/DBF/bin/sqldetector.exe"
	default:
		logger.Errorf("Unknown OS type: %s", osType)
		return nil, fmt.Errorf("unknown os_type: %s", osType)
	}

	// Build sqldetector command
	command := fmt.Sprintf("%s --action %s --type %s --host %s --port %d --username %s --password %s",
		executablePath, params.Action, params.Type, params.Host, params.Port, params.Username, params.Password)

	// Oracle connections require service name to identify the target database instance
	if strings.ToLower(params.Type) == "oracle" && params.ServiceName != "" {
		command += fmt.Sprintf(" --service_name %s", params.ServiceName)
	}

	logger.Debugf("Constructed connection test command: %s --action %s --type %s --host %s --port %d --username *** --password *** --service_name %s",
		executablePath, params.Action, params.Type, params.Host, params.Port, params.ServiceName)

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
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

		// Execute command via agent API
		ctx, cancel := context.WithTimeout(context.Background(), config.Cfg.AgentExecutionTimeout)

		cmd := exec.CommandContext(ctx,
			"sudo",
			config.Cfg.AgentAPIPath,
			"--json",
			"cmd",
			clientID,
			command,
		)

		outputBytes, err := cmd.Output()
		cancel()

		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				lastErr = fmt.Errorf("connection test timed out after %v", config.Cfg.AgentExecutionTimeout)
			} else {
				lastErr = fmt.Errorf("failed to run dbfAgentAPI: %v", err)
			}

			if !isRetryableAgentError(lastErr) {
				logger.Errorf("Non-retryable connection test error on attempt %d: %v", attempt, lastErr)
				return nil, lastErr
			}

			logger.Warnf("Retryable connection test error on attempt %d/%d: %v", attempt, maxRetries, lastErr)
			continue
		}

		// Parse agent API response
		var agentResp AgentAPIResponse
		if err := json.Unmarshal(outputBytes, &agentResp); err != nil {
			lastErr = fmt.Errorf("failed to parse agent API response: %v", err)
			logger.Warnf("JSON parsing error on attempt %d: %v", attempt, lastErr)
			continue
		}

		// Check agent response status
		if agentResp.Status == "timeout" {
			lastErr = fmt.Errorf("agent command timed out: %s", agentResp.Message)
			logger.Warnf("Agent timeout on attempt %d: %v", attempt, lastErr)
			continue
		}

		if attempt > 1 {
			logger.Infof("Connection test succeeded on attempt %d/%d", attempt, maxRetries)
		}

		return &agentResp, nil
	}

	logger.Errorf("Connection test failed after %d attempts, last error: %v", maxRetries, lastErr)
	return nil, fmt.Errorf("connection test failed after %d attempts: %w", maxRetries, lastErr)
}
