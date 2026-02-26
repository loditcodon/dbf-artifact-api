package services

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

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

// Oracle CDB/PDB user query constants following pdb_service.go pattern
const (
	// oracleCDBUsersSQL fetches common users from Oracle CDB root container.
	// Filters: con_id=1 (root), common='YES' (shared across containers), oracle_maintained='N' (user-created only).
	// oracleCDBUsersSQL = "SELECT username FROM CDB_USERS WHERE con_id = 1 AND common = 'YES' AND oracle_maintained = 'N';"
	oracleCDBUsersSQL = "SELECT username FROM CDB_USERS WHERE con_id = 1;"

	// oraclePDBUsersSQL fetches local users for a specific Oracle PDB.
	// Uses V$CONTAINERS subquery to resolve PDB name to con_id dynamically.
	// Filters: oracle_maintained='N' (user-created), common='NO' (PDB-local only).
	// Requires ${pdbname} variable substitution before execution.
	oraclePDBUsersSQL = "SELECT username FROM CDB_USERS WHERE con_id = (SELECT con_id FROM V$CONTAINERS WHERE name = '${pdbname}') AND oracle_maintained = 'N' AND common = 'NO';"
)

// DBActorMgtService provides business logic for database actor management operations.
type DBActorMgtService interface {
	CreateAll(ctx context.Context, cntMgtID uint) (int, error)
	Create(ctx context.Context, data models.DBActorMgt) (*models.DBActorMgt, error)
	Update(ctx context.Context, id uint, data dto.DBActorMgtUpdate) (*models.DBActorMgt, error)
	Delete(ctx context.Context, id uint) error
}

type dbActorMgtService struct {
	baseRepo       repository.BaseRepository
	cntmgtRepo     repository.CntMgtRepository
	dbActorRepo    repository.DBActorRepository
	endpointRepo   repository.EndpointRepository
	dbActorMgtRepo repository.DBActorMgtRepository
	dbMgtRepo      repository.DBMgtRepository
	actorAll       []models.DBActor
}

// NewDBActorMgtService creates a new database actor management service instance.
func NewDBActorMgtService() DBActorMgtService {
	return &dbActorMgtService{
		baseRepo:       repository.NewBaseRepository(),
		cntmgtRepo:     repository.NewCntMgtRepository(),
		dbActorRepo:    repository.NewDBActorRepository(),
		endpointRepo:   repository.NewEndpointRepository(),
		dbActorMgtRepo: repository.NewDBActorMgtRepository(),
		dbMgtRepo:      repository.NewDBMgtRepository(),
		actorAll:       bootstrap.DBActorAll,
	}
}

// CreateAll synchronizes all database users from remote server to local management system.
// Performs two-phase sync: INSERT missing users, DELETE obsolete users.
// Returns total number of changes (inserts + deletes).
func (s *dbActorMgtService) CreateAll(ctx context.Context, cntMgtID uint) (int, error) {
	if cntMgtID == 0 {
		return 0, fmt.Errorf("invalid connection management ID: must be greater than 0")
	}

	logger.Debugf("Starting database user synchronization for cntmgt: %d", cntMgtID)
	tx := s.baseRepo.Begin()

	// Retrieve connection management configuration
	logger.Debugf("Looking up connection management record: %d", cntMgtID)
	cmt, err := s.cntmgtRepo.GetCntMgtByID(tx, cntMgtID)
	if err != nil {
		tx.Rollback()
		logger.Errorf("CntMgt id=%d not found: %v", cntMgtID, err)
		return 0, fmt.Errorf("cntmgt id=%d not found: %v", cntMgtID, err)
	}
	logger.Infof("Found connection management id=%d, type=%s", cmt.ID, cmt.CntType)

	// Retrieve endpoint configuration for VeloArtifact execution
	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("cannot find endpoint with id=%d: %v", cmt.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	// Route to database-specific synchronization logic
	insertCount := 0
	deleteCount := 0
	if strings.ToLower(cmt.CntType) == "mysql" {
		insertCount, deleteCount, err = s.processMySQLCreateAll(tx, cmt, ep)
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("MySQL user synchronization failed: %w", err)
		}
	} else if strings.ToLower(cmt.CntType) == "oracle" {
		insertCount, deleteCount, err = s.processOracleCreateAll(tx, cmt, ep)
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("Oracle user synchronization failed: %w", err)
		}
	} else {
		tx.Commit()
		return 0, fmt.Errorf("database type %s is not supported yet", cmt.CntType)
	}

	if err := tx.Commit().Error; err != nil {
		return 0, err
	}
	// Return total number of changes (inserts + deletes)
	return insertCount + deleteCount, nil
}

