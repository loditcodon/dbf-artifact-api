package controllers

import (
	"net/http"

	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services"
	"dbfartifactapi/utils"

	"github.com/gin-gonic/gin"
)

var downloadSrv = services.NewDownloadService()

// SetDownloadService initializes the file download service instance.
func SetDownloadService(srv services.DownloadService) {
	downloadSrv = srv
}

// DownloadJobStartResponse represents successful download job start response.
type DownloadJobStartResponse struct {
	Status  string `json:"status" example:"success"`
	Message string `json:"message" example:"Download job started successfully for file: config.json"`
	JobID   string `json:"job_id" example:"filedownload_config_1234567890"`
}

// DownloadValidationErrorResponse represents validation error response for download requests.
type DownloadValidationErrorResponse struct {
	Error   string `json:"error" example:"validation error"`
	Message string `json:"message" example:"source_path is required"`
}

// DownloadErrorResponse represents general error response for download operations.
type DownloadErrorResponse struct {
	Error   string `json:"error" example:"internal server error"`
	Message string `json:"message" example:"failed to execute filedownload"`
}

// ExecuteDownload executes file download operation from server to agent via dbfAgentAPI.
// @Summary Execute file download operation
// @Description Starts background job to download file from server to agent. If source_path is a directory, it will be compressed to tar.gz before sending. The agent will verify MD5 hash and extract compressed files automatically.
// @Tags Download
// @Accept json
// @Produce json
// @Param request body models.DownloadRequest true "Download request with connection ID, source path, and save path"
// @Success 200 {object} DownloadJobStartResponse "Download job started successfully with job ID"
// @Failure 400 {object} DownloadValidationErrorResponse "Invalid request body or validation error"
// @Failure 500 {object} DownloadErrorResponse "Internal server error during download execution"
// @Router /api/queries/download [post]
func executeDownload(c *gin.Context) {
	var req models.DownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf("Invalid download request: %v", err)
		utils.ErrorResponse(c, err)
		return
	}

	if err := utils.ValidateStruct(&req); err != nil {
		logger.Errorf("Download request validation failed: %v", err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Debugf("Executing download: cnt_id=%d, source_path=%s, save_path=%s",
		req.CntID, req.SourcePath, req.SavePath)

	jobID, jobMessage, err := downloadSrv.ExecuteDownload(c.Request.Context(), req)
	if err != nil {
		logger.Errorf("Failed to execute download for cnt_id=%d: %v", req.CntID, err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Infof("Successfully started download job: %s (job_id=%s)", jobMessage, jobID)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"status":  "success",
		"message": jobMessage,
		"job_id":  jobID,
	})
}

// RegisterDownloadRoutes registers HTTP endpoints for file download operations.
func RegisterDownloadRoutes(rg *gin.RouterGroup) {
	download := rg.Group("/download")
	{
		download.POST("", executeDownload)
	}
}
