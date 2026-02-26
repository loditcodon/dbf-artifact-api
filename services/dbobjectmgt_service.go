package services

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"dbfartifactapi/bootstrap"
	"dbfartifactapi/config"
	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services/agent"
	"dbfartifactapi/services/dto"
	"dbfartifactapi/utils"

	"gorm.io/gorm"
)

// objectInput represents processed object template data similar to policyInput
type objectInput struct {
	dbobject models.DBObject
	objectId int
	finalSQL string // Store the processed SQL to avoid re-processing
}

// CombinedObjectJobContext contains context data for combined object job completion
type CombinedObjectJobContext struct {
	CntMgtID   uint                            `json:"cnt_mgt_id"`
	DbMgts     []models.DBMgt                  `json:"db_mgts"`
	CMT        *models.CntMgt                  `json:"cmt"`
	EndpointID uint                            `json:"endpoint_id"`
	DbQueries  map[uint]map[string]objectInput `json:"db_queries"` // dbmgt_id -> queries
}

// DBObjectMgtService provides business logic for database object management operations.
type DBObjectMgtService interface {
	// Get(ctx context.Context, params models.DBObjectMgt) ([]models.DBObjectMgt, error)
	GetByDbMgtId(ctx context.Context, id uint) (string, error)
	GetByCntMgtId(ctx context.Context, id uint) (string, error)
	Create(ctx context.Context, data models.DBObjectMgt) (*models.DBObjectMgt, error)
	Update(ctx context.Context, id uint, data models.DBObjectMgt) (*models.DBObjectMgt, error)
	Delete(ctx context.Context, id uint) error
}

type dbObjectMgtService struct {
	baseRepo        repository.BaseRepository
	dbObjectMgtRepo repository.DBObjectMgtRepository
	dbMgtRepo       repository.DBMgtRepository
	cntMgtRepo      repository.CntMgtRepository
	endpointRepo    repository.EndpointRepository
	dbobjectAllMap  map[uint]models.DBObject
}

// NewDBObjectMgtService creates a new database object management service instance.
func NewDBObjectMgtService() DBObjectMgtService {
	return &dbObjectMgtService{
		baseRepo:        repository.NewBaseRepository(),
		dbObjectMgtRepo: repository.NewDBObjectMgtRepository(),
		dbMgtRepo:       repository.NewDBMgtRepository(),
		cntMgtRepo:      repository.NewCntMgtRepository(),
		endpointRepo:    repository.NewEndpointRepository(),
		dbobjectAllMap:  bootstrap.DBObjectAllMap,
	}
}

