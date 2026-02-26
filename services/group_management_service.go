package services

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"dbfartifactapi/bootstrap"
	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services/agent"
	"dbfartifactapi/services/dto"
	"dbfartifactapi/utils"

	"gorm.io/gorm"
)

// GroupManagementService provides business logic for group management with policy and actor assignments.
type GroupManagementService interface {
	// Group Management
	CreateGroup(ctx context.Context, group *models.DBGroupMgt) (*models.DBGroupMgt, error)
	UpdateGroup(ctx context.Context, id uint, group *models.DBGroupMgt) (*models.DBGroupMgt, error)
	DeleteGroup(ctx context.Context, id uint) error
	GetGroupByID(ctx context.Context, id uint) (*models.DBGroupMgt, error)
	GetAllGroups(ctx context.Context) ([]models.DBGroupMgt, error)
	GetGroupsByDatabaseType(ctx context.Context, databaseTypeID uint) ([]models.DBGroupMgt, error)

	// Policy Group Management
	AssignPoliciesToGroup(ctx context.Context, groupID uint, policyIDs []uint) error   // Not send VeloArtifact jobs yet
	RemovePoliciesFromGroup(ctx context.Context, groupID uint, policyIDs []uint) error // Not send VeloArtifact jobs yet
	GetActivePoliciesForGroup(ctx context.Context, groupID uint) ([]models.DBGroupListPolicies, error)
	GetGroupsForPolicy(ctx context.Context, policyID uint) ([]models.DBGroupMgt, error)

	// Actor Group Management
	AssignActorsToGroup(ctx context.Context, groupID uint, actorIDs []uint) (*ActorAssignmentResult, error) // Sends VeloArtifact allow commands before database update
	RemoveActorsFromGroup(ctx context.Context, groupID uint, actorIDs []uint) (*ActorRemovalResult, error)  // Sends VeloArtifact deny commands before database update
	GetActiveActorsForGroup(ctx context.Context, groupID uint) ([]models.DBActorMgt, error)
	GetGroupsForActor(ctx context.Context, actorID uint) ([]models.DBGroupMgt, error)

	// Comprehensive Group Information
	GetGroupWithPoliciesAndActors(ctx context.Context, groupID uint) (*GroupInfo, error)

	// Optimized Bulk Update - updates both policies and actors with minimal VeloArtifact executions
	UpdateGroupAssignments(ctx context.Context, groupID uint, request *GroupAssignmentsUpdateRequest) (*GroupAssignmentsUpdateResult, error)
}

// GroupInfo contains complete group information with policies and actors
type GroupInfo struct {
	Group    *models.DBGroupMgt           `json:"group"`
	Policies []models.DBGroupListPolicies `json:"policies"`
	Actors   []models.DBActorMgt          `json:"actors"`
}

// GroupAssignmentsUpdateRequest defines the request for bulk update of group assignments
type GroupAssignmentsUpdateRequest struct {
	PolicyIDs []uint `json:"policy_ids"`
	ActorIDs  []uint `json:"actor_ids"`
}

// GroupAssignmentsUpdateResult contains the result of bulk update operation
type GroupAssignmentsUpdateResult struct {
	PoliciesAdded   []uint   `json:"policies_added"`
	PoliciesRemoved []uint   `json:"policies_removed"`
	ActorsAdded     []uint   `json:"actors_added"`
	ActorsRemoved   []uint   `json:"actors_removed"`
	VeloJobsCreated []string `json:"velo_jobs_created"`
	TotalExecutions int      `json:"total_executions"`
}

// ActorAssignmentResult contains the result of actor assignment operation
type ActorAssignmentResult struct {
	ActorsAssigned    []uint               `json:"actors_assigned"`
	AlreadyAssigned   []uint               `json:"already_assigned"`
	Success           bool                 `json:"success"`
	PartialSuccess    bool                 `json:"partial_success"`
	VeloJobsSucceeded []string             `json:"velo_jobs_succeeded"`
	VeloJobsFailed    []VeloExecutionError `json:"velo_jobs_failed"`
	TotalExecutions   int                  `json:"total_executions"`
}

// ActorRemovalResult contains the result of actor removal operation
type ActorRemovalResult struct {
	ActorsRemoved     []uint               `json:"actors_removed"`
	NotAssigned       []uint               `json:"not_assigned"`
	Success           bool                 `json:"success"`
	PartialSuccess    bool                 `json:"partial_success"`
	VeloJobsSucceeded []string             `json:"velo_jobs_succeeded"`
	VeloJobsFailed    []VeloExecutionError `json:"velo_jobs_failed"`
	TotalExecutions   int                  `json:"total_executions"`
}

// VeloArtifactExecution represents a VeloArtifact execution operation
type VeloArtifactExecution struct {
	Operation        string // "allow" or "deny"
	GroupID          uint
	ConnectionID     uint     // Connection ID (cntid) for batching
	BatchSQLCommands []string // Pre-processed SQL commands ready for execution
}

// VeloExecutionError represents a failed VeloArtifact execution
type VeloExecutionError struct {
	ConnectionID uint   `json:"connection_id"`
	Operation    string `json:"operation"`
	Error        string `json:"error"`
}

// VeloExecutionResult contains detailed results of VeloArtifact operations
type VeloExecutionResult struct {
	SuccessfulJobs []string             `json:"successful_jobs"`
	FailedJobs     []VeloExecutionError `json:"failed_jobs"`
	TotalAttempted int                  `json:"total_attempted"`
	TotalSucceeded int                  `json:"total_succeeded"`
	TotalFailed    int                  `json:"total_failed"`
}

// diffResult contains the difference between current and target assignments
type diffResult struct {
	PoliciesToAdd    []uint
	PoliciesToRemove []uint
	ActorsToAdd      []uint
	ActorsToRemove   []uint
}

type groupManagementService struct {
	baseRepo              repository.BaseRepository
	groupRepo             repository.DBGroupMgtRepository
	policyGroupsRepo      repository.DBPolicyGroupsRepository
	actorGroupsRepo       repository.DBActorGroupsRepository
	groupListPoliciesRepo repository.DBGroupListPoliciesRepository
	actorMgtRepo          repository.DBActorMgtRepository
	dbpolicyRepo          repository.DBPolicyRepository
	// VeloArtifact dependencies
	dbMgtRepo    repository.DBMgtRepository
	cntMgtRepo   repository.CntMgtRepository
	endpointRepo repository.EndpointRepository
}

// NewGroupManagementService creates a new group management service instance.
func NewGroupManagementService() GroupManagementService {
	return &groupManagementService{
		baseRepo:              repository.NewBaseRepository(),
		groupRepo:             repository.NewDBGroupMgtRepository(),
		policyGroupsRepo:      repository.NewDBPolicyGroupsRepository(),
		actorGroupsRepo:       repository.NewDBActorGroupsRepository(),
		groupListPoliciesRepo: repository.NewDBGroupListPoliciesRepository(),
		actorMgtRepo:          repository.NewDBActorMgtRepository(),
		dbpolicyRepo:          repository.NewDBPolicyRepository(),
		// VeloArtifact dependencies
		dbMgtRepo:    repository.NewDBMgtRepository(),
		cntMgtRepo:   repository.NewCntMgtRepository(),
		endpointRepo: repository.NewEndpointRepository(),
	}
}

// Group Management Functions

