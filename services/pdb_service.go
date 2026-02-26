package services

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services/agent"
	"dbfartifactapi/services/dto"
	"dbfartifactapi/utils"
)

// Hardcoded Oracle PDB SQL commands - not queried from database templates
const (
	pdbGetAllSQL  = "SELECT name, pdb, con_id FROM v$services;"
	pdbCreateSQL  = "CREATE PLUGGABLE DATABASE %s %s;"
	pdbAlterSQL   = "ALTER PLUGGABLE DATABASE %s %s;"
	pdbDropClose  = "BEGIN EXECUTE IMMEDIATE 'ALTER PLUGGABLE DATABASE %s CLOSE IMMEDIATE INSTANCES=ALL'; EXCEPTION WHEN OTHERS THEN IF SQLCODE != -65020 THEN RAISE; END IF; END;"
	pdbDropDrop   = "DROP PLUGGABLE DATABASE %s INCLUDING DATAFILES"
	oraclePLSQLSep = "\n/\n"
)

// PDBService provides business logic for Oracle Pluggable Database management.
// All methods accept context.Context for cancellation and timeout control.
type PDBService interface {
	// GetAll synchronizes local PDB records with remote Oracle CDB.
	// Queries v$services on remote server and syncs with cntmgt table.
	// Returns the total number of changes (inserts + deletes) made during synchronization.
	GetAll(ctx context.Context, cntMgtID uint) (int, error)

	// Create creates a new PDB on the remote Oracle server and registers it in cntmgt.
	// Returns the created cntmgt record with auto-generated ID.
	Create(ctx context.Context, req PDBCreateRequest) (*models.CntMgt, error)

	// Update executes ALTER PLUGGABLE DATABASE on the remote Oracle server.
	Update(ctx context.Context, id uint, req PDBUpdateRequest) error

	// Delete drops a PDB from the remote Oracle server and removes the cntmgt record.
	Delete(ctx context.Context, id uint, sqlParam string) error
}

// PDBCreateRequest contains parameters for creating a new Oracle PDB.
type PDBCreateRequest struct {
	CntMgt   uint   `json:"cntmgt" validate:"required"`
	PDBName  string `json:"pdbname" validate:"required"`
	SqlParam string `json:"sql_param,omitempty"`
}

// PDBUpdateRequest contains parameters for altering an existing Oracle PDB.
type PDBUpdateRequest struct {
	SqlParam string `json:"sql_param" validate:"required"`
}

type pdbService struct {
	baseRepo     repository.BaseRepository
	cntMgtRepo   repository.CntMgtRepository
	endpointRepo repository.EndpointRepository
}

// NewPDBService creates a new PDB management service instance.
func NewPDBService() PDBService {
	return &pdbService{
		baseRepo:     repository.NewBaseRepository(),
		cntMgtRepo:   repository.NewCntMgtRepository(),
		endpointRepo: repository.NewEndpointRepository(),
	}
}

// NewPDBServiceWithDeps creates a service instance with injected dependencies.
// Used for testing to provide mock implementations of repositories.
func NewPDBServiceWithDeps(
	baseRepo repository.BaseRepository,
	cntMgtRepo repository.CntMgtRepository,
	endpointRepo repository.EndpointRepository,
) PDBService {
	return &pdbService{
		baseRepo:     baseRepo,
		cntMgtRepo:   cntMgtRepo,
		endpointRepo: endpointRepo,
	}
}

// remotePDBInfo holds PDB metadata discovered from v$services query.
type remotePDBInfo struct {
	pdbName     string // Oracle PDB name (e.g., "PDB3")
	serviceName string // Oracle service name for connections (e.g., "pdb3")
}

