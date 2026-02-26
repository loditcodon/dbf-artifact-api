package services

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// policyInput represents processed policy template data
type policyInput struct {
	policydf models.DBPolicyDefault
	actorId  uint
	objectId int
	dbmgtId  int    // Database management ID, -1 for all databases
	finalSQL string // Store the processed SQL to avoid re-processing
}

// PolicyJobContext contains context data needed for policy creation callback
type PolicyJobContext struct {
	DBMgtID     uint
	SqlFinalMap map[string]policyInput
	DBMgt       *models.DBMgt
	CMT         *models.CntMgt
	EndpointID  uint
}

// CombinedPolicyJobContext contains context data for combined policy job completion
type CombinedPolicyJobContext struct {
	CntMgtID   uint                            `json:"cnt_mgt_id"`
	DbMgts     []models.DBMgt                  `json:"db_mgts"`
	CMT        *models.CntMgt                  `json:"cmt"`
	EndpointID uint                            `json:"endpoint_id"`
	DbQueries  map[uint]map[string]policyInput `json:"db_queries"` // dbmgt_id -> queries
}

// DBPolicyService provides business logic for database policy operations and VeloArtifact execution.
type DBPolicyService interface {
	GetByCntMgt(ctx context.Context, id uint) (string, error)
	Create(ctx context.Context, data models.DBPolicy) (*models.DBPolicy, error)
	Update(ctx context.Context, id uint, data models.DBPolicy) (*models.DBPolicy, error)
	Delete(ctx context.Context, id uint) error
	BulkDelete(ctx context.Context, ids []uint) (deletedCount int, failedIDs []uint, errors []string)
	BulkUpdatePoliciesByActor(ctx context.Context, req dto.BulkPolicyUpdateRequest) (string, error)
}

type dbPolicyService struct {
	baseRepo               repository.BaseRepository
	cntMgtRepo             repository.CntMgtRepository
	dbPolicyRepo           repository.DBPolicyRepository
	dbPolicyDfRepo         repository.DBPolicyDefaultRepository
	dbMgtRepo              repository.DBMgtRepository
	dbActorMgtRepo         repository.DBActorMgtRepository
	dbObjectMgtRepo        repository.DBObjectMgtRepository
	endpointRepo           repository.EndpointRepository
	DBPolicyDefaultsAllMap map[uint]models.DBPolicyDefault
}

// NewDBPolicyService creates a new database policy service instance.
func NewDBPolicyService() DBPolicyService {
	return &dbPolicyService{
		baseRepo:               repository.NewBaseRepository(),
		cntMgtRepo:             repository.NewCntMgtRepository(),
		dbPolicyRepo:           repository.NewDBPolicyRepository(),
		dbPolicyDfRepo:         repository.NewDBPolicyDefaultRepository(),
		dbMgtRepo:              repository.NewDBMgtRepository(),
		dbActorMgtRepo:         repository.NewDBActorMgtRepository(),
		dbObjectMgtRepo:        repository.NewDBObjectMgtRepository(),
		endpointRepo:           repository.NewEndpointRepository(),
		DBPolicyDefaultsAllMap: bootstrap.DBPolicyDefaultsAllMap,
	}
}

// GetByDBMgt discovers and generates database policies for a specific database management instance.
// This is the most complex operation that processes hex-encoded SQL templates with variable substitution,
// executes them via VeloArtifact background jobs, and creates policies based on Allow/Deny rules.
// GetByCntMgt processes policy generation for all databases under a connection management instance.
// Routes to MySQL privilege session or Oracle privilege session based on connection type.
// Returns job message with background job ID for tracking.
func (s *dbPolicyService) GetByCntMgt(ctx context.Context, id uint) (string, error) {
	// Input validation at service boundary
	if id == 0 {
		return "", fmt.Errorf("invalid connection management ID: must be greater than 0")
	}
	if ctx == nil {
		return "", fmt.Errorf("context cannot be nil")
	}

	// Get connection management to determine database type
	cmt, err := s.cntMgtRepo.GetCntMgtByID(nil, id)
	if err != nil {
		return "", fmt.Errorf("cntmgt with id=%d not found: %v", id, err)
	}

	// Route to appropriate handler based on database type
	cntType := strings.ToLower(cmt.CntType)
	switch cntType {
	case "oracle":
		return s.GetByCntMgtWithOraclePrivilegeSession(ctx, id, cmt)
	case "mysql":
		return s.GetByCntMgtWithPrivilegeSession(ctx, id)
	default:
		return "", fmt.Errorf("unsupported database type: %s", cmt.CntType)
	}
}