// GetByDbMgtId discovers and generates database objects for a specific database instance.
// Processes hex-encoded SQL templates with variable substitution, executes via VeloArtifact.
// Returns job ID for tracking background processing.
func (s *dbObjectMgtService) GetByDbMgtId(ctx context.Context, id uint) (string, error) {
	if id == 0 {
		return "", fmt.Errorf("invalid database management ID: must be greater than 0")
	}

	tx := s.baseRepo.Begin()
	logger.Debugf("Getting database objects for dbmgt ID: %d", id)

	// Retrieve database management configuration
	dbmgt, err := s.dbMgtRepo.GetByID(tx, id)
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to get dbmgt with id=%d: %w", id, err)
	}
	logger.Infof("Found database management record: id=%d", id)

	// Retrieve connection management configuration
	cmt, err := s.cntMgtRepo.GetCntMgtByID(tx, dbmgt.CntID)
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to get cntmgt with id=%d: %w", dbmgt.CntID, err)
	}
	logger.Infof("Found connection management: id=%d, type=%s, username=%s", cmt.ID, cmt.CntType, cmt.Username)

	// Retrieve endpoint configuration for VeloArtifact execution
	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to get endpoint with id=%d: %w", cmt.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	// Build SQL queries similar to GetByDBMgt pattern
	sqlFinalMap, err := s.buildObjectQueries(tx, id, dbmgt, cmt)
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to build object queries: %w", err)
	}

	// Group queries by object context for efficient batch processing
	listQuery := s.buildObjectQueryGroups(sqlFinalMap)

	// Write queries to file for batch processing
	filename, err := s.writeObjectQueryFile(id, listQuery)
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to write query file: %w", err)
	}

	// Start background job using VeloArtifact pattern
	queryParamBuilder := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cmt.CntType)).
		SetHost(cmt.IP).
		SetPort(cmt.Port).
		SetUser(cmt.Username).
		SetPassword(cmt.Password)

	// Oracle requires ServiceName as the database identifier for CDB/PDB routing
	if strings.ToLower(cmt.CntType) == "oracle" {
		queryParamBuilder.SetDatabase(cmt.ServiceName)
	} else {
		queryParamBuilder.SetDatabase(dbmgt.DbName)
	}

	queryParam := queryParamBuilder.Build()
	queryParam.Query = filename // use filename only, not full path
	queryParam.Action = "download"
	queryParam.Option = "--background"

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to create agent command JSON: %w", err)
	}

	// Job response structure
	type JobResponse struct {
		JobID          string `json:"job_id"`
		Message        string `json:"message"`
		MonitorCommand string `json:"monitor_command"`
		PID            int    `json:"pid"`
		ResultsCommand string `json:"results_command"`
		Success        bool   `json:"success"`
	}

	// Start background job with --background option
	stdout, err := agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "download", hexJSON, "--background", true)
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to start agent API job: %w", err)
	}

	// Parse job response
	var jobResp JobResponse
	if err := json.Unmarshal([]byte(stdout), &jobResp); err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to parse job response: %w", err)
	}

	if !jobResp.Success {
		tx.Rollback()
		return "", fmt.Errorf("job failed to start: %s", jobResp.Message)
	}

	logger.Infof("Background job started successfully: %s", jobResp.JobID)

	// Prepare context data for job completion callback
	objectContext := &ObjectJobContext{
		DBMgtID:       id,
		ObjectQueries: listQuery,
		DBMgt:         dbmgt,
		CMT:           cmt,
		EndpointID:    ep.ID,
	}

	contextData := map[string]interface{}{
		"object_context": objectContext,
	}

	// Add job to monitoring system with completion callback for atomic object creation
	jobMonitor := GetJobMonitorService()
	completionCallback := CreateObjectCompletionHandler()
	jobMonitor.AddJobWithCallback(jobResp.JobID, id, ep.ClientID, ep.OsType, completionCallback, contextData)

	// Frontend should use /api/jobs/{job_id}/status for progress tracking
	logger.Infof("Job %s added to monitoring system with completion callback", jobResp.JobID)

	// ATOMIC REQUIREMENT: Don't commit until job completion handler processes results
	// This ensures data consistency - either all objects are created or none
	// Rollback preparation transaction, actual objects created atomically on job completion
	tx.Rollback()

	logger.Infof("Object generation job %s started successfully for dbmgt_id=%d", jobResp.JobID, id)
	logger.Infof("Preparation transaction rolled back - atomic object creation on job completion")
	return fmt.Sprintf("Background job started: %s. Use job monitoring to track completion.", jobResp.JobID), nil
}

