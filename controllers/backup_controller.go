package controllers

import (
	"net/http"

	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services/fileops"
	"dbfartifactapi/utils"

	"github.com/gin-gonic/gin"
)

var backupSrv = fileops.NewBackupService()

// SetBackupService initializes the database backup service instance.
func SetBackupService(srv fileops.BackupService) {
	backupSrv = srv
}

// ExecuteBackup executes backup operation via VeloArtifact
// @Summary Execute backup operation
// @Description Starts background jobs to execute backup steps (dump, binlog check, etc.) via VeloArtifact. Each step is executed as a separate background job. For dump type with fileName, supports both OS commands (os_execute) and SQL commands (execute). For other types like check_binlog, all steps are SQL commands.
// @Tags Backup
// @Accept json
// @Produce json
// @Param request body models.BackupRequest true "Backup request with hex-encoded command containing steps"
// @Success 200 {object} BackupJobStartResponse "Background jobs started successfully with job IDs"
// @Failure 400 {object} BackupValidationErrorResponse "Invalid request body or validation error"
// @Failure 500 {object} BackupErrorResponse "Internal server error during backup execution"
// @Router /api/queries/backup [post]
func executeBackup(c *gin.Context) {
	var req models.BackupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf("Invalid backup request: %v", err)
		utils.ErrorResponse(c, err)
		return
	}

	if err := utils.ValidateStruct(&req); err != nil {
		logger.Errorf("Backup request validation failed: %v", err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Debugf("Executing backup: job_id=%d, cnt_id=%d, type=%s, file_name=%s command=%s",
		req.JobID, req.CntID, req.Type, req.FileName, req.Command)

	jobID, jobMessage, err := backupSrv.ExecuteBackup(c.Request.Context(), req)
	if err != nil {
		logger.Errorf("Failed to execute backup for job_id=%d: %v", req.JobID, err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Infof("Successfully started backup job: %s (job_id=%s)", jobMessage, jobID)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"status":  "success",
		"message": jobMessage,
		"job_id":  jobID,
	})
}

// RegisterBackupRoutes registers HTTP endpoints for database backup operations.
func RegisterBackupRoutes(rg *gin.RouterGroup) {
	backup := rg.Group("/backup")
	{
		backup.POST("", executeBackup)
	}
}
