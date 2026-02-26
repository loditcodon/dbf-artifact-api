package controllers

import (
	"net/http"

	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services/fileops"
	"dbfartifactapi/utils"

	"github.com/gin-gonic/gin"
)

var uploadSrv = fileops.NewUploadService()

// SetUploadService initializes the file upload service instance.
func SetUploadService(srv fileops.UploadService) {
	uploadSrv = srv
}

// ExecuteUpload executes file upload operation via VeloArtifact
// @Summary Execute file upload operation
// @Description Starts background job to upload file from client to server via VeloArtifact. The upload operation uses the source job ID to locate the file on the client, then uploads it to the specified path on the server.
// @Tags Upload
// @Accept json
// @Produce json
// @Param request body models.UploadRequest true "Upload request with connection ID, source job ID, file name, and file path"
// @Success 200 {object} UploadJobStartResponse "Upload job started successfully with job ID"
// @Failure 400 {object} UploadValidationErrorResponse "Invalid request body or validation error"
// @Failure 500 {object} UploadErrorResponse "Internal server error during upload execution"
// @Router /api/queries/upload [post]
func executeUpload(c *gin.Context) {
	var req models.UploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf("Invalid upload request: %v", err)
		utils.ErrorResponse(c, err)
		return
	}

	if err := utils.ValidateStruct(&req); err != nil {
		logger.Errorf("Upload request validation failed: %v", err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Debugf("Executing upload: cnt_id=%d, source_job_id=%s, file_name=%s, file_path=%s",
		req.CntID, req.SourceJobID, req.FileName, req.FilePath)

	jobID, jobMessage, err := uploadSrv.ExecuteUpload(c.Request.Context(), req)
	if err != nil {
		logger.Errorf("Failed to execute upload for cnt_id=%d: %v", req.CntID, err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Infof("Successfully started upload job: %s (job_id=%s)", jobMessage, jobID)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"status":  "success",
		"message": jobMessage,
		"job_id":  jobID,
	})
}

// RegisterUploadRoutes registers HTTP endpoints for file upload operations.
func RegisterUploadRoutes(rg *gin.RouterGroup) {
	upload := rg.Group("/upload")
	{
		upload.POST("", executeUpload)
	}
}