// buildObjectQueries builds SQL queries with unique keys to prevent collision.
// Uses ObjectType:ID pattern for simple objects, ObjectType:ID_DependentIndex:N for dependent objects.
func (s *dbObjectMgtService) buildObjectQueries(tx *gorm.DB, dbmgtID uint, dbmgt *models.DBMgt, cmt *models.CntMgt) (map[string]objectInput, error) {
	sqlFinalMap := make(map[string]objectInput)

	// Database-type-specific ObjectId exclusions
	var exceptMap map[uint]struct{}
	cntTypeLower := strings.ToLower(cmt.CntType)

	switch cntTypeLower {
	case "mysql":
		// ObjectId 1 = Database objects (handled by dbmgt table)
		// ObjectId 12 = Actor objects (handled by dbactormgt table)
		// ObjectId 15 = Schema objects (actually same as dbmgt for MySQL)
		exceptListObject := []uint{1, 12, 15}
		exceptMap = make(map[uint]struct{})
		for _, v := range exceptListObject {
			exceptMap[v] = struct{}{}
		}
		logger.Debugf("MySQL database type detected - excluding ObjectId 1, 12, and 15 from object queries")
	case "oracle":
		// Oracle objects not yet supported - pending implementation
		exceptListObject := []uint{1025, 1038, 1041, 1042, 1047, 1048, 1052, 1055, 1056, 1057}
		exceptMap = make(map[uint]struct{})
		for _, v := range exceptListObject {
			exceptMap[v] = struct{}{}
		}
		logger.Debugf("Oracle database type detected - excluding ObjectIds %v from object queries", exceptListObject)
	default:
		exceptMap = make(map[uint]struct{})
		logger.Debugf("Non-MySQL database type (%s) - using standard object management", cmt.CntType)
	}

	// Determine Oracle CDB/PDB scope for variable substitution
	var oracleScope string
	if cntTypeLower == "oracle" {
		if cmt.ParentConnectionID == nil {
			oracleScope = "cdb"
		} else {
			oracleScope = "pdb"
		}
		logger.Debugf("Oracle connection scope: %s (cntmgt_id=%d, service_name=%s)", oracleScope, cmt.ID, cmt.ServiceName)
	}

	logger.Debugf("Building object queries for dbmgt ID: %d", dbmgtID)
	processedCount := 0

	for _, dbObject := range s.dbobjectAllMap {
		processedCount++
		logger.Debugf("Processing object %d: ID=%d, SqlInputType=%d", processedCount, dbObject.ID, dbObject.SqlInputType)

		// Skip objects that don't match the current database type
		if dbObject.DBType != "" && strings.ToLower(dbObject.DBType) != cntTypeLower {
			continue
		}

		// Skip excluded ObjectIds based on database type
		if _, exists := exceptMap[dbObject.ID]; exists {
			logger.Debugf("Skipping excluded ObjectId %d for database type %s", dbObject.ID, cmt.CntType)
			continue
		}

		// Skip objects with empty SQL
		if dbObject.SQLGet == "" {
			logger.Warnf("Dbobject.sql_get is empty for objecttype=%d", dbObject.ID)
			continue
		}

		// Decode hex-encoded SQL template for security compliance
		sqlBytes, err := hex.DecodeString(dbObject.SQLGet)
		if err != nil {
			logger.Warnf("Failed to decode hex SQL template for objecttype=%d, skipping: %v", dbObject.ID, err)
			continue
		}

		rawSQL := string(sqlBytes)
		logger.Debugf("Raw SQL for objecttype=%d: %s", dbObject.ID, rawSQL)

		// Replace common placeholders
		finalSQL := strings.ReplaceAll(rawSQL, "${dbmgt.dbname}", dbmgt.DbName)

		// Oracle-specific placeholder substitution for CDB/PDB queries
		if cntTypeLower == "oracle" {
			finalSQL = strings.ReplaceAll(finalSQL, "${cntmgt.servicename}", cmt.ServiceName)
			finalSQL = strings.ReplaceAll(finalSQL, "${scope}", oracleScope)
			// Oracle programmatic interfaces reject trailing semicolons (SQL*Plus delimiter only)
			finalSQL = strings.TrimRight(finalSQL, "; \t\n\r")
		}

		// Handle objects that depend on other object types
		if dbObject.SqlInputType != 0 {
			logger.Debugf("Object %d requires input from object type %d", dbObject.ID, dbObject.SqlInputType)
			dbObjectMgts, err := s.dbObjectMgtRepo.GetByObjectIdAndDbMgt(tx, dbObject.SqlInputType, dbmgtID)
			if err != nil {
				logger.Errorf("Failed to get dbobjectmgts for object_id=%d and dbmgt=%d: %v", dbObject.SqlInputType, dbmgtID, err)
				continue
			}
			if len(dbObjectMgts) == 0 {
				logger.Warnf("No dbobjectmgts found for object_id=%d, skipping objecttype=%d", dbObject.SqlInputType, dbObject.ID)
				continue
			}

			logger.Debugf("Found %d input objects for objecttype=%d", len(dbObjectMgts), dbObject.ID)
			for i, dbObjectMgt := range dbObjectMgts {
				objectSQL := strings.ReplaceAll(finalSQL, "${dbobjectmgt.objectname_byinputtype}", dbObjectMgt.ObjectName)
				// Use unique key to prevent collision: "ObjectType:ID_DependentIndex:N"
				uniqueKey := fmt.Sprintf("ObjectType:%d_DependentIndex:%d", dbObject.ID, i+1)
				sqlFinalMap[uniqueKey] = objectInput{
					dbobject: dbObject,
					objectId: utils.MustUintToInt(dbObject.ID),
					finalSQL: objectSQL, // Store processed SQL
				}
				logger.Debugf("Added SQL for objecttype=%d with table=%s, key=%s", dbObject.ID, dbObjectMgt.ObjectName, uniqueKey)
			}
		} else {
			// Simple objects without dependencies - use ObjectType:ID as unique key
			uniqueKey := fmt.Sprintf("ObjectType:%d", dbObject.ID)
			sqlFinalMap[uniqueKey] = objectInput{
				dbobject: dbObject,
				objectId: utils.MustUintToInt(dbObject.ID),
				finalSQL: finalSQL, // Store processed SQL
			}
			logger.Debugf("Added simple SQL for objecttype=%d, key=%s", dbObject.ID, uniqueKey)
		}
	}

	logger.Debugf("Built queries for %d object SQL statements", len(sqlFinalMap))
	return sqlFinalMap, nil
}

// buildObjectQueryGroups creates individual keys for each query for easier result parsing
func (s *dbObjectMgtService) buildObjectQueryGroups(sqlFinalMap map[string]objectInput) map[string][]string {
	// Use the unique keys from sqlFinalMap directly to prevent SQL collision
	// This ensures each object type gets its own key even if SQL is identical
	listQuery := make(map[string][]string)

	for uniqueKey, objectData := range sqlFinalMap {
		// Use the pre-processed finalSQL from objectInput
		listQuery[uniqueKey] = []string{objectData.finalSQL} // Use unique key to prevent collision
		logger.Debugf("Added query with unique key: %s, SQL: %s", uniqueKey, objectData.finalSQL)
	}

	return listQuery
}