// Create executes SqlUpdateAllow commands to grant database permissions and creates a new policy record.
// Performs real-time policy enforcement via VeloArtifact before database persistence.
// Returns created policy with enabled status.
func (s *dbPolicyService) Create(ctx context.Context, data models.DBPolicy) (*models.DBPolicy, error) {
	// Input validation at service boundary
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	if data.DBPolicyDefault == 0 {
		return nil, fmt.Errorf("invalid policy default ID: must be greater than 0")
	}
	if data.DBActorMgt == 0 {
		return nil, fmt.Errorf("invalid actor management ID: must be greater than 0")
	}
	if data.DBObjectMgt == 0 || (data.DBObjectMgt < -1) {
		return nil, fmt.Errorf("invalid object management ID: must be greater than 0 or -1 for wildcard")
	}
	if data.CntMgt == 0 && data.DBMgt == 0 {
		return nil, fmt.Errorf("either connection management ID or database management ID must be provided")
	}

	tx := s.baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	// execute sql in agent
	sqlUpdateAllow := s.DBPolicyDefaultsAllMap[data.DBPolicyDefault].SqlUpdateAllow
	if err := s.executePolicyUpdateSql(ctx, sqlUpdateAllow, data); err != nil {
		return nil, err
	}

	// create dbpolicy
	dbpolicy := models.DBPolicy{
		DBPolicyDefault: data.DBPolicyDefault,
		DBActorMgt:      data.DBActorMgt,
		DBObjectMgt:     data.DBObjectMgt,
		Status:          "enabled",
		Description:     data.Description,
	}

	// Set both fields if provided - database supports storing both
	dbpolicy.CntMgt = data.CntMgt
	dbpolicy.DBMgt = data.DBMgt
	if err := tx.Create(&dbpolicy).Error; err != nil {
		return nil, fmt.Errorf("[SERVICE] Insert dbpolicy fail: %v", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	txCommitted = true

	return &dbpolicy, nil
}

// Update revokes old permissions via SqlUpdateDeny, applies new permissions via SqlUpdateAllow,
// and updates the policy record. Ensures atomic permission changes to prevent security gaps.
// Returns updated policy with new configuration.
func (s *dbPolicyService) Update(ctx context.Context, id uint, data models.DBPolicy) (*models.DBPolicy, error) {
	// Input validation at service boundary
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	if id == 0 {
		return nil, fmt.Errorf("invalid policy ID: must be greater than 0")
	}
	if data.DBPolicyDefault == 0 {
		return nil, fmt.Errorf("invalid policy default ID: must be greater than 0")
	}
	if data.DBActorMgt == 0 {
		return nil, fmt.Errorf("invalid actor management ID: must be greater than 0")
	}
	if data.DBObjectMgt == 0 || (data.DBObjectMgt < -1) {
		return nil, fmt.Errorf("invalid object management ID: must be greater than 0 or -1 for wildcard")
	}

	tx := s.baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	// check dbpolicy exist
	dbpolicy, err := s.dbPolicyRepo.GetById(tx, id)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("dbpolicy with id=%d not found: %v", id, err)
	}

	// execute revoke command with old data dbpolicy
	sqlUpdatedataDeny := s.DBPolicyDefaultsAllMap[dbpolicy.DBPolicyDefault].SqlUpdateDeny
	if err := s.executePolicyUpdateSql(ctx, sqlUpdatedataDeny, *dbpolicy); err != nil {
		return nil, err
	}

	// execute grant command with new data dbpolicy
	if data.Status == "enabled" {
		sqlUpdatedataAllow := s.DBPolicyDefaultsAllMap[data.DBPolicyDefault].SqlUpdateAllow
		if err := s.executePolicyUpdateSql(ctx, sqlUpdatedataAllow, data); err != nil {
			return nil, err
		}
	}

	// update policy
	dbpolicy.CntMgt = data.CntMgt
	dbpolicy.DBMgt = data.DBMgt
	dbpolicy.DBActorMgt = data.DBActorMgt
	dbpolicy.DBObjectMgt = data.DBObjectMgt
	dbpolicy.DBPolicyDefault = data.DBPolicyDefault
	dbpolicy.Status = data.Status
	dbpolicy.Description = data.Description

	if err := tx.Save(&dbpolicy).Error; err != nil {
		return nil, fmt.Errorf("update dbpolicy=%d fail: %v", id, err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	txCommitted = true

	return dbpolicy, nil
}

// executePolicyUpdateSql performs hex-decoding of SQL commands, variable substitution,
// and executes policy changes via VeloArtifact. Critical for real-time permission enforcement.
// Returns error if VeloArtifact execution fails to maintain security consistency.
func (s *dbPolicyService) executePolicyUpdateSql(ctx context.Context, sqlcmd string, data models.DBPolicy) error {
	// Input validation at service boundary
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}
	if strings.TrimSpace(sqlcmd) == "" {
		return fmt.Errorf("SQL command cannot be empty")
	}
	if data.DBActorMgt == 0 {
		return fmt.Errorf("invalid actor management ID: must be greater than 0")
	}
	if data.DBObjectMgt == 0 || (data.DBObjectMgt < -1) {
		return fmt.Errorf("invalid object management ID: must be greater than 0 or -1 for wildcard")
	}

	var dbmgt models.DBMgt
	var cmtId uint

	// If both are provided, DBMgt takes priority and CntMgt overrides the connection
	if data.DBMgt != 0 && data.DBMgt != -1 {
		dbmgtbyid, err := s.dbMgtRepo.GetByID(nil, utils.MustIntToUint(data.DBMgt))
		if err != nil {
			return fmt.Errorf("DbMgt with id=%d not found: %v", data.DBMgt, err)
		}
		dbmgt = *dbmgtbyid
		logger.Infof("Found dbmgt with id=%d", data.DBMgt)

		// If CntMgt is also provided, use it to override the connection
		if data.CntMgt != 0 {
			cmtId = data.CntMgt
			logger.Infof("Using override CntMgt with id=%d", data.CntMgt)
		} else {
			cmtId = dbmgtbyid.CntID
			logger.Infof("Using default CntMgt with id=%d from DBMgt", dbmgtbyid.CntID)
		}
	} else if data.CntMgt != 0 {
		cmtId = data.CntMgt
		logger.Infof("Using CntMgt with id=%d", data.CntMgt)
	} else {
		return fmt.Errorf("either DBMgt or CntMgt must be provided")
	}

	cmt, err := s.cntMgtRepo.GetCntMgtByID(nil, cmtId)
	if err != nil {
		return fmt.Errorf("cntmgt with id=%d not found: %v", cmtId, err)
	}
	logger.Infof("Found cntmgt with id=%d", cmtId)

	ep, err := s.endpointRepo.GetByID(nil, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		return fmt.Errorf("cannot find endpoint with id=%d: %v", cmt.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	actor, err := s.dbActorMgtRepo.GetByID(nil, data.DBActorMgt)
	if err != nil {
		return fmt.Errorf("cannot find dbactormgt with id=%d: %v", data.DBActorMgt, err)
	}

	// Handle DBObjectMgt = -1 case
	var objectName string
	if data.DBObjectMgt == -1 {
		// TODO: Add validation for wildcard case if needed
		logger.Infof("Using wildcard DBObjectMgt (-1) for policy execution")
		objectName = "*" // Use wildcard for template substitution
	} else {
		object, err := s.dbObjectMgtRepo.GetById(nil, utils.MustIntToUint(data.DBObjectMgt))
		if err != nil {
			return fmt.Errorf("cannot find dbobjectmgt with id=%d: %v", data.DBObjectMgt, err)
		}
		objectName = object.ObjectName
	}

	if sqlcmd == "" {
		return fmt.Errorf("updatedata command is empty or not exist with policydefaultid=%d", data.DBPolicyDefault)
	}

	sqlBytes, err := hex.DecodeString(sqlcmd)
	if err != nil {
		return fmt.Errorf("unhex error: %v", err)
	}

	rawSQL := string(sqlBytes)

	// Handle DBMgt = -1 case for wildcard database name
	var dbName string
	if data.DBMgt == -1 {
		dbName = "*"
	} else {
		dbName = dbmgt.DbName
	}

	executeSql := strings.ReplaceAll(rawSQL, "${dbmgt.dbname}", dbName)
	executeSql = strings.ReplaceAll(executeSql, "${dbobjectmgt.objectname}", objectName)
	executeSql = strings.ReplaceAll(executeSql, "${dbactormgt.dbuser}", actor.DBUser)
	executeSql = strings.ReplaceAll(executeSql, "${dbactormgt.ip_address}", actor.IPAddress)

	// Oracle CDB/PDB scope substitution: CDB→"*", PDB→"PDB"
	if strings.ToLower(cmt.CntType) == "oracle" {
		connType := GetOracleConnectionType(cmt)
		executeSql = strings.ReplaceAll(executeSql, "${scope}", GetObjectTypeWildcard(connType))
	}

	queryParamBuilder := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cmt.CntType)).
		SetHost(cmt.IP).
		SetPort(cmt.Port).
		SetUser(cmt.Username).
		SetPassword(cmt.Password)

	// Oracle requires ServiceName as the database identifier for CDB/PDB routing
	if strings.ToLower(cmt.CntType) == "oracle" {
		queryParamBuilder.SetDatabase(cmt.ServiceName)
	} else if data.DBMgt != 0 && data.DBMgt != -1 {
		queryParamBuilder.SetDatabase(dbmgt.DbName)
	}

	queryParam := queryParamBuilder.Build()

	logger.Debugf("SQL command: %s", executeSql)

	queryParam.Query = executeSql

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		return fmt.Errorf("failed to create agent command JSON: %v", err)
	}

	_, err = agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", false)
	if err != nil {
		return fmt.Errorf("executeSqlAgentAPI error: %v", err)
	}
	return nil
}

// Delete revokes database permissions via SqlUpdateDeny and removes policy record.
// Performs real-time permission revocation via VeloArtifact before database deletion.
// Ensures complete cleanup of both permissions and policy data.
func (s *dbPolicyService) Delete(ctx context.Context, id uint) error {
	// Input validation at service boundary
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}
	if id == 0 {
		return fmt.Errorf("invalid policy ID: must be greater than 0")
	}

	tx := s.baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	// Get existing policy to execute revoke commands
	dbpolicy, err := s.dbPolicyRepo.GetById(tx, id)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("dbpolicy with id=%d not found: %v", id, err)
	}

	logger.Infof("Found policy to delete: id=%d, status=%s, policy_default=%d",
		id, dbpolicy.Status, dbpolicy.DBPolicyDefault)

	// Only execute revoke SQL if policy is currently enabled
	// Disabled policies should not have active permissions to revoke
	if dbpolicy.Status == "enabled" {
		// Get SqlUpdateDeny command from policy template for permission revocation
		sqlUpdateDeny := s.DBPolicyDefaultsAllMap[dbpolicy.DBPolicyDefault].SqlUpdateDeny
		if sqlUpdateDeny != "" {
			if err := s.executePolicyUpdateSql(ctx, sqlUpdateDeny, *dbpolicy); err != nil {
				return fmt.Errorf("failed to revoke permissions for policy %d: %v", id, err)
			}
			logger.Infof("Successfully revoked permissions for policy id=%d", id)
		} else {
			logger.Warnf("No SqlUpdateDeny command found for policy_default=%d", dbpolicy.DBPolicyDefault)
		}
	} else {
		logger.Infof("Policy id=%d is disabled, skipping permission revocation", id)
	}

	// Delete policy record from database after successful permission revocation
	if err := tx.Delete(&models.DBPolicy{}, id).Error; err != nil {
		return fmt.Errorf("failed to delete policy record with id=%d: %v", id, err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit delete transaction: %v", err)
	}
	txCommitted = true

	logger.Infof("Successfully deleted policy id=%d", id)
	return nil
}