// Create adds a new database user on the remote server and registers it in local management.
// Executes SQL CREATE USER command via VeloArtifact and stores record in database.
// Returns created record with auto-generated ID.
func (s *dbActorMgtService) Create(ctx context.Context, data models.DBActorMgt) (*models.DBActorMgt, error) {
	if data.CntID == 0 {
		return nil, fmt.Errorf("invalid connection management ID: must be greater than 0")
	}

	tx := s.baseRepo.Begin()

	cmt, err := s.cntmgtRepo.GetCntMgtByID(tx, data.CntID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("cntmgt id=%d not found: %v", data.CntID, err)
	}
	logger.Infof("Found cntmgt id=%d, cnttype=%s", cmt.ID, cmt.CntType)

	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("cannot find endpoint with id=%d: %v", cmt.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	// Check duplicate before executing remote command
	countActor, err := s.dbActorMgtRepo.CountByCntIDAndDBUserAndIP(tx, cmt.ID, data.DBUser, data.IPAddress)
	if err != nil {
		logger.Errorf("Duplicate check error: %v", err)
		tx.Rollback()
		return nil, err
	}
	if countActor > 0 {
		tx.Rollback()
		return nil, fmt.Errorf("duplicate found for cntid=%d, dbuser=%s, ip_address=%s", cmt.ID, data.DBUser, data.IPAddress)
	}

	var finalSQL string
	cntTypeLower := strings.ToLower(cmt.CntType)

	if cntTypeLower == "oracle" {
		// Oracle CREATE USER with hardcoded SQL template
		finalSQL = fmt.Sprintf("CREATE USER %s IDENTIFIED BY \"%s\"", data.DBUser, data.Password)
		logger.Debugf("Oracle CREATE USER SQL: %s", finalSQL)
	} else {
		// MySQL and other databases use hex-encoded template from dbactor
		dbactor := s.actorAll[0]
		if dbactor.SQLCreate == "" {
			tx.Rollback()
			return nil, fmt.Errorf("dbactor.sql_create is empty for dbtype=%s", cmt.CntType)
		}
		sqlBytes, err := hex.DecodeString(dbactor.SQLCreate)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("unhex error: %v", err)
		}
		rawSQL := string(sqlBytes)
		logger.Debugf("Raw SQL from dbactor: %s", rawSQL)

		userHost := fmt.Sprintf("'%s'@'%s'", data.DBUser, data.IPAddress)
		finalSQL = strings.ReplaceAll(rawSQL, "'${dbactormgt.dbuser}'0x40'${dbactormgt.ip_address}'", userHost)
		finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.password}", data.Password)
	}
	logger.Debugf("Final SQL: %s", finalSQL)

	queryParamBuilder := dto.NewDBQueryParamBuilder().
		SetDBType(cntTypeLower).
		SetHost(cmt.IP).
		SetPort(cmt.Port).
		SetUser(cmt.Username).
		SetPassword(cmt.Password).
		SetQuery(finalSQL)

	// Oracle requires ServiceName as the database identifier for CDB/PDB routing
	if cntTypeLower == "oracle" {
		queryParamBuilder.SetDatabase(cmt.ServiceName)
	}

	queryParam := queryParamBuilder.Build()

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create agent command JSON for database user creation: %w", err)
	}
	logger.Debugf("Created agent command JSON payload (hex): %s", hexJSON)

	_, err = agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", false)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("executeSqlAgentAPI error: %v", err)
	}

	data.Status = "enabled"
	if err := s.dbActorMgtRepo.Create(tx, &data); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create database actor record: %w", err)
	}

	// Oracle: each user is also a schema, create corresponding DBMgt record
	if cntTypeLower == "oracle" {
		dbMgtRecord := models.DBMgt{
			CntID:  cmt.ID,
			DbName: data.DBUser,
			DbType: cmt.CntType,
			Status: "enabled",
		}
		if err := tx.Create(&dbMgtRecord).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create Oracle schema (DBMgt) record for %s: %w", data.DBUser, err)
		}
		logger.Infof("Created Oracle schema (DBMgt) record: %s (cntid=%d)", data.DBUser, cmt.ID)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return &data, nil
}

// Update modifies an existing database user on the remote server and in local management.
// Handles flexible updates: rename user, change password, update metadata, or combination.
// Executes appropriate SQL commands via VeloArtifact based on changed fields.
func (s *dbActorMgtService) Update(ctx context.Context, id uint, data dto.DBActorMgtUpdate) (*models.DBActorMgt, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid database actor ID: must be greater than 0")
	}

	tx := s.baseRepo.Begin()

	existing, err := s.dbActorMgtRepo.GetByID(tx, id)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("dbactormgt id=%d not found: %v", id, err)
	}

	cmt, err := s.cntmgtRepo.GetCntMgtByID(tx, existing.CntID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("cntmgt id=%d not found: %v", existing.CntID, err)
	}
	logger.Infof("Found connection management id=%d, type=%s", cmt.ID, cmt.CntType)

	cntTypeLower := strings.ToLower(cmt.CntType)

	if cntTypeLower == "mysql" {
		err = s.processMySQLUpdate(tx, existing, data, cmt)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("MySQL update failed: %w", err)
		}
	} else if cntTypeLower == "oracle" {
		err = s.processOracleUpdate(tx, existing, data, cmt)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("Oracle update failed: %w", err)
		}
	} else {
		tx.Rollback()
		return nil, fmt.Errorf("database type %s is not supported yet", cmt.CntType)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("transaction commit failed: %w", err)
	}

	return existing, nil
}