// writeObjectQueryFile writes query data to JSON file for VeloArtifact processing.
// File-based execution reduces command complexity and enables audit trail.
func (s *dbObjectMgtService) writeObjectQueryFile(dbmgtID uint, listQuery map[string][]string) (string, error) {
	filename := fmt.Sprintf("getdbobject_%d_%s.json", dbmgtID, time.Now().Format("20060102_150405"))
	filePath := fmt.Sprintf("%s/%s", config.Cfg.DBFWebTempDir, filename)

	// JSON formatting ensures readability for debugging and audit purposes
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	// Only queries are written to file, connection params sent separately for security
	if err := encoder.Encode(listQuery); err != nil {
		logger.Errorf("Marshal listQuery error: %v", err)
		return "", fmt.Errorf("marshal listQuery error: %v", err)
	}

	// Ensure temp directory exists for VeloArtifact file operations
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Errorf("Create dir error: %v", err)
		return "", fmt.Errorf("create dir error: %v", err)
	}

	if err := os.WriteFile(filePath, buf.Bytes(), 0644); err != nil {
		logger.Errorf("Write listQuery to file error: %v", err)
		return "", fmt.Errorf("write listQuery to file error: %v", err)
	}

	logger.Infof("Object ListQuery written to %s", filePath)
	return filename, nil
}

// GetByCntMgtId retrieves database objects for all databases under a connection.
// Uses optimized version that combines all queries into single VeloArtifact job.
func (s *dbObjectMgtService) GetByCntMgtId(ctx context.Context, id uint) (string, error) {
	return s.GetByCntMgtIdOptimized(ctx, id)
}

