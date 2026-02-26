package services

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"dbfartifactapi/bootstrap"
	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services/agent"
	"dbfartifactapi/services/dto"
	"dbfartifactapi/utils"
)

// DBMgtService provides business logic for database management operations.
// All methods accept context.Context for cancellation and timeout control.
type DBMgtService interface {
	// CreateAll synchronizes local database records with remote databases for a connection.
	// Returns the total number of changes (inserts + deletes) made during synchronization.
	CreateAll(ctx context.Context, cntMgtID uint) (int, error)

	// Create creates a new database on the remote server and registers it locally.
	// Returns ErrDuplicateDatabase if database already exists for this connection.
	Create(ctx context.Context, data models.DBMgt) (*models.DBMgt, error)

	// Delete drops the database from remote server and removes local record.
	// Returns ErrNotFound if database record doesn't exist.
	Delete(ctx context.Context, id uint) error
}

type dbMgtService struct {
	baseRepo     repository.BaseRepository
	dbMgtRepo    repository.DBMgtRepository
	dbTypeRepo   repository.DBTypeRepository
	cntMgtRepo   repository.CntMgtRepository
	endpointRepo repository.EndpointRepository
	dbTypeAll    []models.DBType
}

// NewDBMgtService creates a new database management service instance.
func NewDBMgtService() DBMgtService {
	return &dbMgtService{
		baseRepo:     repository.NewBaseRepository(),
		dbMgtRepo:    repository.NewDBMgtRepository(),
		dbTypeRepo:   repository.NewDBTypeRepository(),
		cntMgtRepo:   repository.NewCntMgtRepository(),
		endpointRepo: repository.NewEndpointRepository(),
		dbTypeAll:    bootstrap.DBTypeAll,
	}
}

// NewDBMgtServiceWithDeps creates a service instance with injected dependencies.
// Used for testing to provide mock implementations of repositories.
func NewDBMgtServiceWithDeps(
	baseRepo repository.BaseRepository,
	dbMgtRepo repository.DBMgtRepository,
	cntMgtRepo repository.CntMgtRepository,
	endpointRepo repository.EndpointRepository,
	dbTypeAll []models.DBType,
) DBMgtService {
	return &dbMgtService{
		baseRepo:     baseRepo,
		dbMgtRepo:    dbMgtRepo,
		cntMgtRepo:   cntMgtRepo,
		endpointRepo: endpointRepo,
		dbTypeAll:    dbTypeAll,
	}
}