// processMySQLUpdate handles MySQL database user updates with flexible field changes
func (s *dbActorMgtService) processMySQLUpdate(tx interface{}, existing *models.DBActorMgt, data dto.DBActorMgtUpdate, cmt interface{}) error {
	// Get connection management details
	cntMgt, ok := cmt.(*models.CntMgt)
	if !ok {
		return fmt.Errorf("invalid connection management type")
	}

	// Get SQL template for updates
	dbactor := s.actorAll[0]
	if dbactor.SQLUpdate == "" {
		return fmt.Errorf("dbactor.sql_update is empty for MySQL")
	}

	// Decode hex-encoded SQL template for security compliance
	sqlBytes, err := hex.DecodeString(dbactor.SQLUpdate)
	if err != nil {
		return fmt.Errorf("failed to decode SQL template: %w", err)
	}

	rawSQL := string(sqlBytes)
	logger.Debugf("Raw SQL template: %s", rawSQL)

	// Determine update strategy based on changed fields
	needsRename := (data.DBUser != "" && data.DBUser != existing.DBUser) ||
		(data.IPAddress != "" && data.IPAddress != existing.IPAddress)
	needsPasswordChange := data.Password != "" && data.Password != existing.Password

	// Build SQL command based on changes needed
	var finalSQL string
	if needsRename && needsPasswordChange {
		// Both rename and password change needed
		finalSQL = s.buildMySQLRenameAndPasswordSQL(rawSQL, existing, data)
	} else if needsRename {
		// Only rename needed (remove password change part)
		finalSQL = s.buildMySQLRenameSQL(rawSQL, existing, data)
	} else if needsPasswordChange {
		// Only password change needed (remove rename part)
		finalSQL = s.buildMySQLPasswordSQL(rawSQL, existing, data)
	} else {
		// No critical changes, only update local metadata
		return s.updateLocalMetadata(tx, existing, data)
	}

	logger.Debugf("Final SQL for execution: %s", finalSQL)

	// Execute SQL via VeloArtifact
	err = s.executeMySQLUpdate(cntMgt, finalSQL)
	if err != nil {
		return fmt.Errorf("failed to execute MySQL update: %w", err)
	}

	// Update local database record with new values
	return s.updateDatabaseRecord(tx, existing, data)
}

// buildMySQLRenameAndPasswordSQL constructs SQL for both user rename and password change
func (s *dbActorMgtService) buildMySQLRenameAndPasswordSQL(rawSQL string, existing *models.DBActorMgt, data dto.DBActorMgtUpdate) string {
	// Use original user@host for the FROM clause (after RENAME USER)
	oldUserHost := fmt.Sprintf("'%s'@'%s'", existing.DBUser, existing.IPAddress)

	// Use new user@host for the TO clause (after TO keyword)
	newUser := data.DBUser
	if newUser == "" {
		newUser = existing.DBUser
	}
	newIP := data.IPAddress
	if newIP == "" {
		newIP = existing.IPAddress
	}
	newUserHost := fmt.Sprintf("'%s'@'%s'", newUser, newIP)

	// Template structure: RENAME USER '${old}' TO '${new}'; ALTER USER '${new}' IDENTIFIED BY '${password}'
	// Context-aware replacement ensures correct substitution in multi-command SQL
	renamePattern := "RENAME USER '${dbactormgt.dbuser}'0x40'${dbactormgt.ip_address}'"
	toPattern := " TO '${dbactormgt.dbuser}'0x40'${dbactormgt.ip_address}'"
	alterPattern := "ALTER USER '${dbactormgt.dbuser}'0x40'${dbactormgt.ip_address}'"

	sql := rawSQL

	// Replace RENAME USER pattern with old user@host
	sql = strings.Replace(sql, renamePattern, fmt.Sprintf("RENAME USER %s", oldUserHost), 1)

	// Replace TO pattern with new user@host
	sql = strings.Replace(sql, toPattern, fmt.Sprintf(" TO %s", newUserHost), 1)

	// Replace ALTER USER pattern with new user@host (after rename)
	sql = strings.Replace(sql, alterPattern, fmt.Sprintf("ALTER USER %s", newUserHost), 1)

	// Replace password placeholder
	sql = strings.ReplaceAll(sql, "${dbactormgt.password}", data.Password)

	// Clean up any extra whitespace or tab characters
	return strings.TrimSpace(sql)
}

// buildMySQLRenameSQL constructs SQL for user rename only.
// Extracts RENAME USER command from template, removing password change portion.
func (s *dbActorMgtService) buildMySQLRenameSQL(rawSQL string, existing *models.DBActorMgt, data dto.DBActorMgtUpdate) string {
	sqlParts := strings.Split(rawSQL, ";")
	if len(sqlParts) == 0 {
		return ""
	}

	renameSQL := strings.TrimSpace(sqlParts[0])

	// Use original user@host for the FROM clause
	oldUserHost := fmt.Sprintf("'%s'@'%s'", existing.DBUser, existing.IPAddress)

	// Use new user@host for the TO clause
	newUser := data.DBUser
	if newUser == "" {
		newUser = existing.DBUser
	}
	newIP := data.IPAddress
	if newIP == "" {
		newIP = existing.IPAddress
	}
	newUserHost := fmt.Sprintf("'%s'@'%s'", newUser, newIP)

	// Use context-aware replacement patterns to distinguish positions
	renamePattern := "RENAME USER '${dbactormgt.dbuser}'0x40'${dbactormgt.ip_address}'"
	toPattern := " TO '${dbactormgt.dbuser}'0x40'${dbactormgt.ip_address}'"

	// Replace RENAME USER pattern with old user@host
	renameSQL = strings.Replace(renameSQL, renamePattern, fmt.Sprintf("RENAME USER %s", oldUserHost), 1)

	// Replace TO pattern with new user@host
	renameSQL = strings.Replace(renameSQL, toPattern, fmt.Sprintf(" TO %s", newUserHost), 1)

	// Clean up any extra whitespace or tab characters
	return strings.TrimSpace(renameSQL) + ";"
}