// GetAll synchronizes local PDB records with remote Oracle CDB.
// Queries v$services on the remote server to discover PDBs and their service names,
// then performs two-phase sync:
// Phase 1: INSERT new PDBs found remotely but not locally
// Phase 2: DELETE obsolete PDBs existing locally but not remotely
// Returns total number of changes (inserts + deletes) for audit trail.
func (s *pdbService) GetAll(ctx context.Context, cntMgtID uint) (int, error) {
	tx := s.baseRepo.Begin()

	cdb, err := s.cntMgtRepo.GetCntMgtByID(tx, cntMgtID)
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("cntmgt id=%d not found: %w", cntMgtID, err)
	}
	logger.Infof("Found CDB connection: id=%d, cnttype=%s", cdb.ID, cdb.CntType)

	if strings.ToLower(cdb.CntType) != "oracle" {
		tx.Rollback()
		return 0, fmt.Errorf("PDB operations only supported for Oracle connections, got %s", cdb.CntType)
	}

	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cdb.Agent))
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("cannot find endpoint with id=%d: %w", cdb.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	// Oracle requires service_name to establish connection
	queryParam := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cdb.CntType)).
		SetHost(cdb.IP).
		SetPort(cdb.Port).
		SetUser(cdb.Username).
		SetPassword(cdb.Password).
		SetDatabase(cdb.ServiceName).
		SetQuery(pdbGetAllSQL).
		Build()

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("failed to create agent command JSON for cntmgt %d: %w", cntMgtID, err)
	}
	logger.Debugf("Created agent command JSON payload (hex): %s", hexJSON)

	stdout, err := agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", true)
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("executeSqlAgentAPI error: %w", err)
	}

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

	logger.Infof("SQL execution successful: %s, found %d services", sqlResp.Message, sqlResp.RowCount)

	// Parse v$services results: [service_name, pdb_name, con_id]
	// Filter out CDB$ROOT entries - only keep actual PDB services
	remotePDBs := make(map[string]remotePDBInfo)
	for _, result := range sqlResp.Results {
		if len(result) < 3 {
			continue
		}
		serviceName := strings.TrimSpace(result[0])
		pdbName := strings.TrimSpace(result[1])

		// CDB$ROOT is the container root, not a pluggable database
		if strings.ToUpper(pdbName) == "CDB$ROOT" {
			continue
		}

		// Skip entries with NULL service name
		if serviceName == "" || strings.ToUpper(serviceName) == "NULL" {
			continue
		}

		pdbKey := strings.ToLower(pdbName)
		// First service found for a PDB wins (avoid duplicates)
		if _, exists := remotePDBs[pdbKey]; !exists {
			remotePDBs[pdbKey] = remotePDBInfo{
				pdbName:     pdbName,
				serviceName: serviceName,
			}
			logger.Debugf("Discovered PDB: name=%s, service_name=%s", pdbName, serviceName)
		}
	}

	// Get existing local PDB records under this CDB
	existingPDBs, err := s.cntMgtRepo.GetByParentConnectionID(tx, cdb.ID)
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("failed to get existing PDBs for CDB %d: %w", cdb.ID, err)
	}

	// Build index of local PDBs by cntname for efficient lookup
	localPDBs := make(map[string]*models.CntMgt)
	for i := range existingPDBs {
		pdb := &existingPDBs[i]
		localPDBs[strings.ToLower(pdb.CntName)] = pdb
	}

	insertCount := 0
	deleteCount := 0

	// Phase 1: Add new PDBs to maintain complete inventory
	for pdbKey, info := range remotePDBs {
		if _, exists := localPDBs[pdbKey]; !exists {
			parentID := cdb.ID
			record := models.CntMgt{
				CntName:            info.pdbName,
				CntType:            cdb.CntType,
				IP:                 cdb.IP,
				Port:               cdb.Port,
				ConfigFilePath:     cdb.ConfigFilePath,
				Username:           cdb.Username,
				Password:           cdb.Password,
				UserIP:             cdb.UserIP,
				Agent:              cdb.Agent,
				Status:             "enabled",
				Profile:            cdb.Profile,
				ParentConnectionID: &parentID,
				ServiceName:        info.serviceName,
				Description:        "Auto-collected by V2-DBF Agent",
			}
			if err := s.cntMgtRepo.Create(tx, &record); err != nil {
				tx.Rollback()
				return 0, fmt.Errorf("failed to create PDB record for %s: %w", info.pdbName, err)
			}
			insertCount++
			logger.Infof("Created new PDB record: %s (service_name=%s)", record.CntName, info.serviceName)
		}
	}

	// Phase 2: Remove stale PDB references to maintain data consistency
	for localKey, localPDB := range localPDBs {
		if _, exists := remotePDBs[localKey]; !exists {
			if err := s.cntMgtRepo.DeleteByID(tx, localPDB.ID); err != nil {
				tx.Rollback()
				return 0, fmt.Errorf("failed to delete obsolete PDB record for %s: %w", localPDB.CntName, err)
			}
			deleteCount++
			logger.Infof("Deleted obsolete PDB record: %s", localPDB.CntName)
		}
	}

	logger.Infof("PDB synchronization completed - Inserted: %d, Deleted: %d", insertCount, deleteCount)

	if err := tx.Commit().Error; err != nil {
		return 0, fmt.Errorf("failed to commit PDB synchronization transaction: %w", err)
	}

	return insertCount + deleteCount, nil
}