// BulkDelete deletes multiple database policies by IDs.
// Each deletion is independent - failures do not affect other deletions.
// Returns count of successful deletions, list of failed IDs, and error messages.
func (s *dbPolicyService) BulkDelete(ctx context.Context, ids []uint) (deletedCount int, failedIDs []uint, errors []string) {
	// Input validation at service boundary
	if ctx == nil {
		return 0, ids, []string{"context cannot be nil"}
	}
	if len(ids) == 0 {
		return 0, nil, []string{"no policy IDs provided"}
	}

	logger.Infof("Starting bulk delete for %d policies", len(ids))

	deletedCount = 0
	failedIDs = make([]uint, 0)
	errors = make([]string, 0)

	// Process each ID independently to allow partial success
	for _, id := range ids {
		if id == 0 {
			failedIDs = append(failedIDs, id)
			errors = append(errors, fmt.Sprintf("ID %d: invalid policy ID (must be greater than 0)", id))
			continue
		}

		// Use existing Delete method for consistency
		if err := s.Delete(ctx, id); err != nil {
			failedIDs = append(failedIDs, id)
			errors = append(errors, fmt.Sprintf("ID %d: %v", id, err))
			logger.Errorf("Failed to delete policy %d in bulk operation: %v", id, err)
		} else {
			deletedCount++
		}
	}

	if deletedCount > 0 {
		logger.Infof("Bulk delete completed: deleted=%d, failed=%d", deletedCount, len(failedIDs))
	} else {
		logger.Warnf("Bulk delete completed with no successful deletions: failed=%d", len(failedIDs))
	}

	return deletedCount, failedIDs, errors
}

// getDBMgts gets all dbmgt records for MySQL ObjectId = 1
func (s *dbPolicyService) getDBMgts(tx *gorm.DB, cntID uint) ([]*models.DBMgt, error) {
	dbMgts, err := s.dbMgtRepo.GetAllByCntIDAndDBType(tx, cntID, "mysql")
	if err != nil {
		return nil, fmt.Errorf("failed to get dbmgt records for cntid=%d: %w", cntID, err)
	}

	// logger.Debugf("Retrieved %d database records for cntid=%d", len(dbMgts), cntID)
	return dbMgts, nil
}

// getDBActorMgts gets all dbactormgt records for MySQL ObjectId = 12
func (s *dbPolicyService) getDBActorMgts(tx *gorm.DB, cntID uint) ([]*models.DBActorMgt, error) {
	dbActorMgts, err := s.dbActorMgtRepo.GetAllByCntID(tx, cntID)
	if err != nil {
		return nil, fmt.Errorf("failed to get dbactormgt records for cntid=%d: %w", cntID, err)
	}

	// logger.Debugf("Retrieved %d actor records for cntid=%d", len(dbActorMgts), cntID)
	return dbActorMgts, nil
}