// buildMySQLPasswordSQL constructs SQL for password change only.
// Extracts ALTER USER command from template, removing RENAME USER portion.
// Template structure: RENAME USER '${old}' TO '${new}'; ALTER USER '${new}' IDENTIFIED BY '${password}'
func (s *dbActorMgtService) buildMySQLPasswordSQL(rawSQL string, existing *models.DBActorMgt, data dto.DBActorMgtUpdate) string {
	sqlParts := strings.Split(rawSQL, ";")
	if len(sqlParts) < 2 {
		logger.Warnf("Invalid SQL template structure: expected RENAME and ALTER parts")
		return ""
	}

	alterSQL := strings.TrimSpace(sqlParts[1])

	// Validate extracted SQL is ALTER USER command
	if !strings.HasPrefix(strings.ToUpper(alterSQL), "ALTER USER") {
		logger.Warnf("Second SQL part is not ALTER USER command: %s", alterSQL)
		return ""
	}

	currentUserHost := fmt.Sprintf("'%s'@'%s'", existing.DBUser, existing.IPAddress)

	alterSQL = strings.ReplaceAll(alterSQL, "'${dbactormgt.dbuser}'0x40'${dbactormgt.ip_address}'", currentUserHost)
	alterSQL = strings.ReplaceAll(alterSQL, "${dbactormgt.password}", data.Password)

	logger.Debugf("Generated password-only SQL command: %s", alterSQL)

	// Clean up any extra whitespace or tab characters
	return strings.TrimSpace(alterSQL) + ";"
}