// Create creates a new PDB on the remote Oracle server and registers it in cntmgt.
// Inherits connection details from the parent CDB. PDB name must be unique under the CDB.
// Returns the created cntmgt record with auto-generated ID for audit purposes.
func (s *pdbService) Create(ctx context.Context, req PDBCreateRequest) (*models.CntMgt, error) {
	tx := s.baseRepo.Begin()

	cdb, err := s.cntMgtRepo.GetCntMgtByID(tx, req.CntMgt)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("cntmgt id=%d not found: %w", req.CntMgt, err)
	}
	logger.Infof("Found CDB connection: id=%d, cnttype=%s", cdb.ID, cdb.CntType)

	if strings.ToLower(cdb.CntType) != "oracle" {
		tx.Rollback()
		return nil, fmt.Errorf("PDB operations only supported for Oracle connections, got %s", cdb.CntType)
	}

	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cdb.Agent))
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("cannot find endpoint with id=%d: %w", cdb.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	// Duplicate check to prevent conflicting PDB creation on remote server
	countPDB, err := s.cntMgtRepo.CountByParentIDAndCntName(tx, cdb.ID, req.PDBName)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to check duplicate PDB: %w", err)
	}
	if countPDB > 0 {
		tx.Rollback()
		return nil, fmt.Errorf("PDB already exists: parent_id=%d, pdbname=%s", cdb.ID, req.PDBName)
	}

	// Build SQL with optional hex-decoded sql_param
	decodedSqlParam := ""
	if req.SqlParam != "" {
		sqlParamBytes, err := hex.DecodeString(req.SqlParam)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to decode hex sql_param: %w", err)
		}
		decodedSqlParam = string(sqlParamBytes)
	}

	finalSQL := strings.TrimSpace(fmt.Sprintf(pdbCreateSQL, req.PDBName, decodedSqlParam))
	logger.Debugf("Final SQL for PDB creation: %s", finalSQL)

	// Oracle requires service_name to establish connection to CDB
	queryParam := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cdb.CntType)).
		SetHost(cdb.IP).
		SetPort(cdb.Port).
		SetUser(cdb.Username).
		SetPassword(cdb.Password).
		SetDatabase(cdb.ServiceName).
		SetQuery(finalSQL).
		Build()

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create agent command JSON for PDB %s: %w", req.PDBName, err)
	}
	logger.Debugf("Created agent command JSON payload (hex): %s", hexJSON)

	_, err = agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", false)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create PDB %s on remote server: %w", req.PDBName, err)
	}

	// Register PDB in cntmgt with connection details inherited from parent CDB
	// Oracle service_name defaults to lowercase PDB name
	parentID := cdb.ID
	pdbRecord := models.CntMgt{
		CntName:            req.PDBName,
		CntType:            cdb.CntType,
		IP:                 cdb.IP,
		Port:               cdb.Port,
		ConfigFilePath:     cdb.ConfigFilePath,
		Username:           cdb.Username,
		Password:           cdb.Password,
		UserIP:             cdb.UserIP,
		Agent:              cdb.Agent,
		Status:             "enabled",
		Profile:            cdb.Profile,
		ParentConnectionID: &parentID,
		ServiceName:        strings.ToLower(req.PDBName),
		Description:        "Auto-collected by V2-DBF Agent",
	}

	if err := s.cntMgtRepo.Create(tx, &pdbRecord); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to insert PDB record %s: %w", req.PDBName, err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit PDB creation transaction: %w", err)
	}

	return &pdbRecord, nil
}

