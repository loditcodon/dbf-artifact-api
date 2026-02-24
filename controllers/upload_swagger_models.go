package controllers

// UploadJobStartResponse represents successful upload job start response
type UploadJobStartResponse struct {
	Status  string `json:"status" example:"success"`
	Message string `json:"message" example:"Upload job started successfully for file: backup_20240101.sql"`
	JobID   string `json:"job_id" example:"F.C39AKJDLAKEU8VH"`
}

// UploadErrorResponse represents error response for upload operation
type UploadErrorResponse struct {
	Error   string `json:"error" example:"failed to execute upload"`
	Message string `json:"message,omitempty" example:"connection management with id=123 not found"`
}

// UploadValidationErrorResponse represents validation error response
type UploadValidationErrorResponse struct {
	Error   string `json:"error" example:"validation failed"`
	Message string `json:"message,omitempty" example:"invalid connection ID: must be greater than 0"`
}