// executeMySQLUpdate executes SQL update command via VeloArtifact.
// Used by Update operation to execute RENAME USER and/or ALTER USER commands.
func (s *dbActorMgtService) executeMySQLUpdate(cntMgt *models.CntMgt, finalSQL string) error {
	ep, err := s.endpointRepo.GetByID(nil, utils.MustIntToUint(cntMgt.Agent))
	if err != nil {
		return fmt.Errorf("endpoint id=%d not found: %w", cntMgt.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	queryParam := dto.NewDBQueryParamBuilder().
		SetDBType("mysql").
		SetHost(cntMgt.IP).
		SetPort(cntMgt.Port).
		SetUser(cntMgt.Username).
		SetPassword(cntMgt.Password).
		SetQuery(finalSQL).
		Build()

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		return fmt.Errorf("failed to create agent command JSON: %w", err)
	}
	logger.Debugf("Created agent command JSON payload (hex): %s", hexJSON)

	_, err = agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", false)
	if err != nil {
		return fmt.Errorf("executeSqlAgentAPI failed: %w", err)
	}

	return nil
}

// processOracleUpdate handles Oracle database user update (password change only).
// Executes ALTER USER via VeloArtifact, then updates local record.
func (s *dbActorMgtService) processOracleUpdate(tx interface{}, existing *models.DBActorMgt, data dto.DBActorMgtUpdate, cmt interface{}) error {
	cntMgt, ok := cmt.(*models.CntMgt)
	if !ok {
		return fmt.Errorf("invalid connection management type")
	}

	needsPasswordChange := data.Password != "" && data.Password != existing.Password
	if !needsPasswordChange {
		return s.updateLocalMetadata(tx, existing, data)
	}

	// Oracle ALTER USER: password change only
	finalSQL := fmt.Sprintf("ALTER USER %s IDENTIFIED BY \"%s\"", existing.DBUser, data.Password)
	logger.Debugf("Oracle ALTER USER SQL: %s", finalSQL)

	ep, err := s.endpointRepo.GetByID(nil, utils.MustIntToUint(cntMgt.Agent))
	if err != nil {
		return fmt.Errorf("endpoint id=%d not found: %w", cntMgt.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	queryParam := dto.NewDBQueryParamBuilder().
		SetDBType("oracle").
		SetHost(cntMgt.IP).
		SetPort(cntMgt.Port).
		SetUser(cntMgt.Username).
		SetPassword(cntMgt.Password).
		SetDatabase(cntMgt.ServiceName).
		SetQuery(finalSQL).
		Build()

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		return fmt.Errorf("failed to create agent command JSON for Oracle user update: %w", err)
	}
	logger.Debugf("Created agent command JSON payload (hex): %s", hexJSON)

	_, err = agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", false)
	if err != nil {
		return fmt.Errorf("executeSqlAgentAPI failed: %w", err)
	}

	return s.updateDatabaseRecord(tx, existing, data)
}

// updateLocalMetadata updates metadata fields that don't require remote SQL execution.
// Used when only status, description, or other non-critical fields change.
func (s *dbActorMgtService) updateLocalMetadata(tx interface{}, existing *models.DBActorMgt, data dto.DBActorMgtUpdate) error {
	updated := false

	if data.DBClient != "" && data.DBClient != existing.DBClient {
		existing.DBClient = data.DBClient
		updated = true
	}
	if data.OSUser != "" && data.OSUser != existing.OSUser {
		existing.OSUser = data.OSUser
		updated = true
	}
	if data.Description != "" && data.Description != existing.Description {
		existing.Description = data.Description
		updated = true
	}
	if data.Status != "" && data.Status != existing.Status {
		existing.Status = data.Status
		updated = true
	}

	if updated {
		if err := tx.(*gorm.DB).Save(existing).Error; err != nil {
			return fmt.Errorf("failed to update local metadata: %w", err)
		}
	}

	return nil
}

// updateDatabaseRecord updates local record after successful remote SQL execution.
// Applies all changes including critical fields (user, IP, password) and metadata.
func (s *dbActorMgtService) updateDatabaseRecord(tx interface{}, existing *models.DBActorMgt, data dto.DBActorMgtUpdate) error {
	if data.DBUser != "" {
		existing.DBUser = data.DBUser
	}
	if data.IPAddress != "" {
		existing.IPAddress = data.IPAddress
	}
	if data.Password != "" {
		existing.Password = data.Password
	}

	// Update metadata fields
	if data.DBClient != "" {
		existing.DBClient = data.DBClient
	}
	if data.OSUser != "" {
		existing.OSUser = data.OSUser
	}
	if data.Description != "" {
		existing.Description = data.Description
	}
	if data.Status != "" {
		existing.Status = data.Status
	}

	if err := tx.(*gorm.DB).Save(existing).Error; err != nil {
		return fmt.Errorf("failed to update database record: %w", err)
	}

	logger.Infof("Successfully updated DBActorMgt id=%d", existing.ID)
	return nil
}

// Delete removes a database user from remote server and local management system.
// Executes SQL DROP USER command via VeloArtifact and deletes local record.
func (s *dbActorMgtService) Delete(ctx context.Context, id uint) error {
	if id == 0 {
		return fmt.Errorf("invalid database actor ID: must be greater than 0")
	}

	tx := s.baseRepo.Begin()

	existing, err := s.dbActorMgtRepo.GetByID(tx, id)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("dbactormgt id=%d not found: %v", id, err)
	}

	cmt, err := s.cntmgtRepo.GetCntMgtByID(tx, existing.CntID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("cntmgt id=%d not found: %v", existing.CntID, err)
	}
	logger.Infof("Found connection management id=%d, type=%s", cmt.ID, cmt.CntType)

	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("endpoint id=%d not found: %v", cmt.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	var finalSQL string
	cntTypeLower := strings.ToLower(cmt.CntType)

	if cntTypeLower == "oracle" {
		// Oracle DROP USER with CASCADE to remove all schema objects
		finalSQL = fmt.Sprintf("DROP USER %s CASCADE", existing.DBUser)
		logger.Debugf("Oracle DROP USER SQL: %s", finalSQL)
	} else {
		// MySQL and other databases use hex-encoded template from dbactor
		dbactor := s.actorAll[0]
		if dbactor.SQLDelete == "" {
			tx.Rollback()
			return fmt.Errorf("dbactor.sql_delete is empty for dbtype=%s", cmt.CntType)
		}
		sqlBytes, err := hex.DecodeString(dbactor.SQLDelete)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("unhex error: %v", err)
		}
		rawSQL := string(sqlBytes)
		userHost := fmt.Sprintf("'%s'@'%s'", existing.DBUser, existing.IPAddress)
		finalSQL = strings.ReplaceAll(rawSQL, "'${dbactormgt.dbuser}'0x40'${dbactormgt.ip_address}'", userHost)
	}
	logger.Debugf("Final SQL: %s", finalSQL)

	queryParamBuilder := dto.NewDBQueryParamBuilder().
		SetDBType(cntTypeLower).
		SetHost(cmt.IP).
		SetPort(cmt.Port).
		SetUser(cmt.Username).
		SetPassword(cmt.Password).
		SetQuery(finalSQL)

	// Oracle requires ServiceName as the database identifier for CDB/PDB routing
	if cntTypeLower == "oracle" {
		queryParamBuilder.SetDatabase(cmt.ServiceName)
	}

	queryParam := queryParamBuilder.Build()

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create agent command JSON for database user deletion: %w", err)
	}
	logger.Debugf("Created agent command JSON payload (hex): %s", hexJSON)

	_, err = agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", false)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("executeSqlAgentAPI error: %v", err)
	}

	if err := tx.Delete(&models.DBActorMgt{}, id).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete database user record with id=%d: %w", id, err)
	}

	// Oracle: each user is also a schema, delete corresponding DBMgt record
	if cntTypeLower == "oracle" {
		if err := tx.Where("cnt_id = ? AND db_name = ?", cmt.ID, existing.DBUser).Delete(&models.DBMgt{}).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to delete Oracle schema (DBMgt) record for %s: %w", existing.DBUser, err)
		}
		logger.Infof("Deleted Oracle schema (DBMgt) record: %s (cntid=%d)", existing.DBUser, cmt.ID)
	}

	if err := tx.Commit().Error; err != nil {
		return err
	}
	return nil
}