// GetByCntMgtWithPrivilegeSession uses in-memory MySQL server to execute policy templates
// Loads privilege data once via VeloArtifact background job, then processes policies in completion handler
func (s *dbPolicyService) GetByCntMgtWithPrivilegeSession(ctx context.Context, id uint) (string, error) {
	// Input validation at service boundary
	if id == 0 {
		return "", fmt.Errorf("invalid connection management ID: must be greater than 0")
	}
	if ctx == nil {
		return "", fmt.Errorf("context cannot be nil")
	}

	tx := s.baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	// Get all databases under this connection management
	dbmgts, err := s.dbMgtRepo.GetByCntMgtId(nil, id)
	if err != nil {
		return "", fmt.Errorf("cannot find dbmgt with cntid=%d: %v", id, err)
	}

	if len(dbmgts) == 0 {
		return "", fmt.Errorf("no databases found for cntmgt_id=%d", id)
	}

	logger.Infof("Found %d databases for cntmgt_id=%d", len(dbmgts), id)

	// Get connection management info
	cmt, err := s.cntMgtRepo.GetCntMgtByID(nil, id)
	if err != nil {
		return "", fmt.Errorf("cntmgt with id=%d not found: %v", id, err)
	}

	// Get endpoint
	ep, err := s.endpointRepo.GetByID(nil, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		return "", fmt.Errorf("cannot find endpoint with id=%d: %v", cmt.Agent, err)
	}
	logger.Infof("Found endpoint: id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	// Get database actors (users)
	dbActorMgts, err := s.dbActorMgtRepo.GetByCntMgt(nil, cmt.ID)
	if err != nil || len(dbActorMgts) == 0 {
		return "", fmt.Errorf("list dbactormgts with cntmgtid=%d no data: %v", cmt.ID, err)
	}

	logger.Infof("Found %d actors for cntmgt_id=%d", len(dbActorMgts), id)

	// Build privilege data query file
	privilegeQueries, err := s.buildPrivilegeDataQueries(dbActorMgts, dbmgts)
	if err != nil {
		return "", fmt.Errorf("failed to build privilege queries: %v", err)
	}

	// Write privilege queries to file
	filename, err := s.writePrivilegeQueryFile(id, privilegeQueries)
	if err != nil {
		return "", fmt.Errorf("failed to write privilege query file: %v", err)
	}

	// Start background job to load privilege data
	queryParam := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cmt.CntType)).
		SetHost(cmt.IP).
		SetPort(cmt.Port).
		SetUser(cmt.Username).
		SetPassword(cmt.Password).
		Build()
	queryParam.Query = filename
	queryParam.Action = "download"
	queryParam.Option = "--background"

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		return "", fmt.Errorf("failed to create agent command JSON: %v", err)
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

	// Start background job
	stdout, err := agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "download", hexJSON, "--background", true)
	if err != nil {
		return "", fmt.Errorf("failed to start agent API job: %v", err)
	}

	// Parse job response
	var jobResp JobResponse
	if err := json.Unmarshal([]byte(stdout), &jobResp); err != nil {
		return "", fmt.Errorf("failed to parse job response: %v", err)
	}

	if !jobResp.Success {
		return "", fmt.Errorf("job failed to start: %s", jobResp.Message)
	}

	logger.Infof("Privilege session VeloArtifact job started: job_id=%s, pid=%d", jobResp.JobID, jobResp.PID)

	// Prepare context data for job completion callback
	sessionID := fmt.Sprintf("cntmgt_%d_%d", id, time.Now().UnixNano())
	sessionContext := &PrivilegeSessionJobContext{
		CntMgtID:      id,
		DbMgts:        dbmgts,
		DbActorMgts:   dbActorMgts,
		CMT:           cmt,
		EndpointID:    ep.ID,
		SessionID:     sessionID,
		PrivilegeFile: filename,
	}

	contextData := map[string]interface{}{
		"privilege_session_context": sessionContext,
	}

	// Register job with monitoring system
	jobMonitor := GetJobMonitorService()
	completionCallback := CreatePrivilegeSessionCompletionHandler()
	jobMonitor.AddJobWithCallback(jobResp.JobID, id, ep.ClientID, ep.OsType, completionCallback, contextData)

	logger.Infof("Privilege session job added to monitoring: job_id=%s, cntmgt_id=%d, databases=%d", jobResp.JobID, id, len(dbmgts))

	// Rollback preparation transaction
	txCommitted = true
	tx.Rollback()

	logger.Infof("Privilege session job started: job_id=%s, cntmgt_id=%d, databases=%d",
		jobResp.JobID, id, len(dbmgts))
	return fmt.Sprintf("Privilege session background job started: %s. Processing %d databases with in-memory optimization.", jobResp.JobID, len(dbmgts)), nil
}

// GetByCntMgtWithOraclePrivilegeSession processes Oracle privilege collection via dbfAgentAPI.
// Queries Oracle system tables (DBA_SYS_PRIVS, DBA_TAB_PRIVS, V$PWFILE_USERS, DBA_ROLE_PRIVS).
// For CDB connections, also queries CDB_SYS_PRIVS for container-wide privileges.
// Returns job message with background job ID for tracking.
func (s *dbPolicyService) GetByCntMgtWithOraclePrivilegeSession(ctx context.Context, id uint, cmt *models.CntMgt) (string, error) {
	// Get all databases (schemas) under this Oracle connection
	dbmgts, err := s.dbMgtRepo.GetByCntMgtId(nil, id)
	if err != nil {
		return "", fmt.Errorf("cannot find dbmgt with cntid=%d: %v", id, err)
	}

	if len(dbmgts) == 0 {
		return "", fmt.Errorf("no databases/schemas found for oracle cntmgt_id=%d", id)
	}

	logger.Infof("Found %d Oracle schemas for cntmgt_id=%d", len(dbmgts), id)

	// Get endpoint for agent communication
	ep, err := s.endpointRepo.GetByID(nil, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		return "", fmt.Errorf("cannot find endpoint with id=%d: %v", cmt.Agent, err)
	}
	logger.Infof("Found endpoint: id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	// Get database actors (users)
	dbActorMgts, err := s.dbActorMgtRepo.GetByCntMgt(nil, cmt.ID)
	if err != nil || len(dbActorMgts) == 0 {
		return "", fmt.Errorf("list dbactormgts with cntmgtid=%d no data: %v", cmt.ID, err)
	}

	logger.Infof("Found %d Oracle actors for cntmgt_id=%d", len(dbActorMgts), id)

	// Determine Oracle connection type (CDB or PDB)
	connType := GetOracleConnectionType(cmt)
	logger.Infof("Oracle connection type: %s for cntmgt_id=%d", connType.String(), id)

	// Build Oracle privilege data queries (dbActorMgts and dbmgts are already []models.* value slices)
	privilegeQueries, err := s.buildOraclePrivilegeDataQueries(dbActorMgts, connType)
	if err != nil {
		return "", fmt.Errorf("failed to build oracle privilege queries: %v", err)
	}

	// Build Oracle object queries for schema objects
	objectQueries, err := s.buildOracleObjectQueries(dbmgts, connType)
	if err != nil {
		return "", fmt.Errorf("failed to build oracle object queries: %v", err)
	}

	// Merge privilege and object queries
	for key, value := range objectQueries {
		privilegeQueries[key] = value
	}

	// Write queries to file for agent execution
	filename, err := s.writeOraclePrivilegeQueryFile(id, privilegeQueries)
	if err != nil {
		return "", fmt.Errorf("failed to write oracle privilege query file: %v", err)
	}

	// Build query parameters for dbfAgentAPI
	// For Oracle, use ServiceName in the Database field
	queryParam := dto.NewDBQueryParamBuilder().
		SetDBType("oracle").
		SetHost(cmt.IP).
		SetPort(cmt.Port).
		SetUser(cmt.Username).
		SetPassword(cmt.Password).
		SetDatabase(cmt.ServiceName). // Oracle uses ServiceName for connection
		Build()
	queryParam.Query = filename
	queryParam.Action = "download"
	queryParam.Option = "--background"

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		return "", fmt.Errorf("failed to create agent command JSON: %v", err)
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

	// Start background job
	stdout, err := agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "download", hexJSON, "--background", true)
	if err != nil {
		return "", fmt.Errorf("failed to start oracle agent API job: %v", err)
	}

	// Parse job response
	var jobResp JobResponse
	if err := json.Unmarshal([]byte(stdout), &jobResp); err != nil {
		return "", fmt.Errorf("failed to parse oracle job response: %v", err)
	}

	if !jobResp.Success {
		return "", fmt.Errorf("oracle job failed to start: %s", jobResp.Message)
	}

	logger.Infof("Oracle privilege session job started: job_id=%s, pid=%d, conn_type=%s",
		jobResp.JobID, jobResp.PID, connType.String())

	// Prepare context data for job completion callback
	sessionID := fmt.Sprintf("oracle_cntmgt_%d_%d", id, time.Now().UnixNano())
	sessionContext := &OraclePrivilegeSessionJobContext{
		CntMgtID:      id,
		CMT:           cmt,
		EndpointID:    ep.ID,
		ConnType:      connType,
		DbActorMgts:   dbActorMgts,
		DbMgts:        dbmgts,
		SessionID:     sessionID,
		PrivilegeFile: filename,
	}

	contextData := map[string]interface{}{
		"oracle_privilege_session_context": sessionContext,
	}

	// Register job with monitoring system
	jobMonitor := GetJobMonitorService()
	completionCallback := CreateOraclePrivilegeSessionCompletionHandler()
	jobMonitor.AddJobWithCallback(jobResp.JobID, id, ep.ClientID, ep.OsType, completionCallback, contextData)

	logger.Infof("Oracle privilege session job added to monitoring: job_id=%s, cntmgt_id=%d, schemas=%d, conn_type=%s",
		jobResp.JobID, id, len(dbmgts), connType.String())

	return fmt.Sprintf("Oracle privilege session background job started: %s. Processing %d schemas (%s mode).",
		jobResp.JobID, len(dbmgts), connType.String()), nil
}

