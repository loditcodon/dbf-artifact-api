package controllers

// BackupJobStartResponse represents successful backup job start response
type BackupJobStartResponse struct {
	Status  string `json:"status" example:"success"`
	Message string `json:"message" example:"Backup job started successfully with 3 steps"`
	JobID   string `json:"job_id" example:"backup_123"`
}

// BackupErrorResponse represents error response for backup operations
type BackupErrorResponse struct {
	Error   string `json:"error" example:"failed to execute backup"`
	Message string `json:"message,omitempty" example:"connection management with id=123 not found"`
}

// BackupValidationErrorResponse represents validation error response
type BackupValidationErrorResponse struct {
	Error   string `json:"error" example:"validation failed"`
	Message string `json:"message,omitempty" example:"invalid job ID: must be greater than 0"`
}
