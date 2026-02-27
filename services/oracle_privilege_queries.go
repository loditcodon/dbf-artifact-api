package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"dbfartifactapi/config"
	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services/privilege/oracle"
)

// buildOraclePrivilegeDataQueries builds queries to fetch privilege data from Oracle system tables.
// Delegates to oracle.BuildOraclePrivilegeDataQueries for query construction.
func (s *dbPolicyService) buildOraclePrivilegeDataQueries(
	actors []models.DBActorMgt,
	connType oracle.OracleConnectionType,
) (map[string][]string, error) {
	// Convert models.DBActorMgt to oracle.ActorInfo
	actorInfos := make([]oracle.ActorInfo, len(actors))
	for i, actor := range actors {
		actorInfos[i] = oracle.ActorInfo{DBUser: actor.DBUser}
	}
	return oracle.BuildOraclePrivilegeDataQueries(actorInfos, connType)
}

// writeOraclePrivilegeQueryFile writes Oracle privilege queries to JSON file for dbfAgentAPI execution.
// File is written to DBFWEB_TEMP_DIR with unique timestamp-based filename.
// Returns filename (not full path) for agent command construction.
func (s *dbPolicyService) writeOraclePrivilegeQueryFile(
	cntMgtID uint,
	queries map[string][]string,
) (string, error) {
	filename := fmt.Sprintf("oracle_privileges_%d_%s.json",
		cntMgtID, time.Now().Format("20060102_150405"))
	filePath := filepath.Join(config.Cfg.DBFWebTempDir, filename)

	// JSON encoding with readable formatting
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(queries); err != nil {
		logger.Errorf("Marshal Oracle privilege queries error: %v", err)
		return "", fmt.Errorf("marshal oracle privilege queries error: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Errorf("Create dir error for Oracle privileges: %v", err)
		return "", fmt.Errorf("create dir error: %w", err)
	}

	if err := os.WriteFile(filePath, buf.Bytes(), 0644); err != nil {
		logger.Errorf("Write Oracle privilege queries to file error: %v", err)
		return "", fmt.Errorf("write oracle privilege queries error: %w", err)
	}

	logger.Infof("Oracle privilege queries written to %s (%d bytes)", filePath, buf.Len())
	return filename, nil
}

// buildOracleObjectQueries builds queries to fetch database objects for policy matching.
// Delegates to oracle.BuildOracleObjectQueries for query construction.
func (s *dbPolicyService) buildOracleObjectQueries(
	databases []models.DBMgt,
	connType oracle.OracleConnectionType,
) (map[string][]string, error) {
	// Convert models.DBMgt to oracle.DatabaseInfo
	dbInfos := make([]oracle.DatabaseInfo, len(databases))
	for i, db := range databases {
		dbInfos[i] = oracle.DatabaseInfo{DbName: db.DbName}
	}
	return oracle.BuildOracleObjectQueries(dbInfos, connType)
}