// processGeneralTemplatesWithSession executes general SQL templates against in-memory privilege session
// Mirrors processGeneralSQLTemplates logic but uses in-memory session instead of building queries

// extractResultValue extracts first value from query result
// Returns "NULL" if result is empty or first value is nil
func (s *dbPolicyService) extractResultValue(result []map[string]interface{}) string {
	if len(result) == 0 {
		return "NULL"
	}

	// Get first value from first row (any column)
	for _, value := range result[0] {
		if value == nil {
			return "NULL"
		}
		return fmt.Sprintf("%v", value)
	}

	return "NULL"
}

// isPolicyAllowed checks if query result matches allow/deny criteria
// Matches logic from policy_completion_handler.go processQueryResult and isPolicyAllowed
func (s *dbPolicyService) isPolicyAllowed(output, resAllow, resDeny string) bool {
	if output == resDeny {
		return false
	}
	if output == resAllow {
		return true
	}
	// Special case: "NOT NULL" means any non-null value is allowed
	if resAllow == "NOT NULL" && output != "NULL" {
		return true
	}
	return false
}

// buildPrivilegeDataQueries builds queries to fetch all privilege table data
func (s *dbPolicyService) buildPrivilegeDataQueries(actors []models.DBActorMgt, databases []models.DBMgt) (map[string][]string, error) {
	// Build filters for actors
	actorPairs := []string{}
	for _, actor := range actors {
		actorPairs = append(actorPairs, fmt.Sprintf("('%s', '%s')",
			escapeSQL(actor.DBUser), escapeSQL(actor.IPAddress)))
	}
	actorFilter := strings.Join(actorPairs, ", ")

	// Build filters for databases
	dbNames := []string{}
	for _, db := range databases {
		dbNames = append(dbNames, fmt.Sprintf("'%s'", escapeSQL(db.DbName)))
	}
	dbFilter := strings.Join(dbNames, ", ")

	// Build privilege data queries with explicit column ordering
	queries := map[string][]string{
		// "mysql.user": {
		// 	fmt.Sprintf(`SELECT Host, User, Select_priv, Insert_priv, Update_priv, Delete_priv, Create_priv, Drop_priv,
		// 		Reload_priv, Shutdown_priv, Process_priv, File_priv, Grant_priv, References_priv, Index_priv, Alter_priv,
		// 		Show_db_priv, Super_priv, Create_tmp_table_priv, Lock_tables_priv, Execute_priv, Repl_slave_priv,
		// 		Repl_client_priv, Create_view_priv, Show_view_priv, Create_routine_priv, Alter_routine_priv,
		// 		Create_user_priv, Event_priv, Trigger_priv, Create_tablespace_priv, Create_role_priv, Resource_group_admin_priv
		// 		FROM mysql.user WHERE (User, Host) IN (%s)`, actorFilter),
		// },
		// REMOVE Resource_group_admin_priv for MySQL 5.7 compatibility
		// Will need to handle version differences if supporting both MySQL 5.7 and 8.0+
		// For now, assume MySQL 5.7 compatibility
		"mysql.user": {
			fmt.Sprintf(`SELECT Host, User, Select_priv, Insert_priv, Update_priv, Delete_priv, Create_priv, Drop_priv,
				Reload_priv, Shutdown_priv, Process_priv, File_priv, Grant_priv, References_priv, Index_priv, Alter_priv,
				Show_db_priv, Super_priv, Create_tmp_table_priv, Lock_tables_priv, Execute_priv, Repl_slave_priv,
				Repl_client_priv, Create_view_priv, Show_view_priv, Create_routine_priv, Alter_routine_priv,
				Create_user_priv, Event_priv, Trigger_priv, Create_tablespace_priv
				FROM mysql.user WHERE (User, Host) IN (%s)`, actorFilter),
		},
		"mysql.db": {
			fmt.Sprintf(`SELECT Host, Db, User, Select_priv, Insert_priv, Update_priv, Delete_priv, Create_priv, Drop_priv,
				Grant_priv, References_priv, Index_priv, Alter_priv, Create_tmp_table_priv, Lock_tables_priv,
				Create_view_priv, Show_view_priv, Create_routine_priv, Alter_routine_priv, Execute_priv, Event_priv, Trigger_priv
				FROM mysql.db WHERE (User, Host) IN (%s)`, actorFilter),
		},
		"mysql.tables_priv": {
			fmt.Sprintf(`SELECT Host, Db, User, Table_name, Grantor, Timestamp, Table_priv, Column_priv
				FROM mysql.tables_priv WHERE (User, Host) IN (%s) AND Db IN (%s)`, actorFilter, dbFilter),
		},
		"mysql.procs_priv": {
			fmt.Sprintf(`SELECT Host, Db, User, Routine_name, Routine_type, Grantor, Timestamp, Proc_priv
				FROM mysql.procs_priv WHERE (User, Host) IN (%s) AND Db IN (%s)`, actorFilter, dbFilter),
		},
		"mysql.role_edges": {
			fmt.Sprintf(`SELECT FROM_HOST, FROM_USER, TO_HOST, TO_USER, WITH_ADMIN_OPTION
				FROM mysql.role_edges WHERE (TO_USER, TO_HOST) IN (%s)`, actorFilter),
		},
		"mysql.global_grants": {
			fmt.Sprintf(`SELECT USER, HOST, PRIV, WITH_GRANT_OPTION
				FROM mysql.global_grants WHERE (USER, HOST) IN (%s)`, actorFilter),
		},
		"mysql.proxies_priv": {
			fmt.Sprintf(`SELECT Host, User, Proxied_host, Proxied_user, With_grant, Grantor, Timestamp
				FROM mysql.proxies_priv WHERE (User, Host) IN (%s)`, actorFilter),
		},
		"information_schema.USER_PRIVILEGES": {
			fmt.Sprintf(`SELECT GRANTEE, TABLE_CATALOG, PRIVILEGE_TYPE, IS_GRANTABLE
				FROM information_schema.USER_PRIVILEGES WHERE GRANTEE IN (%s)`, buildGranteeFilter(actors)),
		},
		"information_schema.SCHEMA_PRIVILEGES": {
			fmt.Sprintf(`SELECT GRANTEE, TABLE_CATALOG, TABLE_SCHEMA, PRIVILEGE_TYPE, IS_GRANTABLE
				FROM information_schema.SCHEMA_PRIVILEGES WHERE GRANTEE IN (%s) AND TABLE_SCHEMA IN (%s)`, buildGranteeFilter(actors), dbFilter),
		},
		"information_schema.TABLE_PRIVILEGES": {
			fmt.Sprintf(`SELECT GRANTEE, TABLE_CATALOG, TABLE_SCHEMA, TABLE_NAME, PRIVILEGE_TYPE, IS_GRANTABLE
				FROM information_schema.TABLE_PRIVILEGES WHERE GRANTEE IN (%s) AND TABLE_SCHEMA IN (%s)`, buildGranteeFilter(actors), dbFilter),
		},
	}

	logger.Infof("Built %d privilege data queries for %d actors and %d databases", len(queries), len(actors), len(databases))
	return queries, nil
}

