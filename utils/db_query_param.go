package utils

import (
	"bytes"
	"dbfartifactapi/services/dto"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// CreateArtifactJSON converts DBQueryParam to hex-encoded JSON artifact for VeloArtifact execution.
// DEPRECATED: Use CreateAgentCommandJSON for dbfAgentAPI integration.
func CreateArtifactJSON(data *dto.DBQueryParam) (string, error) {
	// chuyển thành json
	paramBytes, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %v", err)
	}

	valueArg := hex.EncodeToString(paramBytes)

	// Set default action if empty
	action := data.Action
	if action == "" {
		action = "execute"
	}

	// Create artifact parameters for new format
	artifactParam := map[string]string{
		"Action": action,
		"Value":  valueArg,
		"Option": data.Option,
	}

	jsonArtifact, err := json.Marshal(artifactParam)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %v", err)
	}

	return string(jsonArtifact), nil
}

// CreateAgentCommandJSON converts DBQueryParam to hex-encoded JSON for dbfAgentAPI execution.
// Returns hex-encoded JSON string ready to be passed to dbfsqlexecute binary.
func CreateAgentCommandJSON(data *dto.DBQueryParam) (string, error) {
	paramBytes, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal DBQueryParam to JSON: %w", err)
	}

	hexEncodedJSON := hex.EncodeToString(paramBytes)
	return hexEncodedJSON, nil
}

// CreateAgentOSExecuteJSON converts DBQueryParam for os_execute to hex-encoded JSON for dbfAgentAPI.
// CommandExec is hex-encoded first, then entire JSON is hex-encoded (double hex encoding).
// This is required for os_execute action to safely pass shell commands through the agent API.
func CreateAgentOSExecuteJSON(data *dto.DBQueryParam) (string, error) {
	if data.CommandExec == "" {
		return "", fmt.Errorf("commandExec is required for os_execute action")
	}

	// Step 1: Hex-encode the command string to prevent injection
	hexCommand := hex.EncodeToString([]byte(data.CommandExec))

	// Step 2: Create a copy with hex-encoded commandExec
	dataCopy := *data
	dataCopy.CommandExec = hexCommand

	// Step 3: Marshal to JSON without HTML escaping
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(dataCopy); err != nil {
		return "", fmt.Errorf("failed to marshal os_execute JSON: %w", err)
	}

	// Step 4: Hex-encode the entire JSON payload
	hexEncodedJSON := hex.EncodeToString(bytes.TrimSpace(buf.Bytes()))
	return hexEncodedJSON, nil
}

// UploadParam represents the upload command parameters for dbfsqlexecute.
type UploadParam struct {
	FileName    string `json:"fileName"`
	FilePath    string `json:"filePath"`
	SourceJobID string `json:"sourceJobId"`
}

// CreateAgentUploadJSON creates hex-encoded JSON for upload operation via dbfAgentAPI.
// Returns hex-encoded JSON string containing fileName, filePath, sourceJobId.
func CreateAgentUploadJSON(fileName, filePath, sourceJobID string) (string, error) {
	if fileName == "" {
		return "", fmt.Errorf("fileName is required for upload action")
	}
	if filePath == "" {
		return "", fmt.Errorf("filePath is required for upload action")
	}
	if sourceJobID == "" {
		return "", fmt.Errorf("sourceJobId is required for upload action")
	}

	uploadParam := UploadParam{
		FileName:    fileName,
		FilePath:    filePath,
		SourceJobID: sourceJobID,
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(uploadParam); err != nil {
		return "", fmt.Errorf("failed to marshal upload JSON: %w", err)
	}

	hexEncodedJSON := hex.EncodeToString(bytes.TrimSpace(buf.Bytes()))
	return hexEncodedJSON, nil
}

// CreateNoHexArtifactJSON creates VeloArtifact JSON artifact without hex encoding the value.
func CreateNoHexArtifactJSON(actionArg string, valueArg string, optionArg string) (string, error) {

	// Create artifact parameters for new format
	artifactParam := map[string]string{
		"Action": actionArg,
		"Value":  valueArg,
		"Option": optionArg,
	}

	jsonArtifact, err := json.Marshal(artifactParam)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %v", err)
	}

	return string(jsonArtifact), nil
}

// CreateOSExecuteArtifactJSON creates artifact JSON for OS command execution
// CommandExec is hex-encoded first, then entire JSON is hex-encoded (double hex encoding)
func CreateOSExecuteArtifactJSON(data *dto.DBQueryParam) (string, error) {
	// CRITICAL: For os_execute, commandExec must be hex-encoded first
	if data.CommandExec == "" {
		return "", fmt.Errorf("commandExec is required for os_execute action")
	}

	// Step 1: Hex-encode the command string
	hexCommand := hex.EncodeToString([]byte(data.CommandExec))

	// Step 2: Create a copy with hex-encoded commandExec
	dataCopy := *data
	dataCopy.CommandExec = hexCommand

	// Step 3: Marshal to JSON without HTML escaping
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(dataCopy); err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %v", err)
	}

	// Remove trailing newline added by Encoder
	paramBytes := bytes.TrimSpace(buf.Bytes())

	// Step 4: Hex-encode the entire JSON payload (second hex encoding)
	valueArg := hex.EncodeToString(paramBytes)

	// Set default action if empty
	action := data.Action
	if action == "" {
		action = "os_execute"
	}

	// Create artifact parameters for new format
	artifactParam := map[string]string{
		"Action": action,
		"Value":  valueArg,
		"Option": data.Option,
	}

	// Marshal artifact param without HTML escaping
	buf.Reset()
	encoder = json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(artifactParam); err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %v", err)
	}

	return string(bytes.TrimSpace(buf.Bytes())), nil
}

// DownloadParam represents the download command parameters for dbfsqlexecute filedownload.
type DownloadParam struct {
	FileName        string `json:"fileName"`
	SavePath        string `json:"savePath"`
	MD5Hash         string `json:"md5Hash"`
	IsCompressed    bool   `json:"isCompressed"`
	CompressionType string `json:"compressionType"`
}

// CreateAgentDownloadJSON creates hex-encoded JSON for filedownload operation via dbfAgentAPI.
// Returns hex-encoded JSON string containing fileName, savePath, md5Hash, isCompressed, compressionType.
// savePath can be empty - agent will use default path if not specified.
func CreateAgentDownloadJSON(fileName, savePath, md5Hash string, isCompressed bool, compressionType string) (string, error) {
	if fileName == "" {
		return "", fmt.Errorf("fileName is required for download action")
	}
	if md5Hash == "" {
		return "", fmt.Errorf("md5Hash is required for download action")
	}

	downloadParam := DownloadParam{
		FileName:        fileName,
		SavePath:        savePath,
		MD5Hash:         md5Hash,
		IsCompressed:    isCompressed,
		CompressionType: compressionType,
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(downloadParam); err != nil {
		return "", fmt.Errorf("failed to marshal download JSON: %w", err)
	}

	hexEncodedJSON := hex.EncodeToString(bytes.TrimSpace(buf.Bytes()))
	return hexEncodedJSON, nil
}