// GetByCntMgtIdOptimized combines all database queries into single VeloArtifact job.
// Reduces VeloArtifact overhead compared to calling GetByDbMgtId multiple times.
func (s *dbObjectMgtService) GetByCntMgtIdOptimized(ctx context.Context, id uint) (string, error) {
	if id == 0 {
		return "", fmt.Errorf("invalid connection management ID: must be greater than 0")
	}

	tx := s.baseRepo.Begin()
	logger.Debugf("Getting database objects for all databases under cntmgt ID: %d", id)

	// Retrieve all databases under this connection
	dbmgts, err := s.dbMgtRepo.GetByCntMgtId(tx, id)
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to get dbmgt with cntid=%d: %w", id, err)
	}

	if len(dbmgts) == 0 {
		tx.Rollback()
		return "", fmt.Errorf("no databases found for cntmgt_id=%d", id)
	}

	logger.Infof("Found %d databases for cntmgt_id=%d", len(dbmgts), id)

	// Retrieve connection configuration (same for all databases)
	cmt, err := s.cntMgtRepo.GetCntMgtByID(tx, id)
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to get cntmgt with id=%d: %w", id, err)
	}

	// Retrieve endpoint configuration for VeloArtifact execution
	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to get endpoint with id=%d: %w", cmt.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	// Build combined queries for all databases under this connection
	combinedSqlFinalMap := make(map[string]objectInput)
	combinedContext := &CombinedObjectJobContext{
		CntMgtID:   id,
		DbMgts:     dbmgts,
		CMT:        cmt,
		EndpointID: ep.ID,
		DbQueries:  make(map[uint]map[string]objectInput), // dbmgt_id -> queries
	}

	for _, dbmgt := range dbmgts {
		logger.Debugf("Building queries for database: id=%d, name=%s", dbmgt.ID, dbmgt.DbName)

		// Build object queries for this specific database
		dbSqlFinalMap, err := s.buildObjectQueries(tx, dbmgt.ID, &dbmgt, cmt)
		if err != nil {
			logger.Errorf("Failed to build queries for dbmgt_id=%d: %v", dbmgt.ID, err)
			continue
		}

		// Store queries for result processing after job completion
		combinedContext.DbQueries[dbmgt.ID] = dbSqlFinalMap

		// Prefix queries with database ID to distinguish queries from different databases
		for uniqueKey, objectData := range dbSqlFinalMap {
			prefixedKey := fmt.Sprintf("DbMgt:%d_%s", dbmgt.ID, uniqueKey)
			combinedSqlFinalMap[prefixedKey] = objectData
		}
	}

	if len(combinedSqlFinalMap) == 0 {
		tx.Rollback()
		return "", fmt.Errorf("no queries built for any database under cntmgt_id=%d", id)
	}

	// Create query groups for VeloArtifact processing
	listQuery := s.buildCombinedObjectQueryGroups(combinedSqlFinalMap)

	// Write all queries to single file for batch execution
	filename, err := s.writeCombinedObjectQueryFile(id, listQuery)
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to write combined query file: %w", err)
	}

	// Prepare VeloArtifact execution parameters for combined job
	queryParamBuilder := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cmt.CntType)).
		SetHost(cmt.IP).
		SetPort(cmt.Port).
		SetUser(cmt.Username).
		SetPassword(cmt.Password)

	// Oracle requires ServiceName as the database identifier for CDB/PDB routing
	if strings.ToLower(cmt.CntType) == "oracle" {
		queryParamBuilder.SetDatabase(cmt.ServiceName)
	}

	queryParam := queryParamBuilder.Build()
	queryParam.Query = filename
	queryParam.Action = "download"
	queryParam.Option = "--background"

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to create agent command JSON: %w", err)
	}

	// Job response structure
	type JobResponse struct {
		JobID          string `json:"job_id"`
		Message        string `json:"message"`
		MonitorCommand string `json:"monitor_command"`
		PID            int    `json:"pid"`
		ResultsCommand string `json:"results_command"`
		Success        bool   `json:"success"`
	}

	// Start background job with --background option
	stdout, err := agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "download", hexJSON, "--background", true)
	if err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to start agent API job: %w", err)
	}

	// Parse job response
	var jobResp JobResponse
	if err := json.Unmarshal([]byte(stdout), &jobResp); err != nil {
		tx.Rollback()
		return "", fmt.Errorf("failed to parse job response: %w", err)
	}

	if !jobResp.Success {
		tx.Rollback()
		return "", fmt.Errorf("job failed to start: %s", jobResp.Message)
	}

	logger.Infof("Combined background job started successfully: %s", jobResp.JobID)

	// Register job with monitoring system for completion tracking
	contextData := map[string]interface{}{
		"combined_object_context": combinedContext,
	}

	jobMonitor := GetJobMonitorService()
	completionCallback := CreateCombinedObjectCompletionHandler()
	jobMonitor.AddJobWithCallback(jobResp.JobID, id, ep.ClientID, ep.OsType, completionCallback, contextData)

	logger.Infof("Combined job %s added to monitoring system", jobResp.JobID)

	// Rollback preparation transaction - actual objects created atomically on job completion
	tx.Rollback()

	logger.Infof("Combined object generation job %s started for cntmgt_id=%d (%d databases)",
		jobResp.JobID, id, len(dbmgts))

	// Oracle CDB: trigger separate jobs for each child PDB in background
	// Each PDB requires its own job because it uses a different ServiceName for routing
	isOracleCDB := strings.ToLower(cmt.CntType) == "oracle" && cmt.ParentConnectionID == nil
	if isOracleCDB {
		go func(cdbID uint) {
			pdbConnections, err := s.cntMgtRepo.GetByParentConnectionID(s.baseRepo.Begin(), cdbID)
			if err != nil {
				logger.Errorf("Failed to get child PDB connections for CDB cntmgt_id=%d: %v", cdbID, err)
				return
			}

			logger.Infof("Oracle CDB (cntmgt_id=%d): triggering background jobs for %d child PDB connections", cdbID, len(pdbConnections))

			for _, pdb := range pdbConnections {
				_, err := s.GetByCntMgtIdOptimized(ctx, pdb.ID)
				if err != nil {
					logger.Errorf("Failed to start object job for PDB %s (cntmgt_id=%d): %v", pdb.CntName, pdb.ID, err)
					continue
				}
				logger.Infof("Successfully triggered object job for PDB %s (cntmgt_id=%d)", pdb.CntName, pdb.ID)
			}

			logger.Infof("All PDB jobs triggered for CDB cntmgt_id=%d", cdbID)
		}(id)
	}

	return fmt.Sprintf("Combined background job started: %s. Processing %d databases in single job.",
		jobResp.JobID, len(dbmgts)), nil
}

// mergeOutput merges duplicate object type counts from multiple query results.
// Used to consolidate object counts when same object type appears in multiple queries.
func mergeOutput(output string) (string, error) {
	countMap := make(map[string]int)
	regx := regexp.MustCompile(`Object (.+?) has (\d+) records`)

	parts := strings.Split(output, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		matches := regx.FindStringSubmatch(part)
		if len(matches) == 3 {
			objectType := matches[1]
			countStr := matches[2]
			count, err := strconv.Atoi(countStr)
			if err != nil {
				return "", err
			}
			countMap[objectType] += count
		}
	}
	result := []string{}
	for objectType, count := range countMap {
		result = append(result, fmt.Sprintf("Object %s has %d records", objectType, count))
	}
	return strings.Join(result, ","), nil
}