// buildGranteeFilter builds GRANTEE filter for information_schema queries
func buildGranteeFilter(actors []models.DBActorMgt) string {
	grantees := []string{}
	for _, actor := range actors {
		grantees = append(grantees, fmt.Sprintf("\"'%s'@'%s'\"",
			escapeSQL(actor.DBUser), escapeSQL(actor.IPAddress)))
	}
	return strings.Join(grantees, ", ")
}

// escapeSQL escapes single quotes in SQL strings
func escapeSQL(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// writePrivilegeQueryFile writes privilege queries to JSON file
func (s *dbPolicyService) writePrivilegeQueryFile(cntMgtID uint, queries map[string][]string) (string, error) {
	filename := fmt.Sprintf("getprivilegedata_%d_%s.json", cntMgtID, time.Now().Format("20060102_150405"))
	filePath := fmt.Sprintf("%s/%s", config.Cfg.DBFWebTempDir, filename)

	// JSON formatting
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(queries); err != nil {
		logger.Errorf("Marshal privilege queries error: %v", err)
		return "", fmt.Errorf("marshal privilege queries error: %v", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Errorf("Create dir error: %v", err)
		return "", fmt.Errorf("create dir error: %v", err)
	}

	if err := os.WriteFile(filePath, buf.Bytes(), 0644); err != nil {
		logger.Errorf("Write privilege queries to file error: %v", err)
		return "", fmt.Errorf("write privilege queries to file error: %v", err)
	}

	logger.Infof("Privilege queries written to %s", filePath)
	return filename, nil
}

// BulkUpdatePoliciesByActor performs bulk policy update for a specific actor
// Compares existing policies with new desired state and executes changes via VeloArtifact
func (s *dbPolicyService) BulkUpdatePoliciesByActor(ctx context.Context, req dto.BulkPolicyUpdateRequest) (string, error) {
	// Input validation at service boundary
	if ctx == nil {
		return "", fmt.Errorf("context cannot be nil")
	}
	if req.CntMgtID == 0 {
		return "", fmt.Errorf("invalid connection management ID: must be greater than 0")
	}
	if req.DBMgtID == 0 {
		return "", fmt.Errorf("invalid database management ID: must be greater than 0")
	}
	if req.DBActorMgtID == 0 {
		return "", fmt.Errorf("invalid actor management ID: must be greater than 0")
	}
	if len(req.NewPolicyDefaults) == 0 {
		return "", fmt.Errorf("new policy defaults list cannot be empty")
	}
	if len(req.NewObjectMgts) == 0 {
		return "", fmt.Errorf("new object management list cannot be empty")
	}

	tx := s.baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	dbmgt, err := s.dbMgtRepo.GetByID(tx, req.DBMgtID)
	if err != nil {
		return "", fmt.Errorf("dbmgt with id=%d not found: %v", req.DBMgtID, err)
	}
	logger.Infof("Found dbmgt: id=%d, db_name=%s, cnt_id=%d", req.DBMgtID, dbmgt.DbName, dbmgt.CntID)

	cmt, err := s.cntMgtRepo.GetCntMgtByID(tx, req.CntMgtID)
	if err != nil {
		return "", fmt.Errorf("cntmgt with id=%d not found: %v", req.CntMgtID, err)
	}
	logger.Infof("Found cmt: id=%d, cnt_type=%s, username=%s, agent=%d", cmt.ID, cmt.CntType, cmt.Username, cmt.Agent)

	actor, err := s.dbActorMgtRepo.GetByID(tx, req.DBActorMgtID)
	if err != nil {
		return "", fmt.Errorf("dbactormgt with id=%d not found: %v", req.DBActorMgtID, err)
	}
	logger.Infof("Found actor: id=%d, dbuser=%s, ip=%s", actor.ID, actor.DBUser, actor.IPAddress)

	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		return "", fmt.Errorf("cannot find endpoint with id=%d: %v", cmt.Agent, err)
	}
	logger.Infof("Found endpoint: id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	existingCombinations, err := s.getExistingPolicyCombinations(tx, req.CntMgtID, req.DBMgtID, req.DBActorMgtID)
	if err != nil {
		return "", fmt.Errorf("failed to get existing policies: %v", err)
	}
	logger.Infof("Found %d existing policy combinations for actor=%d", len(existingCombinations), req.DBActorMgtID)

	toAdd, toRemove := s.calculatePolicyDiff(existingCombinations, req.NewPolicyDefaults, req.NewObjectMgts)
	logger.Infof("Calculated diff: %d to add, %d to remove", len(toAdd), len(toRemove))

	// Early return when no changes detected
	if len(toAdd) == 0 && len(toRemove) == 0 {
		txCommitted = true
		tx.Rollback()
		return "No changes detected - existing policies match desired state", nil
	}

	commandMap, err := s.buildBulkPolicyCommands(toAdd, toRemove, dbmgt, actor, cmt)
	if err != nil {
		return "", fmt.Errorf("failed to build bulk policy commands: %v", err)
	}
	logger.Infof("Built %d SQL commands for bulk policy update", len(commandMap))

	filename, err := s.writeBulkPolicyUpdateFile(req.CntMgtID, req.DBMgtID, req.DBActorMgtID, commandMap)
	if err != nil {
		return "", fmt.Errorf("failed to write bulk policy update file: %v", err)
	}
	bulkQueryParamBuilder := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cmt.CntType)).
		SetHost(cmt.IP).
		SetPort(cmt.Port).
		SetUser(cmt.Username).
		SetPassword(cmt.Password)

	// Oracle requires ServiceName as the database identifier for CDB/PDB routing
	if strings.ToLower(cmt.CntType) == "oracle" {
		bulkQueryParamBuilder.SetDatabase(cmt.ServiceName)
	} else {
		bulkQueryParamBuilder.SetDatabase(dbmgt.DbName)
	}

	queryParam := bulkQueryParamBuilder.Build()
	queryParam.Query = filename
	queryParam.Action = "download"
	queryParam.Option = "--background"

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		return "", fmt.Errorf("failed to create agent command JSON: %v", err)
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

	// Start background job
	stdout, err := agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "download", hexJSON, "--background", true)
	if err != nil {
		return "", fmt.Errorf("failed to start agent API job: %v", err)
	}

	// Parse job response
	var jobResp JobResponse
	if err := json.Unmarshal([]byte(stdout), &jobResp); err != nil {
		return "", fmt.Errorf("failed to parse job response: %v", err)
	}

	if !jobResp.Success {
		return "", fmt.Errorf("job failed to start: %s", jobResp.Message)
	}

	logger.Infof("Bulk policy update VeloArtifact job started: job_id=%s, pid=%d", jobResp.JobID, jobResp.PID)

	bulkContext := &dto.BulkPolicyUpdateJobContext{
		CntMgtID:        req.CntMgtID,
		DBMgtID:         req.DBMgtID,
		DBActorMgtID:    req.DBActorMgtID,
		PolicesToAdd:    toAdd,
		PolicesToRemove: toRemove,
		DBMgt:           dbmgt,
		CMT:             cmt,
		Actor:           actor,
		EndpointID:      ep.ID,
		CommandMap:      commandMap,
	}

	contextData := map[string]interface{}{
		"bulk_policy_context": bulkContext,
	}

	// Add job to monitoring system with completion callback
	jobMonitor := GetJobMonitorService()
	completionCallback := CreateBulkPolicyUpdateCompletionHandler()
	jobMonitor.AddJobWithCallback(jobResp.JobID, req.DBMgtID, ep.ClientID, ep.OsType, completionCallback, contextData)

	logger.Infof("Bulk policy update job added to monitoring: job_id=%s, actor_id=%d, add=%d, remove=%d",
		jobResp.JobID, req.DBActorMgtID, len(toAdd), len(toRemove))

	// Rollback preparation transaction - actual DB updates happen in completion handler
	txCommitted = true
	tx.Rollback()

	logger.Infof("Bulk policy update job started: job_id=%s, actor_id=%d, add=%d, remove=%d",
		jobResp.JobID, req.DBActorMgtID, len(toAdd), len(toRemove))
	return fmt.Sprintf("Bulk policy update background job started: %s. Adding %d policies, removing %d policies.",
		jobResp.JobID, len(toAdd), len(toRemove)), nil
}