// processMySQLCreateAll handles MySQL-specific user synchronization.
// Decodes hex-encoded SQL template, executes against remote server,
// then performs two-phase sync of user@host records.
// Returns (insertCount, deleteCount, error).
func (s *dbActorMgtService) processMySQLCreateAll(tx *gorm.DB, cmt *models.CntMgt, ep *models.Endpoint) (int, int, error) {
	// Retrieve SQL template for fetching database users
	// Template is hex-encoded to prevent SQL injection during storage
	dbactor := s.actorAll[0]
	if dbactor.SQLGet == "" {
		return 0, 0, fmt.Errorf("dbactor.sql_get is empty for dbtype=%s", cmt.CntType)
	}

	// Decode hex-encoded SQL template for security compliance
	sqlBytes, err := hex.DecodeString(dbactor.SQLGet)
	if err != nil {
		return 0, 0, fmt.Errorf("unhex error: %v", err)
	}
	rawSQL := string(sqlBytes)
	logger.Debugf("Decoded SQL template: %s", rawSQL)

	// Perform variable substitution for connection-specific values
	finalSQL := strings.ReplaceAll(rawSQL, "${cntmgt.ip}", cmt.IP)
	finalSQL = strings.ReplaceAll(finalSQL, "${cntmgt.username}", cmt.Username)

	queryParam := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cmt.CntType)).
		SetHost(cmt.IP).
		SetPort(cmt.Port).
		SetUser(cmt.Username).
		SetPassword(cmt.Password).
		SetQuery(finalSQL).
		Build()

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create agent command JSON for cntmgt %d: %w", cmt.ID, err)
	}
	logger.Debugf("Created agent command JSON payload (hex): %s", hexJSON)

	stdout, err := agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", true)
	if err != nil {
		return 0, 0, fmt.Errorf("executeSqlAgentAPI error: %v", err)
	}

	type SQLResponse struct {
		Message  string     `json:"message"`
		Results  [][]string `json:"results"`
		RowCount int        `json:"row_count"`
		Success  bool       `json:"success"`
	}

	var sqlResp SQLResponse
	if err := json.Unmarshal([]byte(stdout), &sqlResp); err != nil {
		return 0, 0, fmt.Errorf("failed to parse SQL response JSON: %w", err)
	}

	if !sqlResp.Success {
		return 0, 0, fmt.Errorf("SQL execution failed: %s", sqlResp.Message)
	}

	logger.Infof("SQL execution successful: %s, found %d database users", sqlResp.Message, sqlResp.RowCount)

	// Phase 1: Collect remote database users
	remoteUsers := make(map[string]bool) // key: "user@host"
	for _, result := range sqlResp.Results {
		if len(result) == 0 {
			continue
		}

		userHost := strings.TrimSpace(result[0])
		if userHost == "" {
			continue
		}

		// Parse user@host format from MySQL result
		parts := strings.SplitN(userHost, "@", 2)
		if len(parts) < 2 {
			logger.Warnf("Invalid user@host format '%s', skipping", userHost)
			continue
		}

		dbUser := parts[0]
		ipAddress := parts[1]

		// Skip system MySQL users to prevent management of critical accounts
		if config.IsSystemUser(dbUser) {
			logger.Debugf("Skipping system MySQL user: %s", dbUser)
			continue
		}

		userKey := fmt.Sprintf("%s@%s", dbUser, ipAddress)
		remoteUsers[userKey] = true
	}

	// Get existing local database users for this connection
	existingUsers, err := s.dbActorMgtRepo.GetAllByCntID(tx, cmt.ID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get existing database users: %w", err)
	}

	localUsers := make(map[string]*models.DBActorMgt)
	for _, user := range existingUsers {
		userKey := fmt.Sprintf("%s@%s", user.DBUser, user.IPAddress)
		localUsers[userKey] = user
	}

	insertCount := 0
	deleteCount := 0

	// Phase 2a: INSERT missing database users
	for userKey := range remoteUsers {
		if _, exists := localUsers[userKey]; !exists {
			parts := strings.SplitN(userKey, "@", 2)
			if len(parts) != 2 {
				continue
			}

			actor := models.DBActorMgt{
				CntID:       cmt.ID,
				DBUser:      parts[0],
				IPAddress:   parts[1],
				Description: "Auto-collected from V2-DBF Agent",
				Status:      "enabled",
			}

			if err := s.dbActorMgtRepo.Create(tx, &actor); err != nil {
				return 0, 0, fmt.Errorf("failed to create database user record for %s: %w", userKey, err)
			}

			insertCount++
			logger.Infof("Created new database user record: %s", userKey)
		}
	}

	// Phase 2b: DELETE obsolete database users
	for userKey, localUser := range localUsers {
		if !remoteUsers[userKey] {
			if err := tx.Delete(localUser).Error; err != nil {
				return 0, 0, fmt.Errorf("failed to delete obsolete database user record for %s: %w", userKey, err)
			}
			deleteCount++
			logger.Infof("Deleted obsolete database user record: %s", userKey)
		}
	}

	logger.Infof("MySQL user synchronization completed - Inserted: %d, Deleted: %d", insertCount, deleteCount)
	return insertCount, deleteCount, nil
}

