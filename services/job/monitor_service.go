package job

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services/agent"
)

// JobCompletionCallback is called when a job completes (success or failure)
type JobCompletionCallback func(jobID string, jobInfo *JobInfo, statusResp *StatusResponse) error

// JobInfo stores information about a running job
type JobInfo struct {
	JobID        string      `json:"job_id"`
	DBMgtID      uint        `json:"dbmgt_id"`
	ClientID     string      `json:"client_id"`
	OsType       string      `json:"os_type"`
	Status       string      `json:"status"`
	Progress     int         `json:"progress"`
	StartTime    time.Time   `json:"start_time"`
	EndTime      *time.Time  `json:"end_time,omitempty"`
	Message      string      `json:"message"`
	Completed    int         `json:"completed"`
	Failed       int         `json:"failed"`
	TotalQueries int         `json:"total_queries"`
	Error        string      `json:"error,omitempty"`
	Results      interface{} `json:"results,omitempty"`
	// Callback function called when job completes
	CompletionCallback JobCompletionCallback `json:"-"`
	// Context data for callback processing
	ContextData map[string]interface{} `json:"-"`
	// Flag to indicate job was already processed via notification
	ProcessedViaNotification bool `json:"-"`
}

// StatusResponse represents VeloArtifact status check response
type StatusResponse struct {
	Completed    int    `json:"completed"`
	CreatedAt    string `json:"created_at"`
	EndTime      string `json:"end_time"`
	Failed       int    `json:"failed"`
	JobID        string `json:"job_id"`
	Message      string `json:"message"`
	PID          int    `json:"pid"`
	Platform     string `json:"platform"`
	Progress     int    `json:"progress"`
	StartTime    string `json:"start_time"`
	Status       string `json:"status"`
	TotalQueries int    `json:"total_queries"`
	UpdatedAt    string `json:"updated_at"`
	// CRITICAL: Add error field to capture actual error messages from VeloArtifact
	Error string `json:"error"`
	// Results contains command execution results from agent (OS execute, SQL execute, etc.)
	Results []interface{} `json:"results,omitempty"`
}

// JobMonitorService manages background job monitoring
type JobMonitorService struct {
	jobs    map[string]*JobInfo
	mu      sync.RWMutex
	stopCh  chan struct{}
	stopped bool
	// Default completion callback for all jobs
	defaultCallback JobCompletionCallback
}

var (
	jobMonitorInstance *JobMonitorService
	jobMonitorOnce     sync.Once
)

// GetJobMonitorService returns singleton instance of JobMonitorService
func GetJobMonitorService() *JobMonitorService {
	jobMonitorOnce.Do(func() {
		jobMonitorInstance = &JobMonitorService{
			jobs:   make(map[string]*JobInfo),
			stopCh: make(chan struct{}),
		}
		// Start the monitoring goroutine
		go jobMonitorInstance.startMonitoring()
	})
	return jobMonitorInstance
}

// AddJob adds a new job to monitoring
func (jms *JobMonitorService) AddJob(jobID string, dbmgtID uint, clientID, osType string) {
	jms.AddJobWithCallback(jobID, dbmgtID, clientID, osType, nil, nil)
}

// AddJobWithCallback adds a new job to monitoring with completion callback
func (jms *JobMonitorService) AddJobWithCallback(jobID string, dbmgtID uint, clientID, osType string, callback JobCompletionCallback, contextData map[string]interface{}) {
	jms.mu.Lock()
	defer jms.mu.Unlock()

	job := &JobInfo{
		JobID:              jobID,
		DBMgtID:            dbmgtID,
		ClientID:           clientID,
		OsType:             osType,
		Status:             "running",
		Progress:           0,
		StartTime:          time.Now(),
		Message:            "Job started",
		CompletionCallback: callback,
		ContextData:        contextData,
	}

	jms.jobs[jobID] = job
	logger.Infof("Added job %s to monitoring for dbmgt_id %d", jobID, dbmgtID)
}