// Create adds a new database object on the remote server and registers it in local management.
// Executes SQL CREATE command via VeloArtifact and stores record in database.
func (s *dbObjectMgtService) Create(ctx context.Context, data models.DBObjectMgt) (*models.DBObjectMgt, error) {
	if data.DBMgt == 0 {
		return nil, fmt.Errorf("invalid database management ID: must be greater than 0")
	}

	tx := s.baseRepo.Begin()

	// Retrieve database management configuration
	dbmgt, err := s.dbMgtRepo.GetByID(tx, data.DBMgt)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to get dbmgt with id=%d: %w", data.DBMgt, err)
	}

	// Retrieve SQL template for object creation
	dbObject := s.dbobjectAllMap[data.ObjectId]
	if dbObject.SQLCreate == "" {
		tx.Rollback()
		return nil, fmt.Errorf("sql_create template is empty for objecttype=%d", data.ObjectId)
	}

	// Decode hex-encoded SQL template for security compliance
	sqlBytes, err := hex.DecodeString(dbObject.SQLCreate)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to decode hex SQL template: %w", err)
	}

	rawSQL := string(sqlBytes)
	logger.Debugf("Raw SQL: %s", rawSQL)

	// process rawsql
	finalSQL := strings.ReplaceAll(rawSQL, "${dbmgt.dbname}", dbmgt.DbName)
	finalSQL = strings.ReplaceAll(finalSQL, "${dbobjectmgt.objectname}", data.ObjectName)

	// Process optional SQL parameters (hex-encoded for security)
	if data.SqlParam != "" {
		sqlParamBytes, err := hex.DecodeString(data.SqlParam)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to decode hex sql_param: %w", err)
		}
		finalSQL = strings.ReplaceAll(finalSQL, "${dbobjectmgt.sql_param}", string(sqlParamBytes))
	}

	logger.Debugf("Final SQL: %s", finalSQL)

	// get cmt
	cmt, err := s.cntMgtRepo.GetCntMgtByID(tx, dbmgt.CntID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to get cntmgt with id=%d: %w", dbmgt.CntID, err)
	}

	// retrieve endpoints to get client_id and os.type from cmt.Agent
	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to get endpoint with id=%d: %w", cmt.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	// Prevent duplicate object creation for data consistency
	countObjMgt, err := s.dbObjectMgtRepo.CountByDbIdAndObjTypeAndObjName(tx, data.DBMgt, data.ObjectId, data.ObjectName)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if countObjMgt > 0 {
		tx.Rollback()
		return nil, fmt.Errorf("duplicate found for dbid=%d, objectname=%s, objecttype=%d", data.DBMgt, data.ObjectName, data.ObjectId)
	}

	queryParamBuilder := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cmt.CntType)).
		SetHost(cmt.IP).
		SetPort(cmt.Port).
		SetUser(cmt.Username).
		SetPassword(cmt.Password).
		SetQuery(finalSQL)

	// Oracle requires ServiceName as the database identifier for CDB/PDB routing
	if strings.ToLower(cmt.CntType) == "oracle" {
		queryParamBuilder.SetDatabase(cmt.ServiceName)
	} else {
		queryParamBuilder.SetDatabase(dbmgt.DbName)
	}

	queryParam := queryParamBuilder.Build()

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create agent command JSON: %w", err)
	}
	logger.Debugf("Created agent command JSON payload (hex): %s", hexJSON)

	_, err = agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", false)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to execute agent API command: %w", err)
	}

	// Register object in local management system
	dbObjectMgt := models.DBObjectMgt{
		DBMgt:       data.DBMgt,
		ObjectName:  data.ObjectName,
		ObjectId:    data.ObjectId,
		Description: data.Description,
		Status:      "enabled",
		SqlParam:    data.SqlParam,
	}

	if err := tx.Create(&dbObjectMgt).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create dbobjectmgt record: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return &dbObjectMgt, nil
}