// processOracleCreateAll handles Oracle-specific user synchronization.
// Determines CDB vs PDB based on ServiceName and ParentConnectionID,
// then collects users and syncs both actors and schemas (DBMgt).
// CDB: ServiceName != "" AND ParentConnectionID == nil
// PDB: ParentConnectionID != nil
// Returns (totalInserts, totalDeletes, error).
func (s *dbActorMgtService) processOracleCreateAll(tx *gorm.DB, cmt *models.CntMgt, ep *models.Endpoint) (int, int, error) {
	totalInserts := 0
	totalDeletes := 0

	isCDB := cmt.ServiceName != "" && cmt.ParentConnectionID == nil

	if isCDB {
		logger.Infof("Processing Oracle CDB user synchronization for cntmgt id=%d", cmt.ID)

		// Phase 1: Collect CDB common users (shared across all containers)
		cdbUsernames, err := s.executeOracleUserQuery(ep.ClientID, ep.OsType, cmt, oracleCDBUsersSQL)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to collect CDB common users: %w", err)
		}
		logger.Infof("Found %d CDB common users", len(cdbUsernames))

		// Sync CDB actors and schemas
		ins, del, err := s.syncOracleActors(tx, cmt.ID, cdbUsernames)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to sync CDB actors: %w", err)
		}
		totalInserts += ins
		totalDeletes += del

		dbIns, dbDel, err := s.syncOracleDBMgt(tx, cmt.ID, cmt.CntType, cdbUsernames)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to sync CDB schemas: %w", err)
		}
		totalInserts += dbIns
		totalDeletes += dbDel

		// Phase 2: Collect PDB-local users for each child PDB
		pdbs, err := s.cntmgtRepo.GetByParentConnectionID(tx, cmt.ID)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to get PDB list for CDB %d: %w", cmt.ID, err)
		}
		logger.Infof("Found %d PDBs under CDB id=%d", len(pdbs), cmt.ID)

		for i := range pdbs {
			pdb := &pdbs[i]
			logger.Infof("Processing PDB: %s (id=%d)", pdb.CntName, pdb.ID)

			pdbSQL := strings.ReplaceAll(oraclePDBUsersSQL, "${pdbname}", pdb.CntName)
			pdbUsernames, err := s.executeOracleUserQuery(ep.ClientID, ep.OsType, cmt, pdbSQL)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to collect users for PDB %s: %w", pdb.CntName, err)
			}
			logger.Infof("Found %d users for PDB %s", len(pdbUsernames), pdb.CntName)

			ins, del, err := s.syncOracleActors(tx, pdb.ID, pdbUsernames)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to sync actors for PDB %s: %w", pdb.CntName, err)
			}
			totalInserts += ins
			totalDeletes += del

			dbIns, dbDel, err := s.syncOracleDBMgt(tx, pdb.ID, pdb.CntType, pdbUsernames)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to sync schemas for PDB %s: %w", pdb.CntName, err)
			}
			totalInserts += dbIns
			totalDeletes += dbDel
		}
	} else if cmt.ParentConnectionID != nil {
		// PDB input: get parent CDB for connection details
		logger.Infof("Processing Oracle PDB user synchronization for cntmgt id=%d", cmt.ID)

		cdb, err := s.cntmgtRepo.GetCntMgtByID(tx, *cmt.ParentConnectionID)
		if err != nil {
			return 0, 0, fmt.Errorf("parent CDB cntmgt id=%d not found: %w", *cmt.ParentConnectionID, err)
		}

		pdbSQL := strings.ReplaceAll(oraclePDBUsersSQL, "${pdbname}", cmt.CntName)
		// All Oracle queries connect to CDB using its ServiceName
		pdbUsernames, err := s.executeOracleUserQuery(ep.ClientID, ep.OsType, cdb, pdbSQL)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to collect users for PDB %s: %w", cmt.CntName, err)
		}
		logger.Infof("Found %d users for PDB %s", len(pdbUsernames), cmt.CntName)

		ins, del, err := s.syncOracleActors(tx, cmt.ID, pdbUsernames)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to sync actors for PDB %s: %w", cmt.CntName, err)
		}
		totalInserts += ins
		totalDeletes += del

		dbIns, dbDel, err := s.syncOracleDBMgt(tx, cmt.ID, cmt.CntType, pdbUsernames)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to sync schemas for PDB %s: %w", cmt.CntName, err)
		}
		totalInserts += dbIns
		totalDeletes += dbDel
	} else {
		return 0, 0, fmt.Errorf("Oracle connection id=%d is not a valid CDB or PDB: service_name=%q, parent_connection_id=%v",
			cmt.ID, cmt.ServiceName, cmt.ParentConnectionID)
	}

	logger.Infof("Oracle user synchronization completed - Total Inserted: %d, Total Deleted: %d", totalInserts, totalDeletes)
	return totalInserts, totalDeletes, nil
}

// executeOracleUserQuery executes a user query against an Oracle CDB and returns usernames.
// Always connects using the CDB's ServiceName since CDB_USERS and V$CONTAINERS are CDB-level views.
// Returns a slice of usernames found on the remote server.
func (s *dbActorMgtService) executeOracleUserQuery(clientID, osType string, cdb *models.CntMgt, sql string) ([]string, error) {
	queryParam := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cdb.CntType)).
		SetHost(cdb.IP).
		SetPort(cdb.Port).
		SetUser(cdb.Username).
		SetPassword(cdb.Password).
		SetDatabase(cdb.ServiceName).
		SetQuery(sql).
		Build()

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent command JSON: %w", err)
	}
	logger.Debugf("Oracle user query hex payload: %s", hexJSON)

	stdout, err := agent.ExecuteSqlAgentAPI(clientID, osType, "execute", hexJSON, "", true)
	if err != nil {
		return nil, fmt.Errorf("executeSqlAgentAPI error: %w", err)
	}

	type SQLResponse struct {
		Message  string     `json:"message"`
		Results  [][]string `json:"results"`
		RowCount int        `json:"row_count"`
		Success  bool       `json:"success"`
	}

	var sqlResp SQLResponse
	if err := json.Unmarshal([]byte(stdout), &sqlResp); err != nil {
		return nil, fmt.Errorf("failed to parse SQL response JSON: %w", err)
	}

	if !sqlResp.Success {
		return nil, fmt.Errorf("SQL execution failed: %s", sqlResp.Message)
	}

	logger.Infof("Oracle SQL execution successful: %s, found %d users", sqlResp.Message, sqlResp.RowCount)

	var usernames []string
	for _, result := range sqlResp.Results {
		if len(result) == 0 {
			continue
		}
		username := strings.TrimSpace(result[0])
		if username == "" {
			continue
		}

		if config.IsSystemUser(username) {
			logger.Debugf("Skipping system Oracle user: %s", username)
			continue
		}

		usernames = append(usernames, username)
	}

	return usernames, nil
}

