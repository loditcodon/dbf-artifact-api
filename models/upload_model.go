package models

// UploadRequest represents upload request from client to server via VeloArtifact
type UploadRequest struct {
	CntID       uint   `json:"cnt_id" binding:"required" example:"456"`
	SourceJobID string `json:"source_job_id" binding:"required" example:"F.C39AKJDLAKEU8VH"`
	FileName    string `json:"file_name" binding:"required" example:"backup_20240101.sql"`
	FilePath    string `json:"file_path" binding:"required" example:"/tmp/backups/backup_20240101.sql"`
}