// Update modifies an existing database object on the remote server and in local management.
// Executes SQL ALTER command via VeloArtifact and updates local record.
func (s *dbObjectMgtService) Update(ctx context.Context, id uint, data models.DBObjectMgt) (*models.DBObjectMgt, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid database object ID: must be greater than 0")
	}

	tx := s.baseRepo.Begin()

	// Verify object exists before update
	existing, err := s.dbObjectMgtRepo.GetById(tx, id)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to get dbobjectmgt with id=%d: %w", id, err)
	}

	// Retrieve database management configuration
	dbmgt, err := s.dbMgtRepo.GetByID(tx, existing.DBMgt)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to get dbmgt with id=%d: %w", existing.DBMgt, err)
	}

	dbObject := s.dbobjectAllMap[existing.ObjectId]
	if dbObject.SQLUpdate == "" {
		tx.Rollback()
		return nil, fmt.Errorf("sql_update template is empty for objecttype=%d", existing.ObjectId)
	}

	// Decode hex-encoded SQL template for security compliance
	sqlBytes, err := hex.DecodeString(dbObject.SQLUpdate)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to decode hex SQL template: %w", err)
	}

	rawSQL := string(sqlBytes)
	logger.Debugf("Raw SQL: %s", rawSQL)

	// process rawsql
	finalSQL := strings.ReplaceAll(rawSQL, "${dbmgt.dbname}", dbmgt.DbName)
	finalSQL = strings.ReplaceAll(finalSQL, "${dbobjectmgt.objectname}", existing.ObjectName)

	// Process optional SQL parameters (hex-encoded for security)
	if data.SqlParam != "" {
		sqlParamBytes, err := hex.DecodeString(data.SqlParam)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to decode hex sql_param: %w", err)
		}
		finalSQL = strings.ReplaceAll(finalSQL, "${dbobjectmgt.sql_param}", string(sqlParamBytes))
	}

	logger.Debugf("Final SQL: %s", finalSQL)

	// get cmt
	cmt, err := s.cntMgtRepo.GetCntMgtByID(tx, dbmgt.CntID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to get cntmgt with id=%d: %w", dbmgt.CntID, err)
	}

	// retrieve endpoints to get client_id and os.type from cmt.Agent
	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to get endpoint with id=%d: %w", cmt.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	// Prepare agent API execution parameters
	queryParamBuilder := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cmt.CntType)).
		SetHost(cmt.IP).
		SetPort(cmt.Port).
		SetUser(cmt.Username).
		SetPassword(cmt.Password).
		SetQuery(finalSQL)

	// Oracle requires ServiceName as the database identifier for CDB/PDB routing
	if strings.ToLower(cmt.CntType) == "oracle" {
		queryParamBuilder.SetDatabase(cmt.ServiceName)
	} else {
		queryParamBuilder.SetDatabase(dbmgt.DbName)
	}

	queryParam := queryParamBuilder.Build()

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create agent command JSON: %w", err)
	}
	logger.Debugf("Created agent command JSON payload (hex): %s", hexJSON)

	_, err = agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", false)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to execute agent API command: %w", err)
	}

	// Update local management record
	existing.Description = data.Description
	existing.SqlParam = data.SqlParam
	if err := tx.Save(&existing).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update dbobjectmgt id=%d: %w", id, err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return existing, nil
}

// Delete removes a database object from remote server and local management system.
// Executes SQL DROP command via VeloArtifact and deletes local record.
func (s *dbObjectMgtService) Delete(ctx context.Context, id uint) error {
	if id == 0 {
		return fmt.Errorf("invalid database object ID: must be greater than 0")
	}

	tx := s.baseRepo.Begin()

	existing, err := s.dbObjectMgtRepo.GetById(tx, id)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get dbobjectmgt with id=%d: %w", id, err)
	}

	// Retrieve database management configuration
	dbmgt, err := s.dbMgtRepo.GetByID(tx, existing.DBMgt)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get dbmgt with id=%d: %w", existing.DBMgt, err)
	}

	// Retrieve connection management configuration
	cmt, err := s.cntMgtRepo.GetCntMgtByID(tx, dbmgt.CntID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get cntmgt with id=%d: %w", dbmgt.CntID, err)
	}

	// Retrieve endpoint configuration for VeloArtifact execution
	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get endpoint with id=%d: %w", cmt.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	// Retrieve SQL template for object deletion
	dbObject := s.dbobjectAllMap[existing.ObjectId]
	if dbObject.SQLDelete == "" {
		tx.Rollback()
		return fmt.Errorf("sql_delete template is empty for objecttype=%d", existing.ObjectId)
	}

	// Decode hex-encoded SQL template for security compliance
	sqlBytes, err := hex.DecodeString(dbObject.SQLDelete)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to decode hex SQL template: %w", err)
	}

	rawSQL := string(sqlBytes)
	logger.Debugf("Raw SQL: %s", rawSQL)

	// process rawsql
	finalSQL := strings.ReplaceAll(rawSQL, "${dbmgt.dbname}", dbmgt.DbName)
	finalSQL = strings.ReplaceAll(finalSQL, "${dbobjectmgt.objectname}", existing.ObjectName)

	// Special handling for index objects (ObjectId 6) - requires table name extraction
	if existing.ObjectId == 6 {
		if existing.SqlParam != "" {
			sqlParamBytes, err := hex.DecodeString(existing.SqlParam)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to decode hex sql_param: %w", err)
			}

			regx := regexp.MustCompile(`(?i)ON\s+(\w+)\s*\(`)
			matches := regx.FindStringSubmatch(string(sqlParamBytes))

			logger.Debugf("Matches: %v", matches)

			if len(matches) > 1 {
				tableName := strings.TrimSpace(matches[1])
				finalSQL = strings.ReplaceAll(finalSQL, "${dbobjectmgt.objectname_byinputtype}", tableName)
			} else {
				tx.Rollback()
				return fmt.Errorf("cannot find table name")
			}
		}
	}
	logger.Debugf("Final SQL after substitution: %s", finalSQL)

	// Prepare agent API execution parameters
	queryParamBuilder := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cmt.CntType)).
		SetHost(cmt.IP).
		SetPort(cmt.Port).
		SetUser(cmt.Username).
		SetPassword(cmt.Password).
		SetQuery(finalSQL)

	// Oracle requires ServiceName as the database identifier for CDB/PDB routing
	if strings.ToLower(cmt.CntType) == "oracle" {
		queryParamBuilder.SetDatabase(cmt.ServiceName)
	} else {
		queryParamBuilder.SetDatabase(dbmgt.DbName)
	}

	queryParam := queryParamBuilder.Build()

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create agent command JSON for object deletion: %w", err)
	}

	_, err = agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", false)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to execute agent API command: %w", err)
	}

	// Remove from local management system
	if err := tx.Delete(&existing).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete dbobjectmgt id=%d: %w", id, err)
	}

	if err := tx.Commit().Error; err != nil {
		return err
	}
	return nil
}