// syncOracleActors performs two-phase sync of Oracle actor records for a connection.
// Oracle uses username only (no host concept), so IPAddress is left empty.
// Phase 1: INSERT missing actors. Phase 2: DELETE obsolete actors.
// Returns (insertCount, deleteCount, error).
func (s *dbActorMgtService) syncOracleActors(tx *gorm.DB, cntID uint, usernames []string) (int, int, error) {
	// Build set of remote usernames for O(1) lookup
	remoteUsers := make(map[string]bool)
	for _, u := range usernames {
		remoteUsers[u] = true
	}

	existingActors, err := s.dbActorMgtRepo.GetAllByCntID(tx, cntID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get existing actors for cntid %d: %w", cntID, err)
	}

	// Oracle actors keyed by username only (no host concept)
	localActors := make(map[string]*models.DBActorMgt)
	for _, actor := range existingActors {
		localActors[actor.DBUser] = actor
	}

	insertCount := 0
	deleteCount := 0

	// Phase 1: INSERT missing actors
	for _, username := range usernames {
		if _, exists := localActors[username]; !exists {
			actor := models.DBActorMgt{
				CntID:       cntID,
				DBUser:      username,
				IPAddress:   "%",
				Description: "Auto-collected from V2-DBF Agent",
				Status:      "enabled",
			}
			if err := s.dbActorMgtRepo.Create(tx, &actor); err != nil {
				return 0, 0, fmt.Errorf("failed to create Oracle actor record for %s: %w", username, err)
			}
			insertCount++
			logger.Infof("Created Oracle actor record: %s (cntid=%d)", username, cntID)
		}
	}

	// Phase 2: DELETE obsolete actors
	for username, localActor := range localActors {
		if !remoteUsers[username] {
			if err := tx.Delete(localActor).Error; err != nil {
				return 0, 0, fmt.Errorf("failed to delete obsolete Oracle actor record for %s: %w", username, err)
			}
			deleteCount++
			logger.Infof("Deleted obsolete Oracle actor record: %s (cntid=%d)", username, cntID)
		}
	}

	logger.Infof("Oracle actor sync for cntid=%d - Inserted: %d, Deleted: %d", cntID, insertCount, deleteCount)
	return insertCount, deleteCount, nil
}

// syncOracleDBMgt performs two-phase sync of DBMgt records for Oracle connections.
// In Oracle, each user is also a schema (database), so user discovery must also
// create/remove corresponding DBMgt records to maintain schema inventory.
// Returns (insertCount, deleteCount, error).
func (s *dbActorMgtService) syncOracleDBMgt(tx *gorm.DB, cntID uint, cntType string, usernames []string) (int, int, error) {
	// Build set of remote schemas (usernames) for O(1) lookup
	remoteSchemas := make(map[string]bool)
	for _, u := range usernames {
		remoteSchemas[u] = true
	}

	existingDBs, err := s.dbMgtRepo.GetAllByCntIDAndDBType(tx, cntID, cntType)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get existing databases for cntid %d: %w", cntID, err)
	}

	localDBs := make(map[string]*models.DBMgt)
	for _, db := range existingDBs {
		localDBs[db.DbName] = db
	}

	insertCount := 0
	deleteCount := 0

	// Phase 1: INSERT missing schema records
	for _, username := range usernames {
		if _, exists := localDBs[username]; !exists {
			record := models.DBMgt{
				CntID:  cntID,
				DbName: username,
				DbType: cntType,
				Status: "enabled",
			}
			if err := tx.Create(&record).Error; err != nil {
				return 0, 0, fmt.Errorf("failed to create Oracle schema record for %s: %w", username, err)
			}
			insertCount++
			logger.Infof("Created Oracle schema (DBMgt) record: %s (cntid=%d)", username, cntID)
		}
	}

	// Phase 2: DELETE obsolete schema records
	for dbName, localDB := range localDBs {
		if !remoteSchemas[dbName] {
			if err := tx.Delete(localDB).Error; err != nil {
				return 0, 0, fmt.Errorf("failed to delete obsolete Oracle schema record for %s: %w", dbName, err)
			}
			deleteCount++
			logger.Infof("Deleted obsolete Oracle schema (DBMgt) record: %s (cntid=%d)", dbName, cntID)
		}
	}

	logger.Infof("Oracle schema sync for cntid=%d - Inserted: %d, Deleted: %d", cntID, insertCount, deleteCount)
	return insertCount, deleteCount, nil
}