// SetDefaultCallback sets the default completion callback for all jobs
func (jms *JobMonitorService) SetDefaultCallback(callback JobCompletionCallback) {
	jms.mu.Lock()
	defer jms.mu.Unlock()
	jms.defaultCallback = callback
}

// GetJob returns job information
func (jms *JobMonitorService) GetJob(jobID string) (*JobInfo, bool) {
	jms.mu.RLock()
	defer jms.mu.RUnlock()

	job, exists := jms.jobs[jobID]
	if exists {
		// Return a copy to avoid race conditions
		jobCopy := *job
		return &jobCopy, true
	}

	return nil, false
}

// GetAllJobs returns all jobs information
func (jms *JobMonitorService) GetAllJobs() map[string]JobInfo {
	jms.mu.RLock()
	defer jms.mu.RUnlock()

	result := make(map[string]JobInfo)
	for id, job := range jms.jobs {
		result[id] = *job
	}
	return result
}

// PaginatedJobsResult contains paginated jobs data with metadata
type PaginatedJobsResult struct {
	Jobs       []JobInfo `json:"jobs"`
	Total      int       `json:"total"`
	Page       int       `json:"page"`
	PageSize   int       `json:"page_size"`
	TotalPages int       `json:"total_pages"`
}

// GetAllJobsPaginated returns paginated jobs information.
// Converts internal map to sorted slice for consistent pagination ordering.
// Returns empty jobs array when page exceeds available data to prevent client errors.
func (jms *JobMonitorService) GetAllJobsPaginated(page, pageSize int) *PaginatedJobsResult {
	jms.mu.RLock()
	defer jms.mu.RUnlock()

	// API consumers expect 1-indexed pages, enforce minimum valid values before calculations
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// Map must be converted to slice since Go maps have undefined iteration order
	// Consistent ordering is required for reliable pagination across requests
	allJobs := make([]JobInfo, 0, len(jms.jobs))
	for _, job := range jms.jobs {
		allJobs = append(allJobs, *job)
	}

	total := len(allJobs)
	totalPages := (total + pageSize - 1) / pageSize

	start := (page - 1) * pageSize
	end := start + pageSize

	// Return empty array instead of error when page exceeds data to simplify client handling
	if start >= total {
		return &PaginatedJobsResult{
			Jobs:       []JobInfo{},
			Total:      total,
			Page:       page,
			PageSize:   pageSize,
			TotalPages: totalPages,
		}
	}
	if end > total {
		end = total
	}

	return &PaginatedJobsResult{
		Jobs:       allJobs[start:end],
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

// GetJobsByDBMgt returns jobs for specific database management ID
func (jms *JobMonitorService) GetJobsByDBMgt(dbmgtID uint) []JobInfo {
	jms.mu.RLock()
	defer jms.mu.RUnlock()

	var result []JobInfo
	for _, job := range jms.jobs {
		if job.DBMgtID == dbmgtID {
			result = append(result, *job)
		}
	}
	return result
}

// RemoveJob removes a job from monitoring
func (jms *JobMonitorService) RemoveJob(jobID string) {
	jms.mu.Lock()
	defer jms.mu.Unlock()

	delete(jms.jobs, jobID)
	logger.Debugf("Removed job %s from monitoring", jobID)
}

// Stop stops the monitoring service
func (jms *JobMonitorService) Stop() {
	jms.mu.Lock()
	defer jms.mu.Unlock()

	if !jms.stopped {
		close(jms.stopCh)
		jms.stopped = true
		logger.Infof("Job monitor service stopped")
	}
}

// startMonitoring runs the background monitoring loop
func (jms *JobMonitorService) startMonitoring() {
	ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
	defer ticker.Stop()

	logger.Infof("Job monitor service started")

	for {
		select {
		case <-jms.stopCh:
			logger.Infof("Job monitoring stopped")
			return

		case <-ticker.C:
			jms.checkAllJobs()
		}
	}
}

// checkAllJobs checks status of all running jobs
func (jms *JobMonitorService) checkAllJobs() {
	jms.mu.RLock()
	runningJobs := make([]*JobInfo, 0)
	for _, job := range jms.jobs {
		if job.Status == "running" {
			runningJobs = append(runningJobs, job)
		}
	}
	jms.mu.RUnlock()

	for _, job := range runningJobs {
		jms.checkJobStatus(job)
	}
}

// checkJobStatus checks the status of a specific job
func (jms *JobMonitorService) checkJobStatus(job *JobInfo) {
	// Skip VeloArtifact status check if job is marked as no-polling
	// (e.g., master jobs that don't exist on VeloArtifact)
	if job.ProcessedViaNotification {
		logger.Debugf("Skipping VeloArtifact status check for job %s - marked as no-polling", job.JobID)
		return
	}

	statusOutput, err := agent.ExecuteAgentAPISimpleCommand(job.ClientID, job.OsType, "checkstatus", job.JobID, "", true)
	if err != nil {
		logger.Errorf("Failed to check job status for %s: %v", job.JobID, err)
		jms.updateJobError(job.JobID, fmt.Sprintf("Status check failed: %v", err))
		return
	}

	var statusResp StatusResponse
	if err := json.Unmarshal([]byte(statusOutput), &statusResp); err != nil {
		logger.Errorf("Failed to parse status response for job %s: %v", job.JobID, err)
		jms.updateJobError(job.JobID, fmt.Sprintf("Parse error: %v", err))
		return
	}

	// Update job status
	jms.updateJobStatus(job.JobID, &statusResp)

	logger.Debugf("Job %s status: %s, progress: %d%%", job.JobID, statusResp.Status, statusResp.Progress)

	// Handle completion
	switch statusResp.Status {
	case "completed":
		logger.Infof("Job %s completed successfully: %s", job.JobID, statusResp.Message)
		// Execute completion callback
		jms.executeCompletionCallback(job.JobID, job, &statusResp)
	case "failed":
		logger.Errorf("Job %s failed: %s", job.JobID, statusResp.Message)
		// Execute completion callback even for failed jobs
		jms.executeCompletionCallback(job.JobID, job, &statusResp)
	}
}

// updateJobStatus updates job status from StatusResponse
func (jms *JobMonitorService) updateJobStatus(jobID string, statusResp *StatusResponse) {
	jms.mu.Lock()
	defer jms.mu.Unlock()

	job, exists := jms.jobs[jobID]
	if !exists {
		return
	}

	// Skip update if job was already completed via notification to prevent race condition
	// where polling response arrives after notification and overwrites completed status
	if job.ProcessedViaNotification {
		logger.Debugf("Skipping status update for job %s - already processed via notification", jobID)
		return
	}

	// Client reported completion - set to "processing" to indicate server-side processing
	// Completion handler will update to "completed" or "failed" after processing
	if statusResp.Status == "completed" && job.CompletionCallback != nil {
		job.Status = "processing"
		job.Progress = statusResp.Progress
		job.Message = "Client completed, server processing results"
	} else {
		job.Status = statusResp.Status
		job.Progress = statusResp.Progress
		job.Message = statusResp.Message
	}

	job.Completed = statusResp.Completed
	job.Failed = statusResp.Failed
	job.TotalQueries = statusResp.TotalQueries

	// CRITICAL: Extract and save error message from VeloArtifact response
	// This ensures actual error details are available for client display
	if statusResp.Status == "failed" && statusResp.Error != "" {
		job.Error = statusResp.Error
	}

	// Don't set EndTime yet for "processing" status - let handler do it
	if statusResp.Status == "failed" {
		now := time.Now()
		job.EndTime = &now
	}
}

// updateJobError updates job with error information
func (jms *JobMonitorService) updateJobError(jobID, errorMsg string) {
	jms.mu.Lock()
	defer jms.mu.Unlock()

	job, exists := jms.jobs[jobID]
	if !exists {
		return
	}

	job.Status = "error"
	job.Error = errorMsg
	now := time.Now()
	job.EndTime = &now
}

// UpdateJobResults updates job results with proper locking.
// Merges new results into existing map-based results.
func (jms *JobMonitorService) UpdateJobResults(jobID string, results map[string]interface{}) {
	jms.mu.Lock()
	defer jms.mu.Unlock()

	job, exists := jms.jobs[jobID]
	if !exists {
		logger.Warnf("Attempted to update results for non-existent job %s", jobID)
		return
	}

	existingMap, ok := job.Results.(map[string]interface{})
	if !ok || existingMap == nil {
		existingMap = make(map[string]interface{})
	}

	for key, value := range results {
		existingMap[key] = value
	}
	job.Results = existingMap

	logger.Debugf("Updated results for job %s with %d entries", jobID, len(results))
}

// SetJobResults sets job results directly (for array-based results like OS execute).
func (jms *JobMonitorService) SetJobResults(jobID string, results interface{}) {
	jms.mu.Lock()
	defer jms.mu.Unlock()

	job, exists := jms.jobs[jobID]
	if !exists {
		logger.Warnf("Attempted to set results for non-existent job %s", jobID)
		return
	}

	job.Results = results
	logger.Debugf("Set results for job %s", jobID)
}

// MarkJobAsNoPolling marks a job to skip VeloArtifact status polling
// Used for master/tracking jobs that don't exist on VeloArtifact
func (jms *JobMonitorService) MarkJobAsNoPolling(jobID string) error {
	jms.mu.Lock()
	defer jms.mu.Unlock()

	job, exists := jms.jobs[jobID]
	if !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	// Mark as processed to prevent polling loop from checking VeloArtifact status
	job.ProcessedViaNotification = true
	logger.Infof("Marked job %s to skip VeloArtifact polling", jobID)

	return nil
}

// CompleteJobImmediately marks a job as completed without polling
// Used for jobs where results are available immediately (e.g., SQL execute)
func (jms *JobMonitorService) CompleteJobImmediately(jobID, message string, totalCompleted int) error {
	return jms.CompleteJobWithResults(jobID, "completed", message, totalCompleted, 0)
}

// CompleteJobWithResults marks a job with specific status and results without polling.
// Critical for master job status accuracy - ensures failed sub-jobs propagate to master job status.
// Returns ErrJobNotFound if job ID doesn't exist.
func (jms *JobMonitorService) CompleteJobWithResults(jobID, status, message string, totalCompleted, failedCount int) error {
	jms.mu.Lock()
	defer jms.mu.Unlock()

	job, exists := jms.jobs[jobID]
	if !exists {
		return fmt.Errorf("job %s not found for immediate completion", jobID)
	}

	logger.Infof("Marking job as %s immediately: job_id=%s, message=%s", status, jobID, message)

	// Mark as processed to prevent monitoring loop from checking status
	job.ProcessedViaNotification = true

	// Update job status
	now := time.Now()
	job.Status = status
	if status == "completed" {
		job.Progress = 100
	} else {
		job.Progress = 0 // Failed jobs typically have 0 progress
	}
	job.Message = message
	job.Completed = totalCompleted
	job.Failed = failedCount
	job.EndTime = &now

	// Create StatusResponse for callback
	statusResp := &StatusResponse{
		JobID:     jobID,
		Status:    status,
		Progress:  job.Progress,
		Message:   message,
		Completed: totalCompleted,
		Failed:    failedCount,
		StartTime: job.StartTime.Format("2006-01-02T15:04:05Z"),
		EndTime:   now.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: now.Format("2006-01-02T15:04:05Z"),
		Error:     job.Error,
	}

	// Execute completion callback asynchronously
	go jms.executeCompletionCallbackAsync(jobID, job, statusResp)

	return nil
}

// ProcessJobNotification handles job completion notifications from external services
func (jms *JobMonitorService) ProcessJobNotification(jobID, fileName, md5Hash string, success bool) error {
	jms.mu.Lock()
	defer jms.mu.Unlock()

	job, exists := jms.jobs[jobID]
	if !exists {
		return fmt.Errorf("job %s not found for notification processing", jobID)
	}

	logger.Infof("Processing job notification: job_id=%s, file=%s, md5=%s, success=%v",
		jobID, fileName, md5Hash, success)

	// Mark job as processed via notification to prevent duplicate callback from polling
	job.ProcessedViaNotification = true

	// Update job status based on notification
	now := time.Now()
	if success {
		job.Status = "completed"
		job.Progress = 100
		job.Message = fmt.Sprintf("Job completed via notification - file: %s", fileName)
	} else {
		job.Status = "failed"
		job.Message = fmt.Sprintf("Job failed via notification - file: %s", fileName)
		job.Error = "Notified as failed by external service"
	}
	job.EndTime = &now

	// Add notification data to context for callback processing
	if job.ContextData == nil {
		job.ContextData = make(map[string]interface{})
	}
	job.ContextData["notification_data"] = map[string]interface{}{
		"fileName": fileName,
		"md5Hash":  md5Hash,
		"success":  success,
	}

	// Create a StatusResponse for callback compatibility
	statusResp := &StatusResponse{
		JobID:     jobID,
		Status:    job.Status,
		Progress:  job.Progress,
		Message:   job.Message,
		Completed: 1, // Assume 1 for notification-based completion
		Failed:    0,
		StartTime: job.StartTime.Format("2006-01-02T15:04:05Z"),
		EndTime:   now.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: now.Format("2006-01-02T15:04:05Z"),
		Error:     job.Error,
	}

	if !success {
		statusResp.Failed = 1
		statusResp.Completed = 0
	}

	// Execute completion callback asynchronously
	go jms.executeCompletionCallbackAsync(jobID, job, statusResp)

	return nil
}

// executeCompletionCallback executes the job completion callback
func (jms *JobMonitorService) executeCompletionCallback(jobID string, job *JobInfo, statusResp *StatusResponse) {
	jms.executeCompletionCallbackAsync(jobID, job, statusResp)
}

// executeCompletionCallbackAsync executes the job completion callback asynchronously
func (jms *JobMonitorService) executeCompletionCallbackAsync(jobID string, job *JobInfo, statusResp *StatusResponse) {
	// Use job-specific callback if available, otherwise use default
	callback := job.CompletionCallback
	if callback == nil {
		callback = jms.defaultCallback
	}

	if callback != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Job completion callback panic for %s: %v", jobID, r)
				}
			}()

			if err := callback(jobID, job, statusResp); err != nil {
				logger.Errorf("Job completion callback error for %s: %v", jobID, err)
			} else {
				logger.Infof("Job completion callback executed successfully for %s", jobID)
			}
		}()
	} else {
		logger.Debugf("No completion callback defined for job %s", jobID)
	}
}