// buildCombinedObjectQueryGroups creates individual keys for combined queries.
// Formats keys as "DB:dbmgt_id_ObjectType:object_id" for unique identification.
func (s *dbObjectMgtService) buildCombinedObjectQueryGroups(combinedSqlFinalMap map[string]objectInput) map[string][]string {
	listQuery := make(map[string][]string)

	// Use prefixed keys directly since they're already unique
	for prefixedKey, objectData := range combinedSqlFinalMap {
		// Parse "DbMgt:X_ObjectType:Y" format to extract dbmgt_id and create final key
		parts := strings.SplitN(prefixedKey, "_", 2)
		if len(parts) != 2 || !strings.HasPrefix(parts[0], "DbMgt:") {
			logger.Warnf("Invalid prefixed key format: %s", prefixedKey)
			continue
		}

		dbMgtIDStr := strings.TrimPrefix(parts[0], "DbMgt:")
		dbMgtID, err := strconv.ParseUint(dbMgtIDStr, 10, 32)
		if err != nil {
			logger.Warnf("Invalid dbmgt_id in key: %s", prefixedKey)
			continue
		}

		originalKey := parts[1] // This is "ObjectType:Y" or "ObjectType:Y_DependentIndex:N"

		// Format: "DB:dbmgt_id_ObjectType:object_id_*" for unique identification across databases
		finalKey := fmt.Sprintf("DB:%d_%s", dbMgtID, originalKey)

		// Use the pre-processed finalSQL from objectInput
		listQuery[finalKey] = []string{objectData.finalSQL} // Each key has exactly 1 query
		// logger.Debugf("Added combined query with key: %s, SQL: %s", finalKey, objectData.finalSQL)
	}

	return listQuery
}

// writeCombinedObjectQueryFile writes combined query data to JSON file for VeloArtifact processing.
// File-based execution reduces command complexity and enables audit trail.
func (s *dbObjectMgtService) writeCombinedObjectQueryFile(cntMgtID uint, listQuery map[string][]string) (string, error) {
	filename := fmt.Sprintf("getcombinedobject_%d_%s.json", cntMgtID, time.Now().Format("20060102_150405"))
	filePath := fmt.Sprintf("%s/%s", config.Cfg.DBFWebTempDir, filename)

	// JSON formatting ensures readability for debugging and audit purposes
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	// Only queries are written to file, connection params sent separately for security
	if err := encoder.Encode(listQuery); err != nil {
		logger.Errorf("Marshal combined listQuery error: %v", err)
		return "", fmt.Errorf("marshal combined listQuery error: %v", err)
	}

	// Ensure temp directory exists for VeloArtifact file operations
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Errorf("Create dir error: %v", err)
		return "", fmt.Errorf("create dir error: %v", err)
	}

	if err := os.WriteFile(filePath, buf.Bytes(), 0644); err != nil {
		logger.Errorf("Write combined listQuery to file error: %v", err)
		return "", fmt.Errorf("write combined listQuery to file error: %v", err)
	}

	logger.Infof("Combined Object ListQuery written to %s", filePath)
	return filename, nil
}