// getExistingPolicyCombinations retrieves existing policy combinations for an actor
func (s *dbPolicyService) getExistingPolicyCombinations(tx *gorm.DB, cntMgtID, dbMgtID, actorMgtID uint) ([]dto.PolicyCombination, error) {
	policies, err := s.dbPolicyRepo.GetPoliciesByActorAndScope(tx, cntMgtID, dbMgtID, actorMgtID)
	if err != nil {
		return nil, fmt.Errorf("failed to query existing policies: %v", err)
	}

	combinations := make([]dto.PolicyCombination, 0, len(policies))
	for _, policy := range policies {
		combinations = append(combinations, dto.PolicyCombination{
			PolicyDefaultID: policy.DBPolicyDefault,
			ObjectMgtID:     utils.MustIntToUint(policy.DBObjectMgt),
			DBPolicyID:      policy.ID,
		})
	}

	return combinations, nil
}

// calculatePolicyDiff computes policies to add and remove by comparing old and new combinations
func (s *dbPolicyService) calculatePolicyDiff(
	existing []dto.PolicyCombination,
	newPolicyDefaults []uint,
	newObjectMgts []uint,
) (toAdd, toRemove []dto.PolicyCombination) {
	// Build set of existing combinations for fast lookup
	existingSet := make(map[string]dto.PolicyCombination)
	for _, combo := range existing {
		key := fmt.Sprintf("%d_%d", combo.PolicyDefaultID, combo.ObjectMgtID)
		existingSet[key] = combo
	}

	// Build set of desired combinations (Cartesian product)
	desiredSet := make(map[string]dto.PolicyCombination)
	for _, policyDefaultID := range newPolicyDefaults {
		for _, objectMgtID := range newObjectMgts {
			key := fmt.Sprintf("%d_%d", policyDefaultID, objectMgtID)
			desiredSet[key] = dto.PolicyCombination{
				PolicyDefaultID: policyDefaultID,
				ObjectMgtID:     objectMgtID,
			}
		}
	}

	// Calculate toAdd = desired - existing
	for key, combo := range desiredSet {
		if _, exists := existingSet[key]; !exists {
			toAdd = append(toAdd, combo)
		}
	}

	// Calculate toRemove = existing - desired
	for key, combo := range existingSet {
		if _, exists := desiredSet[key]; !exists {
			toRemove = append(toRemove, combo)
		}
	}

	return toAdd, toRemove
}

