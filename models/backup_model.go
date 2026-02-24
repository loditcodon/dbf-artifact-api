package models

import "time"

// BackupRequest represents the backup execution request
type BackupRequest struct {
	JobID    uint   `json:"job_id" binding:"required" example:"123"`
	CntID    uint   `json:"cnt_id" binding:"required" example:"456"`
	Command  string `json:"command" binding:"required" example:"7b2273746570733a205b7b226f72646572223a20312c2022636f6d6d616e64223a20226d7973716c64756d70202e2e2e227d5d7d"`
	Type     string `json:"type" binding:"required" example:"dump" enums:"dump,check_binlog"`
	FileName string `json:"file_name" example:"backup_20240101.sql"`
}

// BackupStep represents a single step in backup command
type BackupStep struct {
	Order   int    `json:"order" example:"1"`
	Command string `json:"command" example:"mysqldump -u root -p dbname > backup.sql"`
	Type    string `json:"type" example:"os" enums:"os,sql"`
}

// BackupCommand represents the decoded backup command structure
type BackupCommand struct {
	Steps []BackupStep `json:"steps"`
}

// BackupJob represents background job information for backup operations
type BackupJob struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	JobID       uint       `json:"job_id"`
	CntID       uint       `json:"cnt_id"`
	VeloJobID   string     `json:"velo_job_id"`
	Type        string     `json:"type"`
	FileName    string     `json:"file_name"`
	Command     string     `json:"command"`
	Status      string     `json:"status"`
	Result      string     `gorm:"type:text" json:"result"`
	Error       string     `gorm:"type:text" json:"error"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// TableName returns the database table name for BackupJob model.
func (BackupJob) TableName() string {
	return "backup_jobs"
}