// CreateAll synchronizes local database records with remote database instances.
// Queries remote database server for all existing databases, then performs two-phase sync:
// Phase 1: INSERT new databases found remotely but not locally
// Phase 2: DELETE obsolete databases existing locally but not remotely
// Returns total number of changes (inserts + deletes) to maintain audit trail.
func (s *dbMgtService) CreateAll(ctx context.Context, cntMgtID uint) (int, error) {
	tx := s.baseRepo.Begin()

	// Connection lookup must succeed for security audit trail
	cmt, err := s.cntMgtRepo.GetCntMgtByID(tx, cntMgtID)
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("cntmgt id=%d not found: %v", cntMgtID, err)
	}
	logger.Infof("Found cntmgt id=%d, cnttype=%s", cmt.ID, cmt.CntType)

	// SQL templates are hex-encoded to prevent injection during storage
	// Must be decoded at execution time only for security compliance
	dbType := s.dbTypeAll[0]
	if dbType.SqlGet == "" {
		tx.Rollback()
		return 0, fmt.Errorf("dbtype.sql_get is empty for dbtype=%s", cmt.CntType)
	}

	sqlBytes, err := hex.DecodeString(dbType.SqlGet)
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("unhex error: %v", err)
	}

	finalSQL := string(sqlBytes)
	logger.Debugf("Raw SQL from dbtype: %s", finalSQL)

	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("cannot find endpoint with id=%d: %v", cmt.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	clientID := ep.ClientID

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
		tx.Rollback()
		return 0, fmt.Errorf("failed to create agent command JSON for cntmgt %d: %w", cntMgtID, err)
	}
	logger.Debugf("Created agent command JSON payload (hex): %s", hexJSON)

	stdout, err := agent.ExecuteSqlAgentAPI(clientID, ep.OsType, "execute", hexJSON, "", true)
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("executeSqlAgentAPI error: %v", err)
	}

	// Two-phase sync required to maintain referential integrity
	// Cannot delete-then-insert as downstream policies may reference databases
	insertCount := 0
	deleteCount := 0
	if strings.ToLower(cmt.CntType) == "mysql" {
		// VeloArtifact returns JSON with results array containing database names
		type SQLResponse struct {
			Message  string     `json:"message"`
			Results  [][]string `json:"results"`
			RowCount int        `json:"row_count"`
			Success  bool       `json:"success"`
		}

		var sqlResp SQLResponse
		if err := json.Unmarshal([]byte(stdout), &sqlResp); err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("failed to parse SQL response JSON: %w", err)
		}

		if !sqlResp.Success {
			tx.Rollback()
			return 0, fmt.Errorf("SQL execution failed: %s", sqlResp.Message)
		}

		logger.Infof("SQL execution successful: %s, found %d databases", sqlResp.Message, sqlResp.RowCount)

		// Phase 1: Build map of remote databases for O(1) lookup during sync
		remoteDatabases := make(map[string]bool)
		for _, result := range sqlResp.Results {
			if len(result) == 0 {
				continue
			}

			dbName := strings.TrimSpace(result[0])
			if dbName == "" {
				continue
			}

			// TODO: Add system database filtering when security policy is defined
			// Currently accepting all databases to avoid missing user databases
			// if config.IsSystemDatabase(dbName) {
			// 	logger.Debugf("Skipping system database: %s", dbName)
			// 	continue
			// }

			remoteDatabases[dbName] = true
		}

		// Get existing local databases for this connection
		existingDBs, err := s.dbMgtRepo.GetAllByCntIDAndDBType(tx, cmt.ID, cmt.CntType)
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("failed to get existing databases: %w", err)
		}

		// Build index of local databases for efficient lookup
		localDatabases := make(map[string]*models.DBMgt)
		for _, db := range existingDBs {
			localDatabases[db.DbName] = db
		}

		// Phase 2a: Add new databases to maintain complete inventory
		// Missing databases may impact security policy enforcement
		for dbName := range remoteDatabases {
			if _, exists := localDatabases[dbName]; !exists {
				record := models.DBMgt{
					CntID:  cmt.ID,
					DbName: dbName,
					DbType: cmt.CntType,
					Status: "enabled",
				}
				if err := tx.Create(&record).Error; err != nil {
					tx.Rollback()
					return 0, fmt.Errorf("failed to create database record for %s: %w", dbName, err)
				}
				insertCount++
				logger.Infof("Created new database record: %s", dbName)
			}
		}

		// Phase 2b: Remove stale references to maintain data consistency
		// Orphaned records may cause policy enforcement errors
		for dbName, localDB := range localDatabases {
			if !remoteDatabases[dbName] {
				if err := tx.Delete(localDB).Error; err != nil {
					tx.Rollback()
					return 0, fmt.Errorf("failed to delete obsolete database record for %s: %w", dbName, err)
				}
				deleteCount++
				logger.Infof("Deleted obsolete database record: %s", dbName)
			}
		}

		logger.Infof("Database synchronization completed - Inserted: %d, Deleted: %d", insertCount, deleteCount)
	} else {
		tx.Commit()
		return 0, fmt.Errorf("database type %s is not supported yet", cmt.CntType)
	}

	if err := tx.Commit().Error; err != nil {
		return 0, fmt.Errorf("failed to commit database synchronization transaction: %w", err)
	}

	return insertCount + deleteCount, nil
}