// buildBulkPolicyCommands builds SQL commands for policies to add and remove
func (s *dbPolicyService) buildBulkPolicyCommands(
	toAdd, toRemove []dto.PolicyCombination,
	dbmgt *models.DBMgt,
	actor *models.DBActorMgt,
	cmt *models.CntMgt,
) (map[string]dto.CommandDetail, error) {
	commandMap := make(map[string]dto.CommandDetail)

	// Oracle CDB/PDB scope substitution: CDB→"*", PDB→"PDB"
	var oracleScope string
	if strings.ToLower(cmt.CntType) == "oracle" {
		connType := GetOracleConnectionType(cmt)
		oracleScope = GetObjectTypeWildcard(connType)
	}

	// Build REVOKE commands for policies to remove
	for _, combo := range toRemove {
		policyDefault, exists := s.DBPolicyDefaultsAllMap[combo.PolicyDefaultID]
		if !exists {
			return nil, fmt.Errorf("policy default id=%d not found in bootstrap data", combo.PolicyDefaultID)
		}

		if policyDefault.SqlUpdateDeny == "" {
			logger.Warnf("No SqlUpdateDeny for policy_default=%d, skipping revoke", combo.PolicyDefaultID)
			continue
		}

		// Get object name
		var objectName string
		if combo.ObjectMgtID == uint(^uint(0)) { // max uint = -1 cast to uint
			objectName = "*"
		} else {
			object, err := s.dbObjectMgtRepo.GetById(nil, combo.ObjectMgtID)
			if err != nil {
				return nil, fmt.Errorf("object with id=%d not found: %v", combo.ObjectMgtID, err)
			}
			objectName = object.ObjectName
		}

		// Hex-decode and substitute variables
		sqlBytes, err := hex.DecodeString(policyDefault.SqlUpdateDeny)
		if err != nil {
			return nil, fmt.Errorf("failed to hex-decode SqlUpdateDeny: %v", err)
		}
		rawSQL := string(sqlBytes)

		executeSql := strings.ReplaceAll(rawSQL, "${dbmgt.dbname}", dbmgt.DbName)
		executeSql = strings.ReplaceAll(executeSql, "${dbobjectmgt.objectname}", objectName)
		executeSql = strings.ReplaceAll(executeSql, "${dbactormgt.dbuser}", actor.DBUser)
		executeSql = strings.ReplaceAll(executeSql, "${dbactormgt.ip_address}", actor.IPAddress)
		if oracleScope != "" {
			executeSql = strings.ReplaceAll(executeSql, "${scope}", oracleScope)
		}

		uniqueKey := fmt.Sprintf("Remove_PolicyDf:%d_Object:%d", combo.PolicyDefaultID, combo.ObjectMgtID)
		commandMap[uniqueKey] = dto.CommandDetail{
			SQL:               executeSql,
			Action:            "remove",
			PolicyCombination: combo,
		}
	}

	// Build GRANT commands for policies to add
	for _, combo := range toAdd {
		policyDefault, exists := s.DBPolicyDefaultsAllMap[combo.PolicyDefaultID]
		if !exists {
			return nil, fmt.Errorf("policy default id=%d not found in bootstrap data", combo.PolicyDefaultID)
		}

		if policyDefault.SqlUpdateAllow == "" {
			return nil, fmt.Errorf("no SqlUpdateAllow for policy_default=%d", combo.PolicyDefaultID)
		}

		// Get object name
		var objectName string
		if combo.ObjectMgtID == uint(^uint(0)) { // max uint = -1 cast to uint
			objectName = "*"
		} else {
			object, err := s.dbObjectMgtRepo.GetById(nil, combo.ObjectMgtID)
			if err != nil {
				return nil, fmt.Errorf("object with id=%d not found: %v", combo.ObjectMgtID, err)
			}
			objectName = object.ObjectName
		}

		// Hex-decode and substitute variables
		sqlBytes, err := hex.DecodeString(policyDefault.SqlUpdateAllow)
		if err != nil {
			return nil, fmt.Errorf("failed to hex-decode SqlUpdateAllow: %v", err)
		}
		rawSQL := string(sqlBytes)

		executeSql := strings.ReplaceAll(rawSQL, "${dbmgt.dbname}", dbmgt.DbName)
		executeSql = strings.ReplaceAll(executeSql, "${dbobjectmgt.objectname}", objectName)
		executeSql = strings.ReplaceAll(executeSql, "${dbactormgt.dbuser}", actor.DBUser)
		executeSql = strings.ReplaceAll(executeSql, "${dbactormgt.ip_address}", actor.IPAddress)
		if oracleScope != "" {
			executeSql = strings.ReplaceAll(executeSql, "${scope}", oracleScope)
		}

		uniqueKey := fmt.Sprintf("Add_PolicyDf:%d_Object:%d", combo.PolicyDefaultID, combo.ObjectMgtID)
		commandMap[uniqueKey] = dto.CommandDetail{
			SQL:               executeSql,
			Action:            "add",
			PolicyCombination: combo,
		}
	}

	return commandMap, nil
}

// writeBulkPolicyUpdateFile writes bulk policy update commands to JSON file
func (s *dbPolicyService) writeBulkPolicyUpdateFile(cntMgtID, dbMgtID, actorMgtID uint, commandMap map[string]dto.CommandDetail) (string, error) {
	filename := fmt.Sprintf("bulkpolicyupdate_cnt%d_db%d_actor%d_%s.json",
		cntMgtID, dbMgtID, actorMgtID, time.Now().Format("20060102_150405"))
	filePath := fmt.Sprintf("%s/%s", config.Cfg.DBFWebTempDir, filename)

	// Convert commandMap to simple map[string][]string format for VeloArtifact
	listQuery := make(map[string][]string)
	for key, cmdDetail := range commandMap {
		listQuery[key] = []string{cmdDetail.SQL}
	}

	// JSON formatting
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(listQuery); err != nil {
		logger.Errorf("Marshal bulk policy update commands error: %v", err)
		return "", fmt.Errorf("marshal bulk policy update commands error: %v", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Errorf("Create dir error: %v", err)
		return "", fmt.Errorf("create dir error: %v", err)
	}

	if err := os.WriteFile(filePath, buf.Bytes(), 0644); err != nil {
		logger.Errorf("Write bulk policy update commands to file error: %v", err)
		return "", fmt.Errorf("write bulk policy update commands to file error: %v", err)
	}

	logger.Infof("Bulk policy update commands written to %s", filePath)
	return filename, nil
}