// CompleteJobAfterProcessing marks job as completed after server-side processing finishes.
// Used when client reports completion but server still needs to process results.
func (jms *JobMonitorService) CompleteJobAfterProcessing(jobID, message string) error {
	jms.mu.Lock()
	defer jms.mu.Unlock()

	job, exists := jms.jobs[jobID]
	if !exists {
		return fmt.Errorf("job %s not found for completion", jobID)
	}

	logger.Infof("Marking job as completed after server processing: job_id=%s, message=%s", jobID, message)

	now := time.Now()
	job.Status = "completed"
	job.Progress = 100
	job.Message = message
	job.EndTime = &now

	return nil
}

// FailJobAfterProcessing marks job as failed after server-side processing fails.
// Used when client reports completion but server processing encounters errors.
func (jms *JobMonitorService) FailJobAfterProcessing(jobID, errorMsg string) error {
	jms.mu.Lock()
	defer jms.mu.Unlock()

	job, exists := jms.jobs[jobID]
	if !exists {
		return fmt.Errorf("job %s not found for failure", jobID)
	}

	logger.Errorf("Marking job as failed after server processing: job_id=%s, error=%s", jobID, errorMsg)

	now := time.Now()
	job.Status = "failed"
	job.Message = "Server processing failed"
	job.Error = errorMsg
	job.EndTime = &now

	return nil
}
