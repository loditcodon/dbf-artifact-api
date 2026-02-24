package controllers

import (
	"fmt"
	"net/http"
	"strconv"

	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services"

	"github.com/gin-gonic/gin"
)

// JobStatusController handles job status API endpoints
type JobStatusController struct {
	jobMonitor *services.JobMonitorService
}

// NewJobStatusController creates a new JobStatusController
func NewJobStatusController() *JobStatusController {
	return &JobStatusController{
		jobMonitor: services.GetJobMonitorService(),
	}
}

// JobStatusResponse represents the API response for job status
type JobStatusResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// PaginatedJobStatusResponse represents paginated job status response
type PaginatedJobStatusResponse struct {
	Success    bool                `json:"success"`
	Message    string              `json:"message"`
	Data       interface{}         `json:"data"`
	Pagination *PaginationMetadata `json:"pagination,omitempty"`
}

// PaginationMetadata contains pagination information
type PaginationMetadata struct {
	Total      int `json:"total"`
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalPages int `json:"total_pages"`
}

// GetJobStatus retrieves status of a specific job
// @Summary Get job status by ID
// @Description Get the current status of a background job
// @Tags job-status
// @Accept json
// @Produce json
// @Param job_id path string true "Job ID"
// @Success 200 {object} JobStatusSingleResponse
// @Failure 400 {object} JobStatusErrorResponse
// @Failure 404 {object} JobNotFoundErrorResponse
// @Router /api/jobs/{job_id}/status [get]
func (jsc *JobStatusController) GetJobStatus(c *gin.Context) {
	jobID := c.Param("job_id")
	if jobID == "" {
		logger.Warnf("Empty job_id provided for job status check")
		c.JSON(http.StatusBadRequest, JobStatusResponse{
			Success: false,
			Message: "Job ID is required",
		})
		return
	}

	job, exists := jsc.jobMonitor.GetJob(jobID)
	if !exists {
		logger.Warnf("Job not found: %s", jobID)
		c.JSON(http.StatusNotFound, JobStatusResponse{
			Success: false,
			Message: "Job not found",
		})
		return
	}

	logger.Debugf("Retrieved job status for %s: %s", jobID, job.Status)
	c.JSON(http.StatusOK, JobStatusResponse{
		Success: true,
		Message: "Job status retrieved successfully",
		Data:    job,
	})
}

// GetAllJobs retrieves status of all jobs with optional pagination
// @Summary Get all jobs status
// @Description Get the current status of all background jobs. Supports optional pagination via query parameters 'page' and 'page_size'
// @Tags job-status
// @Accept json
// @Produce json
// @Param page query int false "Page number (1-indexed, optional)"
// @Param page_size query int false "Number of items per page (optional, default: 10)"
// @Success 200 {object} JobStatusListResponse
// @Router /api/jobs/status [get]
func (jsc *JobStatusController) GetAllJobs(c *gin.Context) {
	pageStr := c.Query("page")
	pageSizeStr := c.Query("page_size")

	// Backward compatibility - existing clients without pagination params get full dataset
	if pageStr == "" && pageSizeStr == "" {
		jobs := jsc.jobMonitor.GetAllJobs()
		logger.Debugf("Retrieved status for %d jobs (non-paginated)", len(jobs))
		c.JSON(http.StatusOK, JobStatusResponse{
			Success: true,
			Message: "All jobs status retrieved successfully",
			Data:    jobs,
		})
		return
	}

	// Use defaults to prevent invalid pagination that would return empty results
	page := 1
	pageSize := 10

	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		} else {
			logger.Warnf("Invalid page parameter: %s, using default: 1", pageStr)
		}
	}

	if pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			pageSize = ps
		} else {
			logger.Warnf("Invalid page_size parameter: %s, using default: 10", pageSizeStr)
		}
	}

	result := jsc.jobMonitor.GetAllJobsPaginated(page, pageSize)

	logger.Debugf("Retrieved %d jobs (page %d of %d, page_size=%d, total=%d)",
		len(result.Jobs), result.Page, result.TotalPages, result.PageSize, result.Total)

	c.JSON(http.StatusOK, PaginatedJobStatusResponse{
		Success: true,
		Message: "Jobs status retrieved successfully",
		Data:    result.Jobs,
		Pagination: &PaginationMetadata{
			Total:      result.Total,
			Page:       result.Page,
			PageSize:   result.PageSize,
			TotalPages: result.TotalPages,
		},
	})
}

// GetJobsByDBMgt retrieves jobs for a specific database management ID
// @Summary Get jobs by database management ID
// @Description Get all jobs associated with a specific database management ID
// @Tags job-status
// @Accept json
// @Produce json
// @Param dbmgt_id path int true "Database Management ID"
// @Success 200 {object} JobStatusListResponse
// @Failure 400 {object} JobStatusErrorResponse
// @Router /api/jobs/dbmgt/{dbmgt_id}/status [get]
func (jsc *JobStatusController) GetJobsByDBMgt(c *gin.Context) {
	dbmgtIDStr := c.Param("dbmgt_id")
	if dbmgtIDStr == "" {
		logger.Warnf("Empty dbmgt_id provided for jobs lookup")
		c.JSON(http.StatusBadRequest, JobStatusResponse{
			Success: false,
			Message: "Database Management ID is required",
		})
		return
	}

	dbmgtID, err := strconv.ParseUint(dbmgtIDStr, 10, 32)
	if err != nil {
		logger.Warnf("Invalid dbmgt_id format: %s", dbmgtIDStr)
		c.JSON(http.StatusBadRequest, JobStatusResponse{
			Success: false,
			Message: "Invalid Database Management ID format",
		})
		return
	}

	jobs := jsc.jobMonitor.GetJobsByDBMgt(uint(dbmgtID))

	logger.Debugf("Retrieved %d jobs for dbmgt_id %d", len(jobs), dbmgtID)
	c.JSON(http.StatusOK, JobStatusResponse{
		Success: true,
		Message: "Jobs status retrieved successfully",
		Data:    jobs,
	})
}