// Update executes ALTER PLUGGABLE DATABASE on the remote Oracle server.
// Validates that the target cntmgt record is a PDB (has parent_connection_id).
func (s *pdbService) Update(ctx context.Context, id uint, req PDBUpdateRequest) error {
	tx := s.baseRepo.Begin()

	pdb, err := s.cntMgtRepo.GetCntMgtByID(tx, id)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("PDB cntmgt id=%d not found: %w", id, err)
	}

	// Ensure target is a PDB, not a CDB
	if pdb.ParentConnectionID == nil {
		tx.Rollback()
		return fmt.Errorf("cntmgt id=%d is not a PDB (no parent_connection_id)", id)
	}

	cdb, err := s.cntMgtRepo.GetCntMgtByID(tx, *pdb.ParentConnectionID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("parent CDB cntmgt id=%d not found: %w", *pdb.ParentConnectionID, err)
	}

	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cdb.Agent))
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("cannot find endpoint with id=%d: %w", cdb.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	// Decode hex-encoded sql_param for ALTER command
	decodedSqlParam := ""
	if req.SqlParam != "" {
		sqlParamBytes, err := hex.DecodeString(req.SqlParam)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to decode hex sql_param: %w", err)
		}
		decodedSqlParam = string(sqlParamBytes)
	}

	// PDB name stored directly as cntname
	finalSQL := strings.TrimSpace(fmt.Sprintf(pdbAlterSQL, pdb.CntName, decodedSqlParam))
	logger.Debugf("Final SQL for PDB alter: %s", finalSQL)

	// Oracle requires service_name to establish connection to CDB
	queryParam := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cdb.CntType)).
		SetHost(cdb.IP).
		SetPort(cdb.Port).
		SetUser(cdb.Username).
		SetPassword(cdb.Password).
		SetDatabase(cdb.ServiceName).
		SetQuery(finalSQL).
		Build()

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create agent command JSON for PDB alter: %w", err)
	}
	logger.Debugf("Created agent command JSON payload (hex): %s", hexJSON)

	_, err = agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", false)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to alter PDB %s on remote server: %w", pdb.CntName, err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit PDB alter transaction: %w", err)
	}

	return nil
}

// Delete drops a PDB from the remote Oracle server and removes the cntmgt record.
// Validates that the target cntmgt record is a PDB (has parent_connection_id).
// Optional sqlParam supports DROP options like "INCLUDING DATAFILES".
func (s *pdbService) Delete(ctx context.Context, id uint, sqlParam string) error {
	tx := s.baseRepo.Begin()

	pdb, err := s.cntMgtRepo.GetCntMgtByID(tx, id)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("PDB cntmgt id=%d not found: %w", id, err)
	}

	// Ensure target is a PDB, not a CDB
	if pdb.ParentConnectionID == nil {
		tx.Rollback()
		return fmt.Errorf("cntmgt id=%d is not a PDB (no parent_connection_id)", id)
	}

	cdb, err := s.cntMgtRepo.GetCntMgtByID(tx, *pdb.ParentConnectionID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("parent CDB cntmgt id=%d not found: %w", *pdb.ParentConnectionID, err)
	}

	ep, err := s.endpointRepo.GetByID(tx, utils.MustIntToUint(cdb.Agent))
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("cannot find endpoint with id=%d: %w", cdb.Agent, err)
	}
	logger.Infof("Found endpoint id=%d, client_id=%s, os_type=%s", ep.ID, ep.ClientID, ep.OsType)

	// Decode optional hex-encoded sql_param for additional DROP command options
	decodedSqlParam := ""
	if sqlParam != "" {
		sqlParamBytes, err := hex.DecodeString(sqlParam)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to decode hex sql_param: %w", err)
		}
		decodedSqlParam = string(sqlParamBytes)
	}

	// Build multi-command PL/SQL: CLOSE then DROP, joined by Oracle separator
	// Follows executeBatchSQLForConnection pattern from group_management_service
	commands := []string{
		fmt.Sprintf(pdbDropClose, pdb.CntName),
		fmt.Sprintf(pdbDropDrop, pdb.CntName),
	}
	if strings.TrimSpace(decodedSqlParam) != "" {
		commands = append(commands, decodedSqlParam)
	}
	finalSQL := strings.Join(commands, oraclePLSQLSep)
	logger.Debugf("Final SQL for PDB drop: %s", finalSQL)

	// Oracle requires service_name to establish connection to CDB
	queryParam := dto.NewDBQueryParamBuilder().
		SetDBType(strings.ToLower(cdb.CntType)).
		SetHost(cdb.IP).
		SetPort(cdb.Port).
		SetUser(cdb.Username).
		SetPassword(cdb.Password).
		SetDatabase(cdb.ServiceName).
		SetQuery(finalSQL).
		Build()

	hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create agent command JSON for PDB drop: %w", err)
	}
	logger.Debugf("Created agent command JSON payload (hex): %s", hexJSON)

	_, err = agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", false)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to drop PDB %s on remote server: %w", pdb.CntName, err)
	}

	if err := s.cntMgtRepo.DeleteByID(tx, id); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete PDB record with id=%d: %w", id, err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit PDB deletion transaction: %w", err)
	}

	return nil
}