// Create creates a new database on the remote server and registers it locally.
// Database must not already exist for this connection (enforced by duplicate check).
// Returns the created database record with auto-generated ID for audit purposes.
func (s *dbMgtService) Create(ctx context.Context, data models.DBMgt) (*models.DBMgt, error) {
	tx := s.baseRepo.Begin()

	cmt, err := s.cntMgtRepo.GetCntMgtByID(tx, data.CntID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("cntmgt id=%d not found: %v", data.CntID, err)
	}
	logger.Infof("Found cntmgt id=%d, cnttype=%s", cmt.ID, cmt.CntType)

	// SQL templates must be decoded at execution time only for security
	dbType := s.dbTypeAll[0]
	if dbType.SqlCreate == "" {
		tx.Rollback()
		return nil, fmt.Errorf("dbtype.sql_create is empty for dbtype=%s", cmt.CntType)
	}

	sqlBytes, err := hex.DecodeString(dbType.SqlCreate)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to decode SQL template for dbtype %s: %w", cmt.CntType, err)
	}

	// Variable substitution performed after decoding to maintain template security
	rawSQL := string(sqlBytes)
	finalSQL := strings.ReplaceAll(rawSQL, "${dbmgt.dbname}", data.DbName)
	logger.Debugf("Final SQL after substitution: %s", finalSQL)

	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("cannot find endpoint with id=%d: %v", cmt.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	clientID := ep.ClientID

	// Duplicate check required to prevent remote database creation conflicts
	// Multiple creates could corrupt database state on remote server
	countDB, err := s.dbMgtRepo.CountByCntIdAndDBNameAndDBType(tx, cmt.ID, data.DbName, cmt.CntType)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to check duplicate database: %w", err)
	}

	if countDB > 0 {
		tx.Rollback()
		return nil, fmt.Errorf("database already exists: cnt_id=%d, dbname=%s, dbtype=%s", cmt.ID, data.DbName, cmt.CntType)
	}

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
		tx.Rollback()
		return nil, fmt.Errorf("failed to create agent command JSON for database %s: %w", data.DbName, err)
	}
	logger.Debugf("Created agent command JSON payload (hex): %s", hexJSON)

	_, err = agent.ExecuteSqlAgentAPI(clientID, ep.OsType, "execute", hexJSON, "", false)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create database %s on remote server: %w", data.DbName, err)
	}

	// Default to enabled for new databases per security policy
	data.Status = "enabled"
	if err := tx.Create(&data).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to insert database record %s: %w", data.DbName, err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit database creation transaction: %w", err)
	}

	return &data, nil
}

// Delete drops the database from remote server and removes local record.
// Both remote deletion and local record removal must succeed to maintain consistency.
// Returns error if database doesn't exist or remote deletion fails.
func (s *dbMgtService) Delete(ctx context.Context, id uint) error {
	tx := s.baseRepo.Begin()

	existing, err := s.dbMgtRepo.GetByID(tx, id)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("dbmgt with id=%d not found: %v", id, err)
	}

	cmt, err := s.cntMgtRepo.GetCntMgtByID(tx, existing.CntID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("cntmgt id=%d not found: %v", existing.CntID, err)
	}
	logger.Infof("Found cntmgt id=%d, cnttype=%s", cmt.ID, cmt.CntType)

	// SQL templates must be decoded securely at execution time
	dbType := s.dbTypeAll[0]
	if dbType.SqlDelete == "" {
		tx.Rollback()
		return fmt.Errorf("dbtype.sql_delete is empty for dbtype=%s", cmt.CntType)
	}

	sqlBytes, err := hex.DecodeString(dbType.SqlDelete)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to decode SQL delete template: %w", err)
	}
	rawSQL := string(sqlBytes)
	finalSQL := strings.ReplaceAll(rawSQL, "${dbmgt.dbname}", existing.DbName)

	logger.Debugf("Final SQL for database deletion: %s", finalSQL)

	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("endpoint id=%d not found: %v", cmt.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	clientID := ep.ClientID

	queryParam := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cmt.CntType)).
		SetHost(cmt.IP).
		SetPort(cmt.Port).
		SetUser(cmt.Username).
		SetPassword(cmt.Password).
		SetQuery(finalSQL).
		SetDatabase(existing.DbName).
		Build()

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create agent command JSON for database deletion %s: %w", existing.DbName, err)
	}
	logger.Debugf("Created agent command JSON payload (hex): %s", hexJSON)

	_, err = agent.ExecuteSqlAgentAPI(clientID, ep.OsType, "execute", hexJSON, "", false)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete database %s on remote server: %w", existing.DbName, err)
	}

	if err := tx.Delete(&models.DBMgt{}, id).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete database record with id=%d: %w", id, err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit database deletion transaction: %w", err)
	}

	return nil
}