func (s *groupManagementService) CreateGroup(ctx context.Context, group *models.DBGroupMgt) (*models.DBGroupMgt, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	if group.Name == "" {
		return nil, fmt.Errorf("group name is required")
	}
	if group.Code == "" {
		return nil, fmt.Errorf("group code is required")
	}

	tx := s.baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	// Check if code already exists
	existingGroup, err := s.groupRepo.GetByCode(tx, group.Code)
	if err == nil && existingGroup != nil {
		return nil, fmt.Errorf("group with code '%s' already exists", group.Code)
	}

	// Set default values
	if group.GroupType == "" {
		group.GroupType = "CUSTOM"
	}
	if !group.IsActive {
		group.IsActive = true
	}
	group.CreatedAt = time.Now()
	group.UpdatedAt = time.Now()

	if err := s.groupRepo.Create(tx, group); err != nil {
		return nil, fmt.Errorf("failed to create group: %v", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}
	txCommitted = true

	logger.Infof("Created group: id=%d, name=%s, code=%s", group.ID, group.Name, group.Code)
	return group, nil
}

func (s *groupManagementService) UpdateGroup(ctx context.Context, id uint, group *models.DBGroupMgt) (*models.DBGroupMgt, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	if id == 0 {
		return nil, fmt.Errorf("invalid group ID: must be greater than 0")
	}

	tx := s.baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	// Get existing group
	existingGroup, err := s.groupRepo.GetByID(tx, id)
	if err != nil {
		return nil, fmt.Errorf("group with id=%d not found: %v", id, err)
	}

	// Check if new code conflicts with other groups
	if group.Code != existingGroup.Code {
		conflictGroup, err := s.groupRepo.GetByCode(tx, group.Code)
		if err == nil && conflictGroup != nil && conflictGroup.ID != id {
			return nil, fmt.Errorf("group with code '%s' already exists", group.Code)
		}
	}

	// Update fields
	existingGroup.Name = group.Name
	existingGroup.Code = group.Code
	existingGroup.Description = group.Description
	existingGroup.DatabaseTypeID = group.DatabaseTypeID
	existingGroup.GroupType = group.GroupType
	existingGroup.ParentGroupID = group.ParentGroupID
	existingGroup.IsTemplate = group.IsTemplate
	existingGroup.Metadata = group.Metadata
	existingGroup.IsActive = group.IsActive
	existingGroup.UpdatedAt = time.Now()

	if err := s.groupRepo.Update(tx, existingGroup); err != nil {
		return nil, fmt.Errorf("failed to update group: %v", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}
	txCommitted = true

	logger.Infof("Updated group: id=%d, name=%s, code=%s", existingGroup.ID, existingGroup.Name, existingGroup.Code)
	return existingGroup, nil
}

func (s *groupManagementService) DeleteGroup(ctx context.Context, id uint) error {
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}
	if id == 0 {
		return fmt.Errorf("invalid group ID: must be greater than 0")
	}

	tx := s.baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	// Check if group exists
	group, err := s.groupRepo.GetByID(tx, id)
	if err != nil {
		return fmt.Errorf("group with id=%d not found: %v", id, err)
	}

	// Check for child groups
	childGroups, err := s.groupRepo.GetChildGroups(tx, id)
	if err != nil {
		return fmt.Errorf("failed to check child groups: %v", err)
	}
	if len(childGroups) > 0 {
		return fmt.Errorf("cannot delete group with child groups. Found %d child groups", len(childGroups))
	}

	// Remove all policy associations
	policyGroups, err := s.policyGroupsRepo.GetByGroupID(tx, id)
	if err != nil {
		return fmt.Errorf("failed to get policy associations: %v", err)
	}
	for _, pg := range policyGroups {
		if err := s.policyGroupsRepo.Delete(tx, pg.ID); err != nil {
			return fmt.Errorf("failed to remove policy association: %v", err)
		}
	}

	// Remove all actor associations
	actorGroups, err := s.actorGroupsRepo.GetByGroupID(tx, id)
	if err != nil {
		return fmt.Errorf("failed to get actor associations: %v", err)
	}
	for _, ag := range actorGroups {
		if err := s.actorGroupsRepo.Delete(tx, ag.ID); err != nil {
			return fmt.Errorf("failed to remove actor association: %v", err)
		}
	}

	// Delete the group
	if err := s.groupRepo.Delete(tx, id); err != nil {
		return fmt.Errorf("failed to delete group: %v", err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}
	txCommitted = true

	logger.Infof("Deleted group: id=%d, name=%s", id, group.Name)
	return nil
}

func (s *groupManagementService) GetGroupByID(ctx context.Context, id uint) (*models.DBGroupMgt, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	if id == 0 {
		return nil, fmt.Errorf("invalid group ID: must be greater than 0")
	}

	return s.groupRepo.GetByID(nil, id)
}

func (s *groupManagementService) GetAllGroups(ctx context.Context) ([]models.DBGroupMgt, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	return s.groupRepo.GetActiveGroups(nil)
}

func (s *groupManagementService) GetGroupsByDatabaseType(ctx context.Context, databaseTypeID uint) ([]models.DBGroupMgt, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	if databaseTypeID == 0 {
		return nil, fmt.Errorf("invalid database type ID: must be greater than 0")
	}

	return s.groupRepo.GetByDatabaseType(nil, databaseTypeID)
}

// Policy Group Management Functions

func (s *groupManagementService) AssignPoliciesToGroup(ctx context.Context, groupID uint, policyIDs []uint) error {
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}
	if groupID == 0 {
		return fmt.Errorf("invalid group ID: must be greater than 0")
	}
	if len(policyIDs) == 0 {
		return fmt.Errorf("policy IDs list cannot be empty")
	}

	tx := s.baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	// Verify group exists
	_, err := s.groupRepo.GetByID(tx, groupID)
	if err != nil {
		return fmt.Errorf("group with id=%d not found: %v", groupID, err)
	}

	now := time.Now()
	for _, policyID := range policyIDs {
		// Verify policy exists
		_, err := s.groupListPoliciesRepo.GetByID(tx, policyID)
		if err != nil {
			return fmt.Errorf("policy with id=%d not found: %v", policyID, err)
		}

		// Check if assignment already exists
		existing, err := s.policyGroupsRepo.GetByGroupID(tx, groupID)
		if err != nil {
			return fmt.Errorf("failed to check existing assignments: %v", err)
		}

		alreadyExists := false
		for _, pg := range existing {
			if pg.DBGroupListPoliciesID == policyID && pg.IsActive {
				alreadyExists = true
				break
			}
		}

		if !alreadyExists {
			policyGroup := &models.DBPolicyGroups{
				DBGroupListPoliciesID: policyID,
				GroupID:               groupID,
				ValidFrom:             now,
				IsActive:              true,
			}

			if err := s.policyGroupsRepo.Create(tx, policyGroup); err != nil {
				return fmt.Errorf("failed to assign policy %d to group %d: %v", policyID, groupID, err)
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}
	txCommitted = true

	logger.Infof("Assigned %d policies to group %d", len(policyIDs), groupID)
	return nil
}

func (s *groupManagementService) RemovePoliciesFromGroup(ctx context.Context, groupID uint, policyIDs []uint) error {
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}
	if groupID == 0 {
		return fmt.Errorf("invalid group ID: must be greater than 0")
	}
	if len(policyIDs) == 0 {
		return fmt.Errorf("policy IDs list cannot be empty")
	}

	tx := s.baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	for _, policyID := range policyIDs {
		if err := s.policyGroupsRepo.DeactivateByGroupIDAndPolicyID(tx, groupID, policyID); err != nil {
			return fmt.Errorf("failed to remove policy %d from group %d: %v", policyID, groupID, err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}
	txCommitted = true

	logger.Infof("Removed %d policies from group %d", len(policyIDs), groupID)
	return nil
}

func (s *groupManagementService) GetActivePoliciesForGroup(ctx context.Context, groupID uint) ([]models.DBGroupListPolicies, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	if groupID == 0 {
		return nil, fmt.Errorf("invalid group ID: must be greater than 0")
	}

	policyGroups, err := s.policyGroupsRepo.GetActivePoliciesByGroupID(nil, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active policies for group %d: %v", groupID, err)
	}

	var policies []models.DBGroupListPolicies
	for _, pg := range policyGroups {
		policy, err := s.groupListPoliciesRepo.GetByID(nil, pg.DBGroupListPoliciesID)
		if err != nil {
			logger.Errorf("Failed to get policy details for ID %d: %v", pg.DBGroupListPoliciesID, err)
			continue
		}
		policies = append(policies, *policy)
	}

	return policies, nil
}

func (s *groupManagementService) GetGroupsForPolicy(ctx context.Context, policyID uint) ([]models.DBGroupMgt, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	if policyID == 0 {
		return nil, fmt.Errorf("invalid policy ID: must be greater than 0")
	}

	policyGroups, err := s.policyGroupsRepo.GetByPolicyID(nil, policyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get groups for policy %d: %v", policyID, err)
	}

	var groups []models.DBGroupMgt
	for _, pg := range policyGroups {
		if pg.IsActive {
			group, err := s.groupRepo.GetByID(nil, pg.GroupID)
			if err != nil {
				logger.Errorf("Failed to get group details for ID %d: %v", pg.GroupID, err)
				continue
			}
			groups = append(groups, *group)
		}
	}

	return groups, nil
}

// Actor Group Management Functions

func (s *groupManagementService) AssignActorsToGroup(ctx context.Context, groupID uint, actorIDs []uint) (*ActorAssignmentResult, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	if groupID == 0 {
		return nil, fmt.Errorf("invalid group ID: must be greater than 0")
	}
	if len(actorIDs) == 0 {
		return nil, fmt.Errorf("actor IDs list cannot be empty")
	}

	// Verify group exists
	group, err := s.groupRepo.GetByID(nil, groupID)
	if err != nil {
		return nil, fmt.Errorf("group with id=%d not found: %v", groupID, err)
	}

	logger.Infof("Assigning %d actors to group %d (%s)", len(actorIDs), groupID, group.Name)

	// Get current policies for the group - actors need to be allowed for all current policies
	currentPolicyIDs, err := s.getCurrentPolicyIDs(groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current policies for group %d: %v", groupID, err)
	}

	// Filter out actors that are already assigned
	var newActorIDs []uint
	var alreadyAssignedIDs []uint
	existingAssignments, err := s.actorGroupsRepo.GetByGroupID(nil, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing assignments: %v", err)
	}

	existingActorMap := make(map[uint]bool)
	for _, ag := range existingAssignments {
		if ag.IsActive {
			existingActorMap[ag.ActorID] = true
		}
	}

	for _, actorID := range actorIDs {
		if !existingActorMap[actorID] {
			newActorIDs = append(newActorIDs, actorID)
		} else {
			alreadyAssignedIDs = append(alreadyAssignedIDs, actorID)
		}
	}

	result := &ActorAssignmentResult{
		ActorsAssigned:    []uint{},
		AlreadyAssigned:   alreadyAssignedIDs,
		Success:           false,
		PartialSuccess:    false,
		VeloJobsSucceeded: []string{},
		VeloJobsFailed:    []VeloExecutionError{},
		TotalExecutions:   0,
	}

	// Early return if no new actors to assign
	if len(newActorIDs) == 0 {
		logger.Infof("All actors already assigned to group %d, no changes needed", groupID)
		result.Success = true
		return result, nil
	}

	// Get actor details for new actors
	var actorsToAdd []models.DBActorMgt
	for _, actorID := range newActorIDs {
		actor, err := s.actorMgtRepo.GetByID(nil, actorID)
		if err != nil {
			return nil, fmt.Errorf("actor with id=%d not found: %v", actorID, err)
		}
		actorsToAdd = append(actorsToAdd, *actor)
	}

	// CRITICAL: Execute VeloArtifact operations BEFORE database changes
	// Build VeloArtifact executions - allow new actors for all current policies
	var executions []VeloArtifactExecution
	if len(currentPolicyIDs) > 0 {
		actorsByConnection := s.groupActorsByConnection(actorsToAdd)

		for cntID, connectionActors := range actorsByConnection {
			batchSQLCommands, err := s.buildOptimizedBatchSQL(currentPolicyIDs, connectionActors, "allow", cntID)
			if err != nil {
				logger.Errorf("Failed to build allow commands for actors on connection %d: %v", cntID, err)
				continue
			}

			if len(batchSQLCommands) > 0 {
				executions = append(executions, VeloArtifactExecution{
					Operation:        "allow",
					GroupID:          groupID,
					ConnectionID:     cntID,
					BatchSQLCommands: batchSQLCommands,
				})
			}
		}
	}

	// Execute VeloArtifact operations first (supports partial success)
	veloResult := s.executeVeloArtifactOperations(ctx, executions)
	result.VeloJobsSucceeded = veloResult.SuccessfulJobs
	result.VeloJobsFailed = veloResult.FailedJobs
	result.TotalExecutions = veloResult.TotalAttempted

	// Check if all VeloArtifact operations failed
	if veloResult.TotalSucceeded == 0 && veloResult.TotalAttempted > 0 {
		return result, fmt.Errorf("all VeloArtifact operations failed (%d/%d)", veloResult.TotalFailed, veloResult.TotalAttempted)
	}

	// Start transaction for database operations
	tx := s.baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	// Apply database changes (at least some VeloArtifact operations succeeded)
	now := time.Now()
	for _, actorID := range newActorIDs {
		actorGroup := &models.DBActorGroups{
			ActorID:   actorID,
			GroupID:   groupID,
			ValidFrom: now,
			IsActive:  true,
		}

		if err := s.actorGroupsRepo.Create(tx, actorGroup); err != nil {
			return nil, fmt.Errorf("failed to assign actor %d to group %d: %v", actorID, groupID, err)
		}
	}

	// Sync dbpolicy table: create actor-wide policies for group's policies
	if len(currentPolicyIDs) > 0 {
		// Extract policy default IDs from current group policies (ASSIGN operation uses dbpolicydefault_id)
		policyDefaultIDs, err := s.extractPolicyDefaultIDs(currentPolicyIDs, PolicyOperationAssign)
		if err != nil {
			return nil, fmt.Errorf("failed to extract policy defaults: %v", err)
		}

		if len(policyDefaultIDs) > 0 {
			// Create dbpolicy records for new actors
			insertedCount, err := s.syncDBPolicyForActors(tx, actorsToAdd, policyDefaultIDs)
			if err != nil {
				return nil, fmt.Errorf("failed to sync dbpolicy records: %v", err)
			}
			logger.Infof("Synced dbpolicy for group %d: inserted %d records", groupID, insertedCount)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}
	txCommitted = true

	// Export DBF policy rules after dbpolicy changes (run in background)
	logger.Infof("Starting background exportDBFPolicy to build rule files for group %d actor assignment", groupID)
	go func(gID uint) {
		if err := utils.ExportDBFPolicy(); err != nil {
			logger.Warnf("Failed to export DBF policy rules for group %d: %v", gID, err)
		} else {
			logger.Infof("Successfully exported DBF policy rules for group %d actor assignment", gID)
		}
	}(groupID)

	result.ActorsAssigned = newActorIDs
	result.Success = veloResult.TotalFailed == 0
	result.PartialSuccess = veloResult.TotalSucceeded > 0 && veloResult.TotalFailed > 0

	logger.Infof("Assigned %d actors to group %d, VeloArtifact: %d succeeded, %d failed (success=%v, partial=%v)",
		len(newActorIDs), groupID, veloResult.TotalSucceeded, veloResult.TotalFailed, result.Success, result.PartialSuccess)
	return result, nil
}

func (s *groupManagementService) RemoveActorsFromGroup(ctx context.Context, groupID uint, actorIDs []uint) (*ActorRemovalResult, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	if groupID == 0 {
		return nil, fmt.Errorf("invalid group ID: must be greater than 0")
	}
	if len(actorIDs) == 0 {
		return nil, fmt.Errorf("actor IDs list cannot be empty")
	}

	// Verify group exists
	group, err := s.groupRepo.GetByID(nil, groupID)
	if err != nil {
		return nil, fmt.Errorf("group with id=%d not found: %v", groupID, err)
	}

	logger.Infof("Removing %d actors from group %d (%s)", len(actorIDs), groupID, group.Name)

	// Get current policies for the group - actors need to be denied for all current policies
	currentPolicyIDs, err := s.getCurrentPolicyIDs(groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current policies for group %d: %v", groupID, err)
	}

	// Filter only actors that are actually assigned to the group
	var activeActorIDs []uint
	var notAssignedIDs []uint
	existingAssignments, err := s.actorGroupsRepo.GetByGroupID(nil, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing assignments: %v", err)
	}

	existingActorMap := make(map[uint]bool)
	for _, ag := range existingAssignments {
		if ag.IsActive {
			existingActorMap[ag.ActorID] = true
		}
	}

	for _, actorID := range actorIDs {
		if existingActorMap[actorID] {
			activeActorIDs = append(activeActorIDs, actorID)
		} else {
			notAssignedIDs = append(notAssignedIDs, actorID)
		}
	}

	result := &ActorRemovalResult{
		ActorsRemoved:     []uint{},
		NotAssigned:       notAssignedIDs,
		Success:           false,
		PartialSuccess:    false,
		VeloJobsSucceeded: []string{},
		VeloJobsFailed:    []VeloExecutionError{},
		TotalExecutions:   0,
	}

	// Early return if no actors to remove
	if len(activeActorIDs) == 0 {
		logger.Infof("No active actors to remove from group %d", groupID)
		result.Success = true
		return result, nil
	}

	// Get actor details for actors to remove
	var actorsToRemove []models.DBActorMgt
	for _, actorID := range activeActorIDs {
		actor, err := s.actorMgtRepo.GetByID(nil, actorID)
		if err != nil {
			return nil, fmt.Errorf("actor with id=%d not found: %v", actorID, err)
		}
		actorsToRemove = append(actorsToRemove, *actor)
	}

	// CRITICAL: Execute VeloArtifact operations BEFORE database changes
	// Build VeloArtifact executions - deny removed actors for all current policies
	var executions []VeloArtifactExecution
	if len(currentPolicyIDs) > 0 {
		actorsByConnection := s.groupActorsByConnection(actorsToRemove)

		for cntID, connectionActors := range actorsByConnection {
			batchSQLCommands, err := s.buildOptimizedBatchSQL(currentPolicyIDs, connectionActors, "deny", cntID)
			if err != nil {
				logger.Errorf("Failed to build deny commands for actors on connection %d: %v", cntID, err)
				continue
			}

			if len(batchSQLCommands) > 0 {
				executions = append(executions, VeloArtifactExecution{
					Operation:        "deny",
					GroupID:          groupID,
					ConnectionID:     cntID,
					BatchSQLCommands: batchSQLCommands,
				})
			}
		}
	}

	// Execute VeloArtifact operations first (supports partial success)
	veloResult := s.executeVeloArtifactOperations(ctx, executions)
	result.VeloJobsSucceeded = veloResult.SuccessfulJobs
	result.VeloJobsFailed = veloResult.FailedJobs
	result.TotalExecutions = veloResult.TotalAttempted

	// Check if all VeloArtifact operations failed
	if veloResult.TotalSucceeded == 0 && veloResult.TotalAttempted > 0 {
		return result, fmt.Errorf("all VeloArtifact operations failed (%d/%d)", veloResult.TotalFailed, veloResult.TotalAttempted)
	}

	// TODO: Uncomment to ignore agent failures and always proceed with database removal
	// if veloResult.TotalFailed > 0 {
	// 	logger.Warnf("VeloArtifact deny operations had failures for group %d (%d/%d failed), proceeding with database removal",
	// 		groupID, veloResult.TotalFailed, veloResult.TotalAttempted)
	// }

	// Start transaction for database operations
	tx := s.baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	// Apply database changes (at least some VeloArtifact operations succeeded)
	for _, actorID := range activeActorIDs {
		if err := s.actorGroupsRepo.DeactivateByActorIDAndGroupID(tx, actorID, groupID); err != nil {
			return nil, fmt.Errorf("failed to remove actor %d from group %d: %v", actorID, groupID, err)
		}
	}

	// Cleanup dbpolicy table: remove actor-wide policies for group's policies
	if len(currentPolicyIDs) > 0 {
		// Extract policy default IDs from current group policies (REMOVE operation uses contained_policydefaults)
		policyDefaultIDs, err := s.extractPolicyDefaultIDs(currentPolicyIDs, PolicyOperationRemove)
		if err != nil {
			return nil, fmt.Errorf("failed to extract policy defaults: %v", err)
		}

		if len(policyDefaultIDs) > 0 {
			// Delete dbpolicy records for removed actors
			if err := s.cleanupDBPolicyForActors(tx, actorsToRemove, policyDefaultIDs); err != nil {
				return nil, fmt.Errorf("failed to cleanup dbpolicy records: %v", err)
			}
			logger.Infof("Cleaned up dbpolicy for group %d: removed records for %d actors", groupID, len(actorsToRemove))
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}
	txCommitted = true

	// Export DBF policy rules after dbpolicy changes (run in background)
	logger.Infof("Starting background exportDBFPolicy to build rule files for group %d actor removal", groupID)
	go func(gID uint) {
		if err := utils.ExportDBFPolicy(); err != nil {
			logger.Warnf("Failed to export DBF policy rules for group %d: %v", gID, err)
		} else {
			logger.Infof("Successfully exported DBF policy rules for group %d actor removal", gID)
		}
	}(groupID)

	result.ActorsRemoved = activeActorIDs
	result.Success = veloResult.TotalFailed == 0
	result.PartialSuccess = veloResult.TotalSucceeded > 0 && veloResult.TotalFailed > 0
	// TODO: Uncomment to ignore agent failures and always treat removal as success
	// result.Success = true

	logger.Infof("Removed %d actors from group %d, VeloArtifact: %d succeeded, %d failed (success=%v, partial=%v)",
		len(activeActorIDs), groupID, veloResult.TotalSucceeded, veloResult.TotalFailed, result.Success, result.PartialSuccess)
	return result, nil
}

func (s *groupManagementService) GetActiveActorsForGroup(ctx context.Context, groupID uint) ([]models.DBActorMgt, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	if groupID == 0 {
		return nil, fmt.Errorf("invalid group ID: must be greater than 0")
	}

	actorGroups, err := s.actorGroupsRepo.GetActiveActorsByGroupID(nil, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active actors for group %d: %v", groupID, err)
	}

	var actors []models.DBActorMgt
	for _, ag := range actorGroups {
		actor, err := s.actorMgtRepo.GetByID(nil, ag.ActorID)
		if err != nil {
			logger.Errorf("Failed to get actor details for ID %d: %v", ag.ActorID, err)
			continue
		}
		actors = append(actors, *actor)
	}

	return actors, nil
}

func (s *groupManagementService) GetGroupsForActor(ctx context.Context, actorID uint) ([]models.DBGroupMgt, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	if actorID == 0 {
		return nil, fmt.Errorf("invalid actor ID: must be greater than 0")
	}

	actorGroups, err := s.actorGroupsRepo.GetActiveGroupsByActorID(nil, actorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get groups for actor %d: %v", actorID, err)
	}

	var groups []models.DBGroupMgt
	for _, ag := range actorGroups {
		group, err := s.groupRepo.GetByID(nil, ag.GroupID)
		if err != nil {
			logger.Errorf("Failed to get group details for ID %d: %v", ag.GroupID, err)
			continue
		}
		groups = append(groups, *group)
	}

	return groups, nil
}

// Comprehensive Group Information

func (s *groupManagementService) GetGroupWithPoliciesAndActors(ctx context.Context, groupID uint) (*GroupInfo, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	if groupID == 0 {
		return nil, fmt.Errorf("invalid group ID: must be greater than 0")
	}

	// Get group details
	group, err := s.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get group: %v", err)
	}

	// Get associated policies
	policies, err := s.GetActivePoliciesForGroup(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get policies: %v", err)
	}

	// Get associated actors
	actors, err := s.GetActiveActorsForGroup(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get actors: %v", err)
	}

	return &GroupInfo{
		Group:    group,
		Policies: policies,
		Actors:   actors,
	}, nil
}

// Optimized Bulk Update Implementation

func (s *groupManagementService) UpdateGroupAssignments(ctx context.Context, groupID uint, request *GroupAssignmentsUpdateRequest) (*GroupAssignmentsUpdateResult, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	if groupID == 0 {
		return nil, fmt.Errorf("invalid group ID: must be greater than 0")
	}
	if request == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	// Input validation
	if len(request.PolicyIDs) == 0 && len(request.ActorIDs) == 0 {
		return nil, fmt.Errorf("at least one policy or actor ID must be provided")
	}

	// Verify group exists
	group, err := s.groupRepo.GetByID(nil, groupID)
	if err != nil {
		return nil, fmt.Errorf("group with id=%d not found: %v", groupID, err)
	}

	logger.Infof("Starting optimized bulk update for group %d (%s)", groupID, group.Name)

	// Get current assignments to calculate diff
	currentPolicyIDs, err := s.getCurrentPolicyIDs(groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current policy assignments: %v", err)
	}

	currentActorIDs, err := s.getCurrentActorIDs(groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current actor assignments: %v", err)
	}

	// Calculate what changes are needed
	diff := s.calculateDiff(currentPolicyIDs, currentActorIDs, request.PolicyIDs, request.ActorIDs)

	result := &GroupAssignmentsUpdateResult{
		PoliciesAdded:   diff.PoliciesToAdd,
		PoliciesRemoved: diff.PoliciesToRemove,
		ActorsAdded:     diff.ActorsToAdd,
		ActorsRemoved:   diff.ActorsToRemove,
		VeloJobsCreated: []string{},
		TotalExecutions: 0,
	}

	// Early return if no changes needed
	if len(diff.PoliciesToAdd) == 0 && len(diff.PoliciesToRemove) == 0 &&
		len(diff.ActorsToAdd) == 0 && len(diff.ActorsToRemove) == 0 {
		logger.Infof("No changes needed for group %d", groupID)
		return result, nil
	}

	// Start transaction for all database operations
	tx := s.baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	// IMPORTANT: Execute VeloArtifact operations BEFORE database changes to avoid inconsistency
	// Collect all VeloArtifact executions needed with optimization
	executions := s.optimizeVeloExecutions(groupID, &diff, currentPolicyIDs, request.PolicyIDs)

	// Execute VeloArtifact operations first (supports partial success)
	veloResult := s.executeVeloArtifactOperations(ctx, executions)

	// Check if all VeloArtifact operations failed
	if veloResult.TotalSucceeded == 0 && veloResult.TotalAttempted > 0 {
		return nil, fmt.Errorf("all VeloArtifact operations failed (%d/%d)", veloResult.TotalFailed, veloResult.TotalAttempted)
	}

	// Apply database changes (at least some VeloArtifact operations succeeded)
	if err := s.applyDatabaseChanges(tx, groupID, &diff); err != nil {
		return nil, fmt.Errorf("failed to apply database changes: %v", err)
	}

	// Sync dbpolicy table based on changes
	if err := s.syncDBPolicyForGroupUpdates(tx, groupID, &diff, currentActorIDs); err != nil {
		return nil, fmt.Errorf("failed to sync dbpolicy: %v", err)
	}

	result.VeloJobsCreated = veloResult.SuccessfulJobs
	result.TotalExecutions = veloResult.TotalAttempted

	// Commit transaction only after successful VeloArtifact operations
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}
	txCommitted = true

	// Export DBF policy rules after dbpolicy changes (run in background)
	logger.Infof("Starting background exportDBFPolicy to build rule files for group %d assignments update", groupID)
	go func(gID uint) {
		if err := utils.ExportDBFPolicy(); err != nil {
			logger.Warnf("Failed to export DBF policy rules for group %d: %v", gID, err)
		} else {
			logger.Infof("Successfully exported DBF policy rules for group %d assignments update", gID)
		}
	}(groupID)

	logger.Infof("Bulk update completed for group %d: %d policies added, %d policies removed, %d actors added, %d actors removed, VeloArtifact: %d succeeded, %d failed",
		groupID, len(diff.PoliciesToAdd), len(diff.PoliciesToRemove), len(diff.ActorsToAdd), len(diff.ActorsToRemove), veloResult.TotalSucceeded, veloResult.TotalFailed)

	return result, nil
}

// Helper methods for optimized bulk update

func (s *groupManagementService) getCurrentPolicyIDs(groupID uint) ([]uint, error) {
	policyGroups, err := s.policyGroupsRepo.GetActivePoliciesByGroupID(nil, groupID)
	if err != nil {
		return nil, err
	}

	var policyIDs []uint
	for _, pg := range policyGroups {
		policyIDs = append(policyIDs, pg.DBGroupListPoliciesID)
	}
	return policyIDs, nil
}

func (s *groupManagementService) getCurrentActorIDs(groupID uint) ([]uint, error) {
	actorGroups, err := s.actorGroupsRepo.GetActiveActorsByGroupID(nil, groupID)
	if err != nil {
		return nil, err
	}

	var actorIDs []uint
	for _, ag := range actorGroups {
		actorIDs = append(actorIDs, ag.ActorID)
	}
	return actorIDs, nil
}

func (s *groupManagementService) calculateDiff(currentPolicyIDs, currentActorIDs, targetPolicyIDs, targetActorIDs []uint) diffResult {
	// Convert to maps for O(1) lookup
	currentPolicies := make(map[uint]bool)
	for _, id := range currentPolicyIDs {
		currentPolicies[id] = true
	}

	targetPolicies := make(map[uint]bool)
	for _, id := range targetPolicyIDs {
		targetPolicies[id] = true
	}

	currentActors := make(map[uint]bool)
	for _, id := range currentActorIDs {
		currentActors[id] = true
	}

	targetActors := make(map[uint]bool)
	for _, id := range targetActorIDs {
		targetActors[id] = true
	}

	diff := diffResult{}

	// Find policies to add (in target but not in current)
	for _, id := range targetPolicyIDs {
		if !currentPolicies[id] {
			diff.PoliciesToAdd = append(diff.PoliciesToAdd, id)
		}
	}

	// Find policies to remove (in current but not in target)
	for _, id := range currentPolicyIDs {
		if !targetPolicies[id] {
			diff.PoliciesToRemove = append(diff.PoliciesToRemove, id)
		}
	}

	// Find actors to add (in target but not in current)
	for _, id := range targetActorIDs {
		if !currentActors[id] {
			diff.ActorsToAdd = append(diff.ActorsToAdd, id)
		}
	}

	// Find actors to remove (in current but not in target)
	for _, id := range currentActorIDs {
		if !targetActors[id] {
			diff.ActorsToRemove = append(diff.ActorsToRemove, id)
		}
	}

	return diff
}

func (s *groupManagementService) applyDatabaseChanges(tx interface{}, groupID uint, diff *diffResult) error {
	// Type assertion to *gorm.DB
	gormTx, ok := tx.(*gorm.DB)
	if !ok {
		return fmt.Errorf("invalid transaction type")
	}
	now := time.Now()

	// Add new policy assignments
	for _, policyID := range diff.PoliciesToAdd {
		// Check if assignment already exists to prevent duplicates
		var existingCount int64
		if err := gormTx.Model(&models.DBPolicyGroups{}).
			Where("group_id = ? AND dbgroup_listpolicies_id = ?", groupID, policyID).
			Count(&existingCount).Error; err != nil {
			return fmt.Errorf("failed to check existing policy assignment for policy %d, group %d: %v", policyID, groupID, err)
		}

		if existingCount > 0 {
			logger.Warnf("Policy assignment already exists for policy %d and group %d, skipping create", policyID, groupID)
			continue
		}

		policyGroup := &models.DBPolicyGroups{
			DBGroupListPoliciesID: policyID,
			GroupID:               groupID,
			ValidFrom:             now,
			IsActive:              true,
		}

		if err := s.policyGroupsRepo.Create(gormTx, policyGroup); err != nil {
			return fmt.Errorf("failed to assign policy %d to group %d: %v", policyID, groupID, err)
		}
	}

	// Remove policy assignments
	for _, policyID := range diff.PoliciesToRemove {
		if err := s.policyGroupsRepo.DeactivateByGroupIDAndPolicyID(gormTx, groupID, policyID); err != nil {
			return fmt.Errorf("failed to remove policy %d from group %d: %v", policyID, groupID, err)
		}
	}

	// Add new actor assignments
	for _, actorID := range diff.ActorsToAdd {
		// Check if assignment already exists to prevent duplicates
		var existingCount int64
		if err := gormTx.Model(&models.DBActorGroups{}).
			Where("group_id = ? AND actor_id = ?", groupID, actorID).
			Count(&existingCount).Error; err != nil {
			return fmt.Errorf("failed to check existing actor assignment for actor %d, group %d: %v", actorID, groupID, err)
		}

		if existingCount > 0 {
			logger.Warnf("Actor assignment already exists for actor %d and group %d, skipping create", actorID, groupID)
			continue
		}

		actorGroup := &models.DBActorGroups{
			ActorID:   actorID,
			GroupID:   groupID,
			ValidFrom: now,
			IsActive:  true,
		}

		if err := s.actorGroupsRepo.Create(gormTx, actorGroup); err != nil {
			return fmt.Errorf("failed to assign actor %d to group %d: %v", actorID, groupID, err)
		}
	}

	// Remove actor assignments
	for _, actorID := range diff.ActorsToRemove {
		if err := s.actorGroupsRepo.DeactivateByActorIDAndGroupID(gormTx, actorID, groupID); err != nil {
			return fmt.Errorf("failed to remove actor %d from group %d: %v", actorID, groupID, err)
		}
	}

	return nil
}

func (s *groupManagementService) optimizeVeloExecutions(groupID uint, diff *diffResult, currentPolicyIDs, finalPolicyIDs []uint) []VeloArtifactExecution {
	var executions []VeloArtifactExecution

	// Get current actors to understand the actor changes
	currentActors, err := s.GetActiveActorsForGroup(context.Background(), groupID)
	if err != nil {
		logger.Errorf("Failed to get current actors for group %d during optimization: %v", groupID, err)
		return executions
	}

	// Convert actor lists for easier processing
	currentActorsByID := make(map[uint]models.DBActorMgt)
	for _, actor := range currentActors {
		currentActorsByID[actor.ID] = actor
	}

	// Get all actors (current + new) that will be involved in executions
	var actorsToRemove []models.DBActorMgt
	var actorsToKeep []models.DBActorMgt
	var actorsToAdd []models.DBActorMgt

	// 1. Actors to remove: need deny for current policies
	for _, actorID := range diff.ActorsToRemove {
		if actor, exists := currentActorsByID[actorID]; exists {
			actorsToRemove = append(actorsToRemove, actor)
		}
	}

	// 2. Actors to keep: need deny for removed policies + allow for added policies
	// Calculate actors staying (in both current and target)
	for _, actor := range currentActors {
		actorID := actor.ID
		// Check if this actor is NOT being removed
		isBeingRemoved := false
		for _, removedID := range diff.ActorsToRemove {
			if actorID == removedID {
				isBeingRemoved = true
				break
			}
		}
		if !isBeingRemoved {
			actorsToKeep = append(actorsToKeep, actor)
		}
	}

	// 3. Actors to add: need allow for final policies
	for _, actorID := range diff.ActorsToAdd {
		// Get actor details
		actor, err := s.actorMgtRepo.GetByID(nil, actorID)
		if err != nil {
			logger.Errorf("Failed to get actor %d details: %v", actorID, err)
			continue
		}
		actorsToAdd = append(actorsToAdd, *actor)
	}

	// Group actors by connection for each scenario
	actorsToRemoveByConnection := s.groupActorsByConnection(actorsToRemove)
	actorsToKeepByConnection := s.groupActorsByConnection(actorsToKeep)
	actorsToAddByConnection := s.groupActorsByConnection(actorsToAdd)

	// Scenario 1: For actors being removed - deny all current policies
	for cntID, connectionActors := range actorsToRemoveByConnection {
		if len(currentPolicyIDs) > 0 {
			batchSQLCommands, err := s.buildOptimizedBatchSQL(currentPolicyIDs, connectionActors, "deny", cntID)
			if err != nil {
				logger.Errorf("Failed to build deny commands for removed actors on connection %d: %v", cntID, err)
				continue
			}

			if len(batchSQLCommands) > 0 {
				executions = append(executions, VeloArtifactExecution{
					Operation:        "deny",
					GroupID:          groupID,
					ConnectionID:     cntID,
					BatchSQLCommands: batchSQLCommands,
				})
				logger.Debugf("Added deny execution for removed actors on connection %d: %d commands", cntID, len(batchSQLCommands))
			}
		}
	}

	// Scenario 2: For actors staying - deny removed policies + allow added policies
	for cntID, connectionActors := range actorsToKeepByConnection {
		// Deny removed policies for staying actors
		if len(diff.PoliciesToRemove) > 0 {
			batchSQLCommands, err := s.buildOptimizedBatchSQL(diff.PoliciesToRemove, connectionActors, "deny", cntID)
			if err != nil {
				logger.Errorf("Failed to build deny commands for staying actors on connection %d: %v", cntID, err)
				continue
			}

			if len(batchSQLCommands) > 0 {
				executions = append(executions, VeloArtifactExecution{
					Operation:        "deny",
					GroupID:          groupID,
					ConnectionID:     cntID,
					BatchSQLCommands: batchSQLCommands,
				})
				logger.Debugf("Added deny execution for staying actors on connection %d: %d commands", cntID, len(batchSQLCommands))
			}
		}

		// Allow added policies for staying actors
		if len(diff.PoliciesToAdd) > 0 {
			batchSQLCommands, err := s.buildOptimizedBatchSQL(diff.PoliciesToAdd, connectionActors, "allow", cntID)
			if err != nil {
				logger.Errorf("Failed to build allow commands for staying actors on connection %d: %v", cntID, err)
				continue
			}

			if len(batchSQLCommands) > 0 {
				executions = append(executions, VeloArtifactExecution{
					Operation:        "allow",
					GroupID:          groupID,
					ConnectionID:     cntID,
					BatchSQLCommands: batchSQLCommands,
				})
				logger.Debugf("Added allow execution for staying actors on connection %d: %d commands", cntID, len(batchSQLCommands))
			}
		}
	}

	// Scenario 3: For actors being added - allow all final policies
	for cntID, connectionActors := range actorsToAddByConnection {
		if len(finalPolicyIDs) > 0 {
			batchSQLCommands, err := s.buildOptimizedBatchSQL(finalPolicyIDs, connectionActors, "allow", cntID)
			if err != nil {
				logger.Errorf("Failed to build allow commands for added actors on connection %d: %v", cntID, err)
				continue
			}

			if len(batchSQLCommands) > 0 {
				executions = append(executions, VeloArtifactExecution{
					Operation:        "allow",
					GroupID:          groupID,
					ConnectionID:     cntID,
					BatchSQLCommands: batchSQLCommands,
				})
				logger.Debugf("Added allow execution for added actors on connection %d: %d commands", cntID, len(batchSQLCommands))
			}
		}
	}

	logger.Infof("Optimized VeloArtifact executions for group %d: %d total executions, %d actors removed, %d actors staying, %d actors added",
		groupID, len(executions), len(actorsToRemove), len(actorsToKeep), len(actorsToAdd))

	return executions
}

func (s *groupManagementService) collectPolicyDefaultIDs(policyIDs []uint) []uint {
	var allDefaultIDs []uint

	for _, policyID := range policyIDs {
		// Get policy from bootstrap data
		if policy, exists := bootstrap.DBGroupListPoliciesAllMap[policyID]; exists {
			// Parse dbpolicydefault_id field which contains comma-separated IDs
			if policy.DBPolicyDefaultID != nil {
				defaultIDs := parsePolicyDefaultIDs(*policy.DBPolicyDefaultID)
				allDefaultIDs = append(allDefaultIDs, defaultIDs...)
			}
		}
	}

	// Remove duplicates
	return removeDuplicateUints(allDefaultIDs)
}

// Helper function to parse comma-separated policy default IDs from string
func parsePolicyDefaultIDs(idString string) []uint {
	// Parses comma-separated string like "10, 50, 52, 53, 54, 56, 100, 104, 105, 111, 112, 120, 121, 122, 123, 137, 158, 159, 162"
	if strings.TrimSpace(idString) == "" {
		return []uint{}
	}

	parts := strings.Split(idString, ",")
	var result []uint

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}

		if id, err := strconv.ParseUint(trimmed, 10, 32); err == nil {
			result = append(result, uint(id))
		} else {
			logger.Warnf("Failed to parse policy default ID '%s': %v", trimmed, err)
		}
	}

	return result
}

// Helper function to remove duplicate uints from slice
func removeDuplicateUints(slice []uint) []uint {
	keys := make(map[uint]bool)
	var result []uint

	for _, item := range slice {
		if !keys[item] {
			keys[item] = true
			result = append(result, item)
		}
	}

	return result
}

func (s *groupManagementService) executeVeloArtifactOperations(ctx context.Context, executions []VeloArtifactExecution) *VeloExecutionResult {
	result := &VeloExecutionResult{
		SuccessfulJobs: []string{},
		FailedJobs:     []VeloExecutionError{},
		TotalAttempted: len(executions),
		TotalSucceeded: 0,
		TotalFailed:    0,
	}

	for _, execution := range executions {
		// Execute batch SQL commands for each connection
		jobID, err := s.executeBatchSQLForConnection(ctx, execution)
		if err != nil {
			// Record failure but continue with other executions
			result.FailedJobs = append(result.FailedJobs, VeloExecutionError{
				ConnectionID: execution.ConnectionID,
				Operation:    execution.Operation,
				Error:        err.Error(),
			})
			result.TotalFailed++
			logger.Errorf("VeloArtifact execution failed for group %d, connection %d, operation %s: %v",
				execution.GroupID, execution.ConnectionID, execution.Operation, err)
		} else {
			// Record success
			result.SuccessfulJobs = append(result.SuccessfulJobs, jobID)
			result.TotalSucceeded++
			logger.Infof("VeloArtifact execution succeeded for connection %d: job_id=%s", execution.ConnectionID, jobID)
		}
	}

	logger.Infof("VeloArtifact batch execution completed: %d succeeded, %d failed out of %d total",
		result.TotalSucceeded, result.TotalFailed, result.TotalAttempted)
	return result
}

// groupActorsByConnection groups actors by their connection ID (cntid) for batch processing
func (s *groupManagementService) groupActorsByConnection(actors []models.DBActorMgt) map[uint][]models.DBActorMgt {
	actorsByConnection := make(map[uint][]models.DBActorMgt)

	for _, actor := range actors {
		// Use actor's CntID to group them
		cntID := actor.CntID
		actorsByConnection[cntID] = append(actorsByConnection[cntID], actor)
	}

	logger.Debugf("Grouped %d actors into %d connections", len(actors), len(actorsByConnection))
	for cntID, connectionActors := range actorsByConnection {
		logger.Debugf("Connection %d has %d actors", cntID, len(connectionActors))
	}

	return actorsByConnection
}

// buildOptimizedBatchSQL builds optimized batch SQL commands with hex decode and template substitution
// This method handles the complete processing pipeline at optimization time
func (s *groupManagementService) buildOptimizedBatchSQL(policyIDs []uint, actors []models.DBActorMgt, operation string, cntID uint) ([]string, error) {
	var batchSQLCommands []string

	// Get connection info to determine Oracle CDB/PDB type
	cmt, err := s.cntMgtRepo.GetCntMgtByID(nil, cntID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection %d: %v", cntID, err)
	}

	// Determine objecttype wildcard for Oracle connections
	objectTypeWildcard := "*" // Default for non-Oracle
	isOracle := strings.ToLower(cmt.CntType) == "oracle"
	if isOracle {
		connType := GetOracleConnectionType(cmt)
		objectTypeWildcard = GetObjectTypeWildcard(connType)
		logger.Debugf("Oracle connection detected: cntID=%d, cntType=%s, connType=%s, objectTypeWildcard=%s",
			cntID, cmt.CntType, connType.String(), objectTypeWildcard)
	}

	// Get policy default IDs from policy list
	policyDefaultIDs := s.collectPolicyDefaultIDs(policyIDs)

	for _, policyDefaultID := range policyDefaultIDs {
		// Get policy template from bootstrap data
		policyDefault, exists := bootstrap.DBPolicyDefaultsAllMap[policyDefaultID]
		if !exists {
			logger.Warnf("Policy default %d not found in bootstrap data", policyDefaultID)
			continue
		}

		// Choose the correct SQL command based on operation
		var sqlCmd string
		if operation == "allow" {
			sqlCmd = policyDefault.SqlUpdateAllow
		} else if operation == "deny" {
			sqlCmd = policyDefault.SqlUpdateDeny
		} else {
			logger.Warnf("Invalid operation %s for policy_default=%d", operation, policyDefaultID)
			continue
		}

		if strings.TrimSpace(sqlCmd) == "" {
			logger.Warnf("No SQL command found for policy_default=%d, operation=%s", policyDefaultID, operation)
			continue
		}

		// Hex decode the SQL command following DBPolicyService pattern
		sqlBytes, err := hex.DecodeString(sqlCmd)
		if err != nil {
			logger.Errorf("Hex decode error for policy_default=%d: %v", policyDefaultID, err)
			continue
		}

		rawSQL := string(sqlBytes)

		// Generate SQL commands for each actor with this policy template
		for _, actor := range actors {
			// Template substitution following DBPolicyService pattern
			executeSql := strings.ReplaceAll(rawSQL, "${dbmgt.dbname}", "*")                         // Use wildcard since no SetDatabase()
			executeSql = strings.ReplaceAll(executeSql, "${dbobjectmgt.objectname}", "*")            // Use wildcard for group policies
			executeSql = strings.ReplaceAll(executeSql, "${dbactormgt.dbuser}", actor.DBUser)        // Actor-specific user
			executeSql = strings.ReplaceAll(executeSql, "${dbactormgt.ip_address}", actor.IPAddress) // Actor-specific IP

			// Oracle-specific: replace ${dbobject.objecttype} with CDB/PDB scope wildcard
			if isOracle {
				beforeReplace := executeSql
				executeSql = strings.ReplaceAll(executeSql, "${dbobject.objecttype}", objectTypeWildcard)
				if beforeReplace != executeSql {
					logger.Debugf("Replaced ${dbobject.objecttype} with '%s' for policy_default=%d", objectTypeWildcard, policyDefaultID)
				}
				// Oracle CDB/PDB scope substitution: CDB→"*", PDB→"PDB"
				executeSql = strings.ReplaceAll(executeSql, "${scope}", objectTypeWildcard)
			}

			// Add the processed SQL command to batch
			batchSQLCommands = append(batchSQLCommands, executeSql)
		}
	}

	logger.Debugf("Generated %d optimized SQL commands (%d policies × %d actors, operation=%s)",
		len(batchSQLCommands), len(policyDefaultIDs), len(actors), operation)

	return batchSQLCommands, nil
}

// executeBatchSQLForConnection executes pre-processed batch SQL commands for a connection
// This implements the optimized approach: SQL commands are already processed in optimizeVeloExecutions
// Splits large batches into smaller chunks to avoid "argument list too long" error on Linux
func (s *groupManagementService) executeBatchSQLForConnection(ctx context.Context, execution VeloArtifactExecution) (string, error) {
	if len(execution.BatchSQLCommands) == 0 {
		logger.Warnf("No batch SQL commands for connection %d, skipping execution", execution.ConnectionID)
		return fmt.Sprintf("skipped_connection_%d_no_commands_%d", execution.ConnectionID, time.Now().Unix()), nil
	}

	// Get connection details
	cmt, err := s.cntMgtRepo.GetCntMgtByID(nil, execution.ConnectionID)
	if err != nil {
		return "", fmt.Errorf("connection %d not found: %v", execution.ConnectionID, err)
	}

	// Get endpoint information
	ep, err := s.endpointRepo.GetByID(nil, utils.MustIntToUint(cmt.Agent))
	if err != nil {
		return "", fmt.Errorf("cannot find endpoint with id=%d for connection %d: %v", cmt.Agent, execution.ConnectionID, err)
	}

	// Split batch into smaller chunks to avoid "argument list too long" error
	// Linux ARG_MAX is typically 128KB-2MB, but we use conservative 50KB limit for hex-encoded JSON
	const maxBatchSize = 50 // Max commands per batch to keep under arg limit

	var allJobIDs []string
	totalCommands := len(execution.BatchSQLCommands)
	batchNum := 0

	for i := 0; i < totalCommands; i += maxBatchSize {
		batchNum++
		end := i + maxBatchSize
		if end > totalCommands {
			end = totalCommands
		}

		batchCommands := execution.BatchSQLCommands[i:end]

		// Join commands in this batch
		// Oracle PL/SQL blocks need to be separated by newline+/+newline
		// MySQL/other databases use space separator
		var separator string
		if strings.ToLower(cmt.CntType) == "oracle" {
			separator = "\n/\n"
		} else {
			separator = " "
		}
		combinedSQL := strings.Join(batchCommands, separator)

		// Build query parameters
		queryParamBuilder := dto.NewDBQueryParamBuilder().
			SetDBType(strings.ToLower(cmt.CntType)).
			SetHost(cmt.IP).
			SetPort(cmt.Port).
			SetUser(cmt.Username).
			SetPassword(cmt.Password)

		// Oracle requires ServiceName for connection
		if strings.ToLower(cmt.CntType) == "oracle" && cmt.ServiceName != "" {
			queryParamBuilder = queryParamBuilder.SetDatabase(cmt.ServiceName)
		}

		queryParam := queryParamBuilder.Build()

		queryParam.Query = combinedSQL

		logger.Infof("Executing batch %d/%d for connection %d: %d commands, operation=%s",
			batchNum, (totalCommands+maxBatchSize-1)/maxBatchSize, execution.ConnectionID, len(batchCommands), execution.Operation)
		logger.Debugf("Batch %d SQL for connection %d: %s", batchNum, execution.ConnectionID, combinedSQL)

		// Create agent command JSON
		hexJSON, err := utils.CreateAgentCommandJSON(queryParam)
		if err != nil {
			return "", fmt.Errorf("failed to create agent command JSON for connection %d batch %d: %v", execution.ConnectionID, batchNum, err)
		}

		// Execute via agent API with batch SQL
		result, err := agent.ExecuteSqlAgentAPI(ep.ClientID, ep.OsType, "execute", hexJSON, "", false)
		if err != nil {
			return "", fmt.Errorf("executeSqlAgentAPI error for connection %d batch %d: %v", execution.ConnectionID, batchNum, err)
		}

		jobID := fmt.Sprintf("batch_%d_conn_%d_group_%d_%s_%d",
			batchNum, execution.ConnectionID, execution.GroupID, execution.Operation, time.Now().Unix())
		allJobIDs = append(allJobIDs, jobID)

		logger.Infof("Batch %d executed successfully for connection %d: job_id=%s, result=%s",
			batchNum, execution.ConnectionID, jobID, result)
	}

	// Return combined job ID for all batches
	finalJobID := fmt.Sprintf("multi_batch_conn_%d_group_%d_%s_%d_batches_%d_commands_%d",
		execution.ConnectionID, execution.GroupID, execution.Operation,
		len(allJobIDs), totalCommands, time.Now().Unix())

	logger.Infof("All %d batches executed successfully for connection %d: final_job_id=%s, total_commands=%d",
		len(allJobIDs), execution.ConnectionID, finalJobID, totalCommands)

	return finalJobID, nil
}

// Helper functions for dbpolicy synchronization

// PolicyOperationType defines the type of policy operation for extraction logic.
type PolicyOperationType string

// Policy operation type constants.
const (
	PolicyOperationAssign PolicyOperationType = "assign" // Use dbpolicydefault_id column
	PolicyOperationRemove PolicyOperationType = "remove" // Use contained_policydefaults column
)

// extractPolicyDefaultIDs extracts all policy default IDs from a list of group policies.
// For assign operations: uses dbpolicydefault_id field
// For remove operations: uses contained_policydefaults field
func (s *groupManagementService) extractPolicyDefaultIDs(policyIDs []uint, operationType PolicyOperationType) ([]uint, error) {
	if len(policyIDs) == 0 {
		return []uint{}, nil
	}

	var allPolicyDefaultIDs []uint
	seenIDs := make(map[uint]bool)

	for _, policyID := range policyIDs {
		// Get policy from database
		policy, err := s.groupListPoliciesRepo.GetByID(nil, policyID)
		if err != nil {
			logger.Warnf("Policy ID %d not found: %v", policyID, err)
			continue
		}

		// Select field based on operation type
		var policyDefaultStr string
		if operationType == PolicyOperationAssign {
			// ASSIGN: use dbpolicydefault_id column
			if policy.DBPolicyDefaultID != nil && strings.TrimSpace(*policy.DBPolicyDefaultID) != "" {
				policyDefaultStr = *policy.DBPolicyDefaultID
			}
		} else {
			// REMOVE: use contained_policydefaults column
			if policy.ContainedPolicydefaults != nil && strings.TrimSpace(*policy.ContainedPolicydefaults) != "" {
				policyDefaultStr = *policy.ContainedPolicydefaults
			}
		}

		if policyDefaultStr == "" {
			continue
		}

		// Parse comma-separated IDs
		policyDefaultIDs := parsePolicyDefaultIDs(policyDefaultStr)
		for _, id := range policyDefaultIDs {
			if !seenIDs[id] {
				seenIDs[id] = true
				allPolicyDefaultIDs = append(allPolicyDefaultIDs, id)
			}
		}
	}

	return allPolicyDefaultIDs, nil
}

// syncDBPolicyForActors creates dbpolicy records for actors with given policy defaults.
// Used when assigning actors to groups - creates actor-wide policies (dbmgt=-1, object=-1).
func (s *groupManagementService) syncDBPolicyForActors(tx *gorm.DB, actors []models.DBActorMgt, policyDefaultIDs []uint) (int, error) {
	if len(actors) == 0 || len(policyDefaultIDs) == 0 {
		return 0, nil
	}

	var policies []models.DBPolicy
	for _, actor := range actors {
		for _, policyDefaultID := range policyDefaultIDs {
			policy := models.DBPolicy{
				CntMgt:          actor.CntID,
				DBPolicyDefault: policyDefaultID,
				DBMgt:           -1,
				DBActorMgt:      actor.ID,
				DBObjectMgt:     -1,
				Status:          "enabled",
				Description:     "Auto-inserted by Group Policy",
			}
			policies = append(policies, policy)
		}
	}

	// Insert with duplicate check
	insertedCount, err := s.dbpolicyRepo.BulkCreateWithDuplicateCheck(tx, policies)
	if err != nil {
		return 0, fmt.Errorf("failed to sync dbpolicy records: %v", err)
	}

	logger.Infof("Synced dbpolicy: %d records inserted for %d actors × %d policy defaults",
		insertedCount, len(actors), len(policyDefaultIDs))

	return insertedCount, nil
}

// cleanupDBPolicyForActors removes dbpolicy records for actors with given policy defaults.
// Used when removing actors from groups or removing policies from groups.
func (s *groupManagementService) cleanupDBPolicyForActors(tx *gorm.DB, actors []models.DBActorMgt, policyDefaultIDs []uint) error {
	if len(actors) == 0 || len(policyDefaultIDs) == 0 {
		return nil
	}

	deletedCount := 0
	for _, actor := range actors {
		err := s.dbpolicyRepo.DeleteByActorAndPolicyDefaults(tx, actor.CntID, actor.ID, policyDefaultIDs)
		if err != nil {
			return fmt.Errorf("failed to cleanup dbpolicy for actor %d: %v", actor.ID, err)
		}
		deletedCount++
	}

	logger.Infof("Cleaned up dbpolicy: deleted records for %d actors × %d policy defaults",
		len(actors), len(policyDefaultIDs))

	return nil
}

// syncDBPolicyForGroupUpdates synchronizes dbpolicy table based on group assignment changes.
// Handles 4 scenarios from UpdateGroupAssignments:
// 1. Policies added → sync new policies for staying actors
// 2. Policies removed → cleanup removed policies for staying actors
// 3. Actors added → sync all final policies for new actors
// 4. Actors removed → cleanup all current policies for removed actors
func (s *groupManagementService) syncDBPolicyForGroupUpdates(tx *gorm.DB, groupID uint, diff *diffResult, currentActorIDs []uint) error {
	// Calculate staying actors (actors that were not added or removed)
	stayingActorIDs := []uint{}
	for _, actorID := range currentActorIDs {
		isBeingRemoved := false
		for _, removedID := range diff.ActorsToRemove {
			if actorID == removedID {
				isBeingRemoved = true
				break
			}
		}
		if !isBeingRemoved {
			stayingActorIDs = append(stayingActorIDs, actorID)
		}
	}

	// Scenario 1 & 2: Policies changed → update staying actors
	if len(diff.PoliciesToAdd) > 0 && len(stayingActorIDs) > 0 {
		// Extract policy defaults from added policies (ASSIGN operation uses dbpolicydefault_id)
		addedPolicyDefaults, err := s.extractPolicyDefaultIDs(diff.PoliciesToAdd, PolicyOperationAssign)
		if err != nil {
			return fmt.Errorf("failed to extract added policy defaults: %v", err)
		}

		if len(addedPolicyDefaults) > 0 {
			// Get staying actor details
			var stayingActors []models.DBActorMgt
			for _, actorID := range stayingActorIDs {
				actor, err := s.actorMgtRepo.GetByID(nil, actorID)
				if err != nil {
					logger.Warnf("Actor ID %d not found: %v", actorID, err)
					continue
				}
				stayingActors = append(stayingActors, *actor)
			}

			// Sync new policies for staying actors
			insertedCount, err := s.syncDBPolicyForActors(tx, stayingActors, addedPolicyDefaults)
			if err != nil {
				return fmt.Errorf("failed to sync added policies: %v", err)
			}
			logger.Infof("Group %d policy add: synced %d dbpolicy records for %d staying actors",
				groupID, insertedCount, len(stayingActors))
		}
	}

	if len(diff.PoliciesToRemove) > 0 && len(stayingActorIDs) > 0 {
		// Extract policy defaults from removed policies (REMOVE operation uses contained_policydefaults)
		removedPolicyDefaults, err := s.extractPolicyDefaultIDs(diff.PoliciesToRemove, PolicyOperationRemove)
		if err != nil {
			return fmt.Errorf("failed to extract removed policy defaults: %v", err)
		}

		if len(removedPolicyDefaults) > 0 {
			// Get staying actor details
			var stayingActors []models.DBActorMgt
			for _, actorID := range stayingActorIDs {
				actor, err := s.actorMgtRepo.GetByID(nil, actorID)
				if err != nil {
					logger.Warnf("Actor ID %d not found: %v", actorID, err)
					continue
				}
				stayingActors = append(stayingActors, *actor)
			}

			// Cleanup removed policies for staying actors
			if err := s.cleanupDBPolicyForActors(tx, stayingActors, removedPolicyDefaults); err != nil {
				return fmt.Errorf("failed to cleanup removed policies: %v", err)
			}
			logger.Infof("Group %d policy remove: cleaned up dbpolicy records for %d staying actors",
				groupID, len(stayingActors))
		}
	}

	// Scenario 3: Actors added → sync all final policies
	if len(diff.ActorsToAdd) > 0 {
		// Get all final policies for the group (after changes applied)
		finalPolicyIDs, err := s.getCurrentPolicyIDs(groupID)
		if err != nil {
			return fmt.Errorf("failed to get final policies: %v", err)
		}

		if len(finalPolicyIDs) > 0 {
			// Extract final policies for new actors (ASSIGN operation uses dbpolicydefault_id)
			finalPolicyDefaults, err := s.extractPolicyDefaultIDs(finalPolicyIDs, PolicyOperationAssign)
			if err != nil {
				return fmt.Errorf("failed to extract final policy defaults: %v", err)
			}

			if len(finalPolicyDefaults) > 0 {
				// Get added actor details
				var addedActors []models.DBActorMgt
				for _, actorID := range diff.ActorsToAdd {
					actor, err := s.actorMgtRepo.GetByID(nil, actorID)
					if err != nil {
						logger.Warnf("Actor ID %d not found: %v", actorID, err)
						continue
					}
					addedActors = append(addedActors, *actor)
				}

				// Sync final policies for new actors
				insertedCount, err := s.syncDBPolicyForActors(tx, addedActors, finalPolicyDefaults)
				if err != nil {
					return fmt.Errorf("failed to sync policies for added actors: %v", err)
				}
				logger.Infof("Group %d actor add: synced %d dbpolicy records for %d new actors",
					groupID, insertedCount, len(addedActors))
			}
		}
	}

	// Scenario 4: Actors removed → cleanup all current policies
	if len(diff.ActorsToRemove) > 0 {
		// Since applyDatabaseChanges already modified group, we use union of removed and staying policies
		// to ensure complete cleanup for removed actors
		allPolicyIDsToCleanup := append([]uint{}, diff.PoliciesToRemove...)
		// Add back any policies that are staying (not being removed)
		currentPoliciesPostChange, err := s.getCurrentPolicyIDs(groupID)
		if err != nil {
			logger.Warnf("Failed to get current policies for cleanup: %v", err)
		} else {
			allPolicyIDsToCleanup = append(allPolicyIDsToCleanup, currentPoliciesPostChange...)
		}

		if len(allPolicyIDsToCleanup) > 0 {
			// Extract policies for cleanup (REMOVE operation uses contained_policydefaults)
			policyDefaultsToCleanup, err := s.extractPolicyDefaultIDs(allPolicyIDsToCleanup, PolicyOperationRemove)
			if err != nil {
				return fmt.Errorf("failed to extract policy defaults for cleanup: %v", err)
			}

			if len(policyDefaultsToCleanup) > 0 {
				// Get removed actor details
				var removedActors []models.DBActorMgt
				for _, actorID := range diff.ActorsToRemove {
					actor, err := s.actorMgtRepo.GetByID(nil, actorID)
					if err != nil {
						logger.Warnf("Actor ID %d not found: %v", actorID, err)
						continue
					}
					removedActors = append(removedActors, *actor)
				}

				// Cleanup all policies for removed actors
				if err := s.cleanupDBPolicyForActors(tx, removedActors, policyDefaultsToCleanup); err != nil {
					return fmt.Errorf("failed to cleanup policies for removed actors: %v", err)
				}
				logger.Infof("Group %d actor remove: cleaned up dbpolicy records for %d removed actors",
					groupID, len(removedActors))
			}
		}
	}

	return nil
}