// JobNotificationRequest represents the request body for job completion notifications
type JobNotificationRequest struct {
	FileName string `json:"fileName" binding:"required"`
	Md5Hash  string `json:"md5Hash" binding:"required"`
	Success  bool   `json:"success"`
}

// NotifyJobCompletion allows external services to notify about job completion
// @Summary Notify job completion
// @Description Allow external services to report job completion with file data
// @Tags job-status
// @Accept json
// @Produce json
// @Param job_id path string true "Job ID"
// @Param notification body JobNotificationRequest true "Job completion notification data"
// @Success 200 {object} JobStatusResponse
// @Failure 400 {object} JobStatusErrorResponse
// @Failure 404 {object} JobNotFoundErrorResponse
// @Router /api/jobs/{job_id}/notify [post]
func (jsc *JobStatusController) NotifyJobCompletion(c *gin.Context) {
	jobID := c.Param("job_id")
	if jobID == "" {
		logger.Warnf("Empty job_id provided for job completion notification")
		c.JSON(http.StatusBadRequest, JobStatusResponse{
			Success: false,
			Message: "Job ID is required",
		})
		return
	}

	var notificationData JobNotificationRequest
	if err := c.ShouldBindJSON(&notificationData); err != nil {
		logger.Warnf("Invalid notification data for job %s: %v", jobID, err)
		c.JSON(http.StatusBadRequest, JobStatusResponse{
			Success: false,
			Message: "Invalid notification data",
		})
		return
	}

	// Check if job exists
	job, exists := jsc.jobMonitor.GetJob(jobID)
	if !exists {
		logger.Warnf("Job not found for notification: %s", jobID)
		c.JSON(http.StatusNotFound, JobStatusResponse{
			Success: false,
			Message: "Job not found",
		})
		return
	}

	logger.Infof("Processing job completion notification: job_id=%s, fileName=%s, md5=%s, success=%v",
		jobID, notificationData.FileName, notificationData.Md5Hash, notificationData.Success)

	// Process the notification
	err := jsc.jobMonitor.ProcessJobNotification(jobID, notificationData.FileName, notificationData.Md5Hash, notificationData.Success)
	if err != nil {
		logger.Errorf("Failed to process job notification for %s: %v", jobID, err)
		c.JSON(http.StatusInternalServerError, JobStatusResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to process notification: %v", err),
		})
		return
	}

	logger.Infof("Job completion notification processed successfully for job %s", jobID)
	c.JSON(http.StatusOK, JobStatusResponse{
		Success: true,
		Message: "Job completion notification processed successfully",
		Data:    job,
	})
}

// DeleteJob removes a job from monitoring (for completed/failed jobs)
// @Summary Delete job from monitoring
// @Description Remove a job from the monitoring system (typically for completed or failed jobs)
// @Tags job-status
// @Accept json
// @Produce json
// @Param job_id path string true "Job ID"
// @Success 200 {object} JobDeleteResponse
// @Failure 400 {object} JobStatusErrorResponse
// @Failure 404 {object} JobNotFoundErrorResponse
// @Router /api/jobs/{job_id} [delete]
func (jsc *JobStatusController) DeleteJob(c *gin.Context) {
	jobID := c.Param("job_id")
	if jobID == "" {
		logger.Warnf("Empty job_id provided for job deletion")
		c.JSON(http.StatusBadRequest, JobStatusResponse{
			Success: false,
			Message: "Job ID is required",
		})
		return
	}

	// Check if job exists first
	_, exists := jsc.jobMonitor.GetJob(jobID)
	if !exists {
		logger.Warnf("Attempted to delete non-existent job: %s", jobID)
		c.JSON(http.StatusNotFound, JobStatusResponse{
			Success: false,
			Message: "Job not found",
		})
		return
	}

	jsc.jobMonitor.RemoveJob(jobID)
	logger.Infof("Job %s removed from monitoring", jobID)

	c.JSON(http.StatusOK, JobStatusResponse{
		Success: true,
		Message: "Job removed from monitoring successfully",
	})
}

// RegisterJobStatusRoutes registers all job status routes
func RegisterJobStatusRoutes(router *gin.RouterGroup) {
	controller := NewJobStatusController()

	jobRoutes := router.Group("/jobs")
	{
		jobRoutes.GET("/:job_id/status", controller.GetJobStatus)
		jobRoutes.GET("/status", controller.GetAllJobs)
		jobRoutes.GET("/dbmgt/:dbmgt_id/status", controller.GetJobsByDBMgt)
		jobRoutes.POST("/:job_id/notify", controller.NotifyJobCompletion)
		jobRoutes.DELETE("/:job_id", controller.DeleteJob)
	}
}
