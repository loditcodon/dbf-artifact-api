package models

// DownloadRequest represents a request to download file from server to agent.
// Server determines compression automatically based on SourcePath type (file vs directory).
type DownloadRequest struct {
	CntID      uint   `json:"cnt_id" binding:"required" example:"456"`
	SourcePath string `json:"source_path" binding:"required" example:"/var/data/config.json"`
	SavePath   string `json:"save_path" example:"/root/"` // Optional - agent uses default path if empty
}

// DownloadAgentPayload represents the JSON payload sent to agent for file download.
// This structure is hex-encoded before sending to dbfsqlexecute filedownload command.
type DownloadAgentPayload struct {
	FileName        string `json:"fileName"`
	SavePath        string `json:"savePath"`
	MD5Hash         string `json:"md5Hash"`
	IsCompressed    bool   `json:"isCompressed"`
	CompressionType string `json:"compressionType"`
}
