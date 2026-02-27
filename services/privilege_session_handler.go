package services

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"dbfartifactapi/config"
	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"
	"dbfartifactapi/services/job"
	"dbfartifactapi/utils"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// processedPrivilegeJobs tracks jobs that have already been processed to prevent duplicate execution
var processedPrivilegeJobs sync.Map

// PrivilegeSessionJobContext contains context data for privilege session job completion
type PrivilegeSessionJobContext struct {
	CntMgtID      uint                `json:"cnt_mgt_id"`
	DbMgts        []models.DBMgt      `json:"db_mgts"`
	DbActorMgts   []models.DBActorMgt `json:"db_actor_mgts"`
	CMT           *models.CntMgt      `json:"cmt"`
	EndpointID    uint                `json:"endpoint_id"`
	SessionID     string              `json:"session_id"`
	PrivilegeFile string              `json:"privilege_file"` // File containing privilege data queries
}

// policyClassification categorizes policy templates by execution order and scope
type policyClassification struct {
	superPrivileges     []models.DBPolicyDefault // Execute first, broadest scope (all actions, all objects, all databases)
	actionWidePrivs     []models.DBPolicyDefault // Execute second, action across all objects/databases
	objectSpecificPrivs []models.DBPolicyDefault // Execute last, specific objects/databases
}

// grantedActionsCache tracks which actions have been globally granted to prevent redundant checks
// Key: actorID, Value: set of actionIDs already granted
// Thread-safe for concurrent access from multiple goroutines
type grantedActionsCache struct {
	mu     sync.RWMutex
	grants map[uint]map[int]bool
}

// allowedPolicyResults stores direct query results that passed isPolicyAllowed check.
// Used by assignActorsToGroups instead of reading from dbpolicy table.
// Key: actorID, Value: set of policy_default_ids that were allowed by query evaluation.
// Thread-safe for concurrent access from multiple goroutines
type allowedPolicyResults struct {
	mu            sync.RWMutex
	actorPolicies map[uint]map[uint]bool
}

// superPrivilegeActors tracks actors that passed PASS-1 super privilege check.
// These actors have ALL privileges on ALL objects and should skip PASS-2 and PASS-3.
// Key: actorID, Value: true if actor has super privileges.
// Thread-safe for concurrent access from multiple goroutines.
type superPrivilegeActors struct {
	mu     sync.RWMutex
	actors map[uint]bool
}

// newGrantedActionsCache creates cache to track globally granted actions.
// Prevents redundant policy creation when action already granted at higher scope.
func newGrantedActionsCache() *grantedActionsCache {
	return &grantedActionsCache{
		grants: make(map[uint]map[int]bool),
	}
}

// markGranted records that actionID has been granted to actorID.
// Used by Pass 2 to prevent duplicate Pass 3 policies.
// Thread-safe for concurrent access.
func (c *grantedActionsCache) markGranted(actorID uint, actionID int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.grants[actorID] == nil {
		c.grants[actorID] = make(map[int]bool)
	}
	c.grants[actorID][actionID] = true
}

// isGranted checks if actionID has already been granted to actorID.
// Returns true if grant exists, preventing redundant Pass 3 policy creation.
// Thread-safe for concurrent access.
func (c *grantedActionsCache) isGranted(actorID uint, actionID int) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if actions, exists := c.grants[actorID]; exists {
		return actions[actionID]
	}
	return false
}

// newAllowedPolicyResults creates storage for direct query results that passed isPolicyAllowed.
// Replaces database reads in assignActorsToGroups with in-memory evaluation results.
func newAllowedPolicyResults() *allowedPolicyResults {
	return &allowedPolicyResults{
		actorPolicies: make(map[uint]map[uint]bool),
	}
}

// newSuperPrivilegeActors creates cache to track actors with super privileges from PASS-1.
// Used to skip PASS-2 and PASS-3 for actors that already have all privileges.
func newSuperPrivilegeActors() *superPrivilegeActors {
	return &superPrivilegeActors{
		actors: make(map[uint]bool),
	}
}

// markSuperPrivilege records that actorID has super privileges (passed PASS-1).
// Thread-safe for concurrent access.
func (s *superPrivilegeActors) markSuperPrivilege(actorID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actors[actorID] = true
}

// hasSuperPrivilege checks if actorID has super privileges.
// Returns true if actor passed PASS-1, indicating they should skip PASS-2 and PASS-3.
// Thread-safe for concurrent access.
func (s *superPrivilegeActors) hasSuperPrivilege(actorID uint) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.actors[actorID]
}

// recordAllowed marks policy_default_id as allowed for actorID based on query result.
// Called when isPolicyAllowed returns true during PASS 1, 2, 3 execution.
// Thread-safe for concurrent access.
func (a *allowedPolicyResults) recordAllowed(actorID uint, policyDefaultID uint) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.actorPolicies[actorID] == nil {
		a.actorPolicies[actorID] = make(map[uint]bool)
	}
	a.actorPolicies[actorID][policyDefaultID] = true
}

// classifyPolicyTemplates categorizes policy templates into three execution tiers based on scope for MySQL only.
// Returns classification with policies sorted by execution priority.
func classifyPolicyTemplates(allPolicies map[uint]models.DBPolicyDefault) *policyClassification {
	classification := &policyClassification{
		superPrivileges:     []models.DBPolicyDefault{},
		actionWidePrivs:     []models.DBPolicyDefault{},
		objectSpecificPrivs: []models.DBPolicyDefault{},
	}

	// MySQL database_type_id = 1
	const mysqlDatabaseTypeID = uint(1)

	groupListPolicyRepo := repository.NewDBGroupListPoliciesRepository()
	groupListPolicies, err := groupListPolicyRepo.GetActiveByDatabaseType(nil, mysqlDatabaseTypeID)
	if err != nil {
		logger.Warnf("Failed to load MySQL DBGroupListPolicies: %v", err)
		groupListPolicies = []models.DBGroupListPolicies{}
	}

	logger.Debugf("Loaded %d active MySQL group list policies from database", len(groupListPolicies))

	groupBPolicyIDs := make(map[uint]bool)
	for _, glp := range groupListPolicies {
		if glp.DBPolicyDefaultID == nil || *glp.DBPolicyDefaultID == "" {
			continue
		}

		rawValue := *glp.DBPolicyDefaultID
		var ids []uint

		// Try parsing as JSON array first: [1,2,3]
		if err := json.Unmarshal([]byte(rawValue), &ids); err == nil {
			logger.Debugf("DBGroupListPolicies entry (JSON array): group_id=%v, policy_ids=%v", glp.ID, ids)
			for _, id := range ids {
				groupBPolicyIDs[id] = true
			}
			continue
		}

		// Try parsing as single number: 123
		var singleID uint
		if err := json.Unmarshal([]byte(rawValue), &singleID); err == nil {
			logger.Debugf("DBGroupListPolicies entry (single number): group_id=%v, policy_id=%v", glp.ID, singleID)
			groupBPolicyIDs[singleID] = true
			continue
		}

		// Try parsing as comma-separated string: "1,2,3"
		parts := strings.Split(rawValue, ",")
		parsed := false
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			var id uint
			if _, err := fmt.Sscanf(part, "%d", &id); err == nil {
				ids = append(ids, id)
				parsed = true
			}
		}

		if parsed && len(ids) > 0 {
			logger.Debugf("DBGroupListPolicies entry (comma-separated): group_id=%v, policy_ids=%v", glp.ID, ids)
			for _, id := range ids {
				groupBPolicyIDs[id] = true
			}
			continue
		}

		logger.Warnf("Failed to parse DBPolicyDefaultID for group_id=%v, value=%s", glp.ID, rawValue)
	}

	logger.Infof("Total action-wide policy IDs from DBGroupListPolicies: %d, IDs=%v",
		len(groupBPolicyIDs), groupBPolicyIDs)

	for id, policy := range allPolicies {
		switch {
		case id == 1:
			classification.superPrivileges = append(classification.superPrivileges, policy)
		case groupBPolicyIDs[id]:
			classification.actionWidePrivs = append(classification.actionWidePrivs, policy)
		default:
			classification.objectSpecificPrivs = append(classification.objectSpecificPrivs, policy)
		}
	}

	logger.Debugf("Policy classification: super=%d, action-wide=%d, object-specific=%d",
		len(classification.superPrivileges), len(classification.actionWidePrivs), len(classification.objectSpecificPrivs))

	return classification
}

// groupListPolicy holds a parsed dbgroup_listpolicies entry with its required policy_default_ids.
type groupListPolicy struct {
	listPolicyID     uint
	policyDefaultIDs map[uint]bool
}

// isExactMatch checks if actor has ALL policies that the group requires (group ⊆ actor).
// Returns true only when every policy in groupPolicies exists in actorPolicies.
func isExactMatch(actorPolicies map[uint]bool, groupPolicies map[uint]bool) bool {
	if len(groupPolicies) == 0 || len(actorPolicies) == 0 {
		return false
	}
	for policyID := range groupPolicies {
		if !actorPolicies[policyID] {
			return false
		}
	}
	return true
}

// collectExactMatchGroups finds all groupListPolicies where the actor has every policy the group requires.
// Returns slice of listPolicyIDs that have 100% match (group ⊆ actor).
func collectExactMatchGroups(actorPolicies map[uint]bool, groupListPolicies []groupListPolicy) []uint {
	var exactMatches []uint
	for _, glp := range groupListPolicies {
		if isExactMatch(actorPolicies, glp.policyDefaultIDs) {
			exactMatches = append(exactMatches, glp.listPolicyID)
		}
	}
	return exactMatches
}

// getPolicyIDsAsSlice converts policy ID map to sorted slice for logging.
func getPolicyIDsAsSlice(policyMap map[uint]bool) []uint {
	ids := make([]uint, 0, len(policyMap))
	for id := range policyMap {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// assignActorsToGroups assigns actors to groups using two-level strict exact-match strategy.
// Super privilege actors (passed PASS-1) are auto-assigned to the group linked to "Full" listpolicy.
// Other actors require: Level 1 (actor ⊇ listpolicy policies) + Level 2 (actor satisfies ALL group listpolicies).
// Among matched groups, the lowest group_id is selected.
func assignActorsToGroups(tx *gorm.DB, cntMgtID uint, cmt *models.CntMgt, allowedResults *allowedPolicyResults, superPrivActors *superPrivilegeActors) error {
	logger.Infof("Assigning actors to groups for cnt_id=%d", cntMgtID)

	// Get database_type_id from cmt.CntType
	var dbType models.DBType
	if err := tx.Where("name = ?", cmt.CntType).First(&dbType).Error; err != nil {
		logger.Warnf("Failed to find database type for %s: %v", cmt.CntType, err)
		return nil
	}

	// Use direct query results from PASS 1, 2, 3 execution instead of reading from database
	actorPolicies := allowedResults.actorPolicies

	logger.Debugf("Found %d actors with allowed policies from query evaluation", len(actorPolicies))

	// Load active MySQL dbgroup_listpolicies for matching
	groupListPolicyRepo := repository.NewDBGroupListPoliciesRepository()
	allGroupListPolicies, err := groupListPolicyRepo.GetActiveByDatabaseType(tx, dbType.ID)
	if err != nil {
		logger.Warnf("Failed to load MySQL DBGroupListPolicies for database_type_id=%d: %v", dbType.ID, err)
		return nil
	}

	// Parse dbgroup_listpolicies to extract policy_default_ids
	groupListPolicies := []groupListPolicy{}
	for _, glp := range allGroupListPolicies {
		if glp.DBPolicyDefaultID == nil || *glp.DBPolicyDefaultID == "" {
			continue
		}

		rawValue := *glp.DBPolicyDefaultID
		policyIDs := make(map[uint]bool)

		// Try JSON array: [1,2,3]
		var ids []uint
		if err := json.Unmarshal([]byte(rawValue), &ids); err == nil {
			for _, id := range ids {
				policyIDs[id] = true
			}
		} else {
			// Try single number: 123
			var singleID uint
			if err := json.Unmarshal([]byte(rawValue), &singleID); err == nil {
				policyIDs[singleID] = true
			} else {
				// Try comma-separated: "1,2,3"
				parts := strings.Split(rawValue, ",")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part == "" {
						continue
					}
					var id uint
					if _, err := fmt.Sscanf(part, "%d", &id); err == nil {
						policyIDs[id] = true
					}
				}
			}
		}

		if len(policyIDs) > 0 {
			groupListPolicies = append(groupListPolicies, groupListPolicy{
				listPolicyID:     glp.ID,
				policyDefaultIDs: policyIDs,
			})
		}
	}

	actorGroupsRepo := repository.NewDBActorGroupsRepository()

	// Build group → required listpolicy IDs map from dbpolicy_groups
	// Each group requires ALL its linked listpolicies to be satisfied
	type groupRequirement struct {
		groupID              uint
		requiredListPolicies map[uint]bool
	}
	var policyGroupRows []struct {
		GroupID        uint `gorm:"column:group_id"`
		ListPoliciesID uint `gorm:"column:dbgroup_listpolicies_id"`
	}
	err = tx.Table("dbpolicy_groups pg").
		Select("pg.group_id, pg.dbgroup_listpolicies_id").
		Joins("INNER JOIN dbgroupmgt g ON pg.group_id = g.id").
		Where("pg.is_active = ? AND g.is_active = ? AND g.database_type_id = ?",
			true, true, dbType.ID).
		Order("pg.group_id ASC").
		Find(&policyGroupRows).Error
	if err != nil {
		logger.Warnf("Failed to load dbpolicy_groups: %v", err)
		return nil
	}

	groupRequirements := make(map[uint]*groupRequirement)
	for _, row := range policyGroupRows {
		gr, exists := groupRequirements[row.GroupID]
		if !exists {
			gr = &groupRequirement{
				groupID:              row.GroupID,
				requiredListPolicies: make(map[uint]bool),
			}
			groupRequirements[row.GroupID] = gr
		}
		gr.requiredListPolicies[row.ListPoliciesID] = true
	}

	logger.Debugf("Loaded %d groups with requirements from dbpolicy_groups", len(groupRequirements))
	for gid, gr := range groupRequirements {
		logger.Debugf("Group %d requires %d listpolicies: %v",
			gid, len(gr.requiredListPolicies), getPolicyIDsAsSlice(gr.requiredListPolicies))
	}

	// superPrivGroupID is the group that super privilege actors are directly assigned to.
	// Super privilege actors passed PASS-1 (have ALL privileges) but were skipped in PASS-2/3,
	// so their allowedResults are incomplete. They are hardcoded to group 1.
	const superPrivGroupID = uint(1)

	assignedCount := 0
actorLoop:
	for actorID, actorPolicyIDs := range actorPolicies {
		if len(actorPolicyIDs) == 0 {
			logger.Debugf("Skipping actor %d - no policies", actorID)
			continue
		}

		// Super privilege actors → direct assign to group 1.
		// Detect via policy_default_id=1 in allowedResults (reliable on re-runs),
		// OR via superPrivActors (set during initial PASS-1 policy creation).
		isSuperPriv := actorPolicyIDs[1] || (superPrivActors != nil && superPrivActors.hasSuperPrivilege(actorID))
		if isSuperPriv {
			existingGroups, err := actorGroupsRepo.GetActiveGroupsByActorID(tx, actorID)
			if err == nil {
				for _, ag := range existingGroups {
					if ag.GroupID == superPrivGroupID {
						logger.Infof("Super privilege actor %d already assigned to group %d", actorID, superPrivGroupID)
						continue actorLoop
					}
				}
			}

			actorGroup := &models.DBActorGroups{
				ActorID:   actorID,
				GroupID:   superPrivGroupID,
				ValidFrom: time.Now(),
				IsActive:  true,
			}
			if err := actorGroupsRepo.Create(tx, actorGroup); err != nil {
				logger.Warnf("Failed to assign super privilege actor %d to group %d: %v", actorID, superPrivGroupID, err)
			} else {
				logger.Infof("Assigned super privilege actor %d to group %d (PASS-1 super privilege)", actorID, superPrivGroupID)
				assignedCount++
			}
			continue actorLoop
		}

		// Level 1: Find which dbgroup_listpolicies the actor fully satisfies
		// Actor must have ALL policies in listpolicy.dbpolicydefault_id
		satisfiedListPolicyIDs := collectExactMatchGroups(actorPolicyIDs, groupListPolicies)

		if len(satisfiedListPolicyIDs) == 0 {
			logger.Warnf("No satisfied listpolicies for actor %d (policies: %v) - skipping",
				actorID, getPolicyIDsAsSlice(actorPolicyIDs))
			continue
		}

		logger.Debugf("Actor %d: satisfied %d listpolicies: %v",
			actorID, len(satisfiedListPolicyIDs), satisfiedListPolicyIDs)

		// Level 2: Find groups where actor satisfies ALL required listpolicies
		satisfiedSet := make(map[uint]bool, len(satisfiedListPolicyIDs))
		for _, lpID := range satisfiedListPolicyIDs {
			satisfiedSet[lpID] = true
		}

		var candidateGroupIDs []uint
		for gid, gr := range groupRequirements {
			// Check if actor satisfies ALL listpolicies required by this group
			allSatisfied := true
			for requiredLP := range gr.requiredListPolicies {
				if !satisfiedSet[requiredLP] {
					allSatisfied = false
					break
				}
			}
			if allSatisfied {
				candidateGroupIDs = append(candidateGroupIDs, gid)
			}
		}

		if len(candidateGroupIDs) == 0 {
			logger.Warnf("No fully-matched group for actor %d (satisfied %d/%d listpolicies) - skipping",
				actorID, len(satisfiedListPolicyIDs), len(groupListPolicies))
			continue
		}

		// Select lowest group_id (highest priority)
		sort.Slice(candidateGroupIDs, func(i, j int) bool {
			return candidateGroupIDs[i] < candidateGroupIDs[j]
		})
		selectedGroupID := candidateGroupIDs[0]

		logger.Debugf("Actor %d: matched %d groups %v, selected group %d",
			actorID, len(candidateGroupIDs), candidateGroupIDs, selectedGroupID)

		// Check if actor already assigned to this group
		existingGroups, err := actorGroupsRepo.GetActiveGroupsByActorID(tx, actorID)
		if err == nil {
			for _, ag := range existingGroups {
				if ag.GroupID == selectedGroupID {
					logger.Infof("Actor %d already assigned to group %d", actorID, selectedGroupID)
					continue actorLoop
				}
			}
		}

		// Create new actor-group assignment
		actorGroup := &models.DBActorGroups{
			ActorID:   actorID,
			GroupID:   selectedGroupID,
			ValidFrom: time.Now(),
			IsActive:  true,
		}

		if err := actorGroupsRepo.Create(tx, actorGroup); err != nil {
			logger.Warnf("Failed to assign actor %d to group %d: %v", actorID, selectedGroupID, err)
			continue
		}

		logger.Infof("Assigned actor %d to group %d (exact-match, satisfied all %d required listpolicies)",
			actorID, selectedGroupID, len(groupRequirements[selectedGroupID].requiredListPolicies))
		assignedCount++
	}

	skippedCount := len(actorPolicies) - assignedCount
	logger.Infof("Assignment completed: %d assigned, %d skipped (no exact match)", assignedCount, skippedCount)
	return nil
}

// CreatePrivilegeSessionCompletionHandler creates callback for privilege session job completion
func CreatePrivilegeSessionCompletionHandler() job.JobCompletionCallback {
	return func(jobID string, jobInfo *job.JobInfo, statusResp *job.StatusResponse) error {
		logger.Infof("Processing privilege session completion for job %s, status: %s", jobID, statusResp.Status)

		// Extract context data from job
		contextData, ok := jobInfo.ContextData["privilege_session_context"]
		if !ok {
			return fmt.Errorf("missing privilege session context data for job %s", jobID)
		}

		// Process completed jobs
		if statusResp.Status == "completed" {
			return processPrivilegeSessionResults(jobID, contextData, statusResp, jobInfo)
		} else {
			logger.Errorf("Privilege session job %s failed", jobID)
			return fmt.Errorf("privilege session job failed: %s", statusResp.Message)
		}
	}
}

// processPrivilegeSessionResults processes the results of privilege data loading job
func processPrivilegeSessionResults(jobID string, contextData interface{}, statusResp *job.StatusResponse, jobInfo *job.JobInfo) error {
	logger.Infof("Processing privilege session results for job %s - completed: %d, failed: %d",
		jobID, statusResp.Completed, statusResp.Failed)

	// Extract PrivilegeSessionJobContext from contextData
	sessionContext, ok := contextData.(*PrivilegeSessionJobContext)
	if !ok {
		return fmt.Errorf("invalid privilege session context data for job %s", jobID)
	}

	// Check if notification-based completion
	if notificationData, exists := jobInfo.ContextData["notification_data"]; exists {
		return processPrivilegeSessionFromNotification(jobID, sessionContext, notificationData, statusResp)
	}

	// Legacy VeloArtifact polling flow
	return processPrivilegeSessionFromVeloArtifact(jobID, sessionContext, statusResp)
}

// processPrivilegeSessionFromNotification handles privilege session processing from notification
func processPrivilegeSessionFromNotification(jobID string, sessionContext *PrivilegeSessionJobContext, notificationData interface{}, statusResp *job.StatusResponse) error {
	logger.Infof("Processing privilege session from notification for job %s", jobID)

	jobMonitor := job.GetJobMonitorService()

	// Extract notification data
	notification, ok := notificationData.(map[string]interface{})
	if !ok {
		err := fmt.Errorf("invalid notification data format for job %s", jobID)
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	fileName, ok := notification["fileName"].(string)
	if !ok {
		err := fmt.Errorf("missing fileName in notification data for job %s", jobID)
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	md5Hash, ok := notification["md5Hash"].(string)
	if !ok {
		err := fmt.Errorf("missing md5Hash in notification data for job %s", jobID)
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	success, ok := notification["success"].(bool)
	if !ok || !success {
		err := fmt.Errorf("job %s was not successful according to notification", jobID)
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	logger.Infof("Processing notification-based privilege data: job_id=%s, file=%s, md5=%s", jobID, fileName, md5Hash)

	// Construct local file path
	localFilePath := fmt.Sprintf("%s/%s/%s", config.Cfg.NotificationFileDir, jobID, md5Hash)
	logger.Infof("Processing privilege data from notification file: %s", localFilePath)

	// Parse privilege data results
	privilegeData, err := parsePrivilegeDataFile(localFilePath)
	if err != nil {
		errMsg := fmt.Sprintf("failed to parse notification privilege data for job %s: %v", jobID, err)
		jobMonitor.FailJobAfterProcessing(jobID, errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	logger.Infof("Successfully parsed %d privilege table results from notification for job %s", len(privilegeData), jobID)

	// Process policies with loaded privilege data
	totalPolicies, err := createPoliciesWithPrivilegeData(jobID, sessionContext, privilegeData)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	// Mark job as completed after all server-side processing is done
	successMsg := fmt.Sprintf("Privilege session completed successfully - created %d policies", totalPolicies)
	if err := jobMonitor.CompleteJobAfterProcessing(jobID, successMsg); err != nil {
		logger.Errorf("Failed to mark job as completed: %v", err)
	}

	logger.Infof("Privilege session notification handler executed successfully for job %s - created %d policies", jobID, totalPolicies)
	return nil
}

// processPrivilegeSessionFromVeloArtifact handles privilege session processing via VeloArtifact polling
func processPrivilegeSessionFromVeloArtifact(jobID string, sessionContext *PrivilegeSessionJobContext, statusResp *job.StatusResponse) error {
	logger.Infof("Processing privilege session from VeloArtifact polling for job %s", jobID)

	jobMonitor := job.GetJobMonitorService()

	// Get endpoint information
	ep, err := getEndpointForJob(jobID, sessionContext.EndpointID)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	// Retrieve privilege data results
	privilegeData, err := retrievePrivilegeDataResults(jobID, ep)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	logger.Infof("Successfully retrieved %d privilege table results via VeloArtifact for job %s", len(privilegeData), jobID)

	// Process policies with loaded privilege data
	totalPolicies, err := createPoliciesWithPrivilegeData(jobID, sessionContext, privilegeData)
	if err != nil {
		jobMonitor.FailJobAfterProcessing(jobID, err.Error())
		return err
	}

	// Mark job as completed after all server-side processing is done
	successMsg := fmt.Sprintf("Privilege session completed successfully - created %d policies", totalPolicies)
	if err := jobMonitor.CompleteJobAfterProcessing(jobID, successMsg); err != nil {
		logger.Errorf("Failed to mark job as completed: %v", err)
	}

	logger.Infof("Privilege session VeloArtifact handler executed successfully for job %s - created %d policies", jobID, totalPolicies)
	return nil
}

// parsePrivilegeDataFile reads and parses privilege data results file
func parsePrivilegeDataFile(filePath string) ([]QueryResult, error) {
	logger.Debugf("Reading privilege data file: %s", filePath)

	// Read file content
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read privilege data file %s: %v", filePath, err)
	}

	// Parse JSON content
	var resultsData []QueryResult
	if err := json.Unmarshal(fileData, &resultsData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON from file %s: %v", filePath, err)
	}

	logger.Debugf("Successfully parsed privilege data file with %d table results", len(resultsData))
	return resultsData, nil
}

// retrievePrivilegeDataResults gets privilege data from VeloArtifact
func retrievePrivilegeDataResults(jobID string, ep *models.Endpoint) ([]QueryResult, error) {
	// Use same logic as policy results retrieval
	return retrieveJobResults(jobID, ep)
}

// writeQueryToLogFile appends query to log file for tracking purposes
func writeQueryToLogFile(logFile *os.File, passName, uniqueKey, query string) {
	if logFile == nil {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logEntry := fmt.Sprintf("[%s] [%s] [%s]\n%s\n\n", timestamp, passName, uniqueKey, query)

	if _, err := logFile.WriteString(logEntry); err != nil {
		logger.Warnf("Failed to write query to log file: %v", err)
	}
}

// createPoliciesWithPrivilegeData creates policies using three-pass execution strategy for MySQL databases only.
// Pass 1: Super privileges (ID=1) - actor_id=-1, object_id=-1, dbmgt_id=-1
// Pass 2: Action-wide privileges (DBGroupListPolicies) - object_id=-1, dbmgt_id=-1
// Pass 3: Object-specific privileges - normal policies with specific objects/databases
// Returns error if database type is not MySQL.
func createPoliciesWithPrivilegeData(jobID string, sessionContext *PrivilegeSessionJobContext, privilegeData []QueryResult) (int, error) {
	// Idempotency check: prevent duplicate processing from notification + polling
	if _, alreadyProcessed := processedPrivilegeJobs.LoadOrStore(jobID, true); alreadyProcessed {
		logger.Warnf("Job %s already processed, skipping duplicate execution", jobID)
		return 0, nil
	}

	// Validate MySQL-only support
	if sessionContext.CMT != nil && strings.ToLower(sessionContext.CMT.CntType) != "mysql" {
		return 0, fmt.Errorf("privilege session only supports MySQL databases, got: %s", sessionContext.CMT.CntType)
	}

	logger.Infof("Creating in-memory privilege session for MySQL database, job %s", jobID)

	ctx := context.Background()
	session, err := NewPrivilegeSession(ctx, sessionContext.SessionID)
	if err != nil {
		return 0, fmt.Errorf("failed to create privilege session: %w", err)
	}
	defer session.Close()

	if err := loadPrivilegeDataFromResults(session, privilegeData); err != nil {
		return 0, fmt.Errorf("failed to load privilege data: %w", err)
	}

	logger.Infof("Privilege data loaded into session %s, classifying policy templates", sessionContext.SessionID)

	// Create query log file for tracking (only if enabled)
	var logFile *os.File
	if config.Cfg.EnableMySQLPrivilegeQueryLogging {
		logFileName := fmt.Sprintf("privilege_queries_%s_%s.log", sessionContext.SessionID, time.Now().Format("20060102_150405"))
		logFilePath := filepath.Join(config.Cfg.DBFWebTempDir, logFileName)
		var err error
		logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			logger.Warnf("Failed to create query log file %s: %v", logFilePath, err)
			logFile = nil
		} else {
			defer logFile.Close()
			logger.Infof("Query log file created: %s", logFilePath)
			logFile.WriteString(fmt.Sprintf("=== Privilege Session Query Log ===\n"))
			logFile.WriteString(fmt.Sprintf("Job ID: %s\n", jobID))
			logFile.WriteString(fmt.Sprintf("Session ID: %s\n", sessionContext.SessionID))
			logFile.WriteString(fmt.Sprintf("Started: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
		}
	}

	baseRepo := repository.NewBaseRepository()
	tx := baseRepo.Begin()
	var txCommitted bool
	defer func() {
		if !txCommitted {
			tx.Rollback()
		}
	}()

	service := NewDBPolicyService().(*dbPolicyService)

	classification := classifyPolicyTemplates(service.DBPolicyDefaultsAllMap)
	grantedActions := newGrantedActionsCache()
	allowedResults := newAllowedPolicyResults()
	superPrivActors := newSuperPrivilegeActors()
	totalPolicies := 0

	// PASS 1: Super privileges - grant ALL actions on ALL objects for ALL databases
	if len(classification.superPrivileges) > 0 {
		logger.Infof("Processing Pass 1: %d super privilege templates", len(classification.superPrivileges))

		superQueries, err := processGeneralSQLTemplatesForSession(
			tx, classification.superPrivileges, sessionContext.DbMgts,
			sessionContext.DbActorMgts, sessionContext.CMT, service)
		if err != nil {
			logger.Errorf("Failed to build super privilege queries: %v", err)
		} else {
			superPolicies := executeSuperPrivilegeQueries(tx, session, superQueries, service, sessionContext.CntMgtID, sessionContext.CMT, allowedResults, superPrivActors, logFile)
			totalPolicies += superPolicies
			logger.Infof("Pass 1 completed: %d super policies created", superPolicies)
		}
	}

	// PASS 2: Action-wide privileges - grant specific action on ALL objects for ALL databases
	if len(classification.actionWidePrivs) > 0 {
		logger.Infof("Processing Pass 2: %d action-wide privilege templates", len(classification.actionWidePrivs))

		actionWideQueries, err := processGeneralSQLTemplatesForSession(
			tx, classification.actionWidePrivs, sessionContext.DbMgts,
			sessionContext.DbActorMgts, sessionContext.CMT, service)
		if err != nil {
			logger.Errorf("Failed to build action-wide queries: %v", err)
		} else {
			actionPolicies := executeActionWideQueries(tx, session, actionWideQueries, service, sessionContext.CntMgtID, sessionContext.CMT, grantedActions, allowedResults, superPrivActors, logFile)
			totalPolicies += actionPolicies
			logger.Infof("Pass 2 completed: %d action-wide policies created", actionPolicies)
		}
	}

	// PASS 3: Object-specific privileges - skip if action already granted in Pass 2
	if len(classification.objectSpecificPrivs) > 0 {
		logger.Infof("Processing Pass 3: %d object-specific privilege templates", len(classification.objectSpecificPrivs))

		// Build cache to eliminate N+1 queries during query building
		cache, err := buildQueryBuildCache(tx, sessionContext.DbMgts[0].CntID, sessionContext.DbMgts, classification.objectSpecificPrivs, service)
		if err != nil {
			logger.Errorf("Failed to build query cache: %v", err)
			// Continue without cache - will be slower but functional
			cache = &queryBuildCache{
				allActorsByCntID: make(map[uint][]*models.DBActorMgt),
				objectsByKey:     make(map[string][]*models.DBObjectMgt),
			}
		}

		generalQueries, err := processGeneralSQLTemplatesForSession(
			tx, classification.objectSpecificPrivs, sessionContext.DbMgts,
			sessionContext.DbActorMgts, sessionContext.CMT, service)
		if err != nil {
			logger.Errorf("Failed to build general object-specific queries: %v", err)
		}

		specificQueries, err := processSpecificSQLTemplatesForSession(
			tx, classification.objectSpecificPrivs, sessionContext.DbMgts,
			sessionContext.DbActorMgts, sessionContext.CMT, service, cache)
		if err != nil {
			logger.Errorf("Failed to build specific object-specific queries: %v", err)
		}

		allObjectQueries := make(map[string]policyInput)
		for k, v := range generalQueries {
			allObjectQueries[k] = v
		}
		for k, v := range specificQueries {
			allObjectQueries[k] = v
		}

		objectPolicies := executeObjectSpecificQueries(tx, session, allObjectQueries, service, sessionContext.CntMgtID, grantedActions, allowedResults, superPrivActors, logFile)
		totalPolicies += objectPolicies
		logger.Infof("Pass 3 completed: %d object-specific policies created", objectPolicies)
	}

	// Assign actors to groups based on direct query results (not database reads)
	logger.Infof("Assigning actors to groups based on allowed query results from %d policies", totalPolicies)
	if err := assignActorsToGroups(tx, sessionContext.CntMgtID, sessionContext.CMT, allowedResults, superPrivActors); err != nil {
		logger.Warnf("Failed to assign actors to groups: %v", err)
	}

	if err := tx.Commit().Error; err != nil {
		return 0, fmt.Errorf("failed to commit policies: %v", err)
	}
	txCommitted = true

	logger.Infof("Successfully created %d policies for job %s (super+action-wide+object-specific)",
		totalPolicies, jobID)

	// Call exportDBFPolicy to build rule files after policy insertion completed (run in background)
	logger.Infof("Starting background exportDBFPolicy to build rule files for job %s", jobID)
	go func(jID string) {
		if err := utils.ExportDBFPolicy(); err != nil {
			logger.Warnf("Failed to export DBF policy rules for job %s: %v", jID, err)
		} else {
			logger.Infof("Successfully exported DBF policy rules for job %s", jID)
		}
	}(jobID)

	return totalPolicies, nil
}

// executeSuperPrivilegeQueries executes Pass 1 queries concurrently and creates super privilege policies
// Creates policies with actor_id=-1, object_id=-1, dbmgt_id=-1
// Records allowed policies in allowedResults for actor-to-group assignment
// Marks actors with super privileges to skip PASS-2 and PASS-3
// Uses goroutines + semaphore for concurrent query execution
func executeSuperPrivilegeQueries(
	tx *gorm.DB,
	session *PrivilegeSession,
	queries map[string]policyInput,
	service *dbPolicyService,
	cntMgtID uint,
	cmt *models.CntMgt,
	allowedResults *allowedPolicyResults,
	superPrivActors *superPrivilegeActors,
	logFile *os.File,
) int {
	type queryResult struct {
		uniqueKey   string
		policyData  policyInput
		resultValue string
		err         error
	}

	// Execute all queries concurrently
	maxConcurrent := config.GetPrivilegeQueryConcurrency()
	logger.Debugf("Using privilege query concurrency: %d for %d super privilege queries", maxConcurrent, len(queries))
	semaphore := make(chan struct{}, maxConcurrent)
	results := make(chan queryResult, len(queries))

	queryCount := 0
	for uniqueKey, policyData := range queries {
		queryCount++
		go func(key string, input policyInput) {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Panic in executeSuperPrivilegeQueries goroutine for key %s: %v", key, r)
					results <- queryResult{uniqueKey: key, policyData: input, err: fmt.Errorf("panic: %v", r)}
				}
			}()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			rewrittenSQL := rewriteQueryForPrivilegeSession(input.finalSQL)

			// Log query if enabled
			writeQueryToLogFile(logFile, "PASS-1-SUPER", key, rewrittenSQL)

			result, err := session.ExecuteTemplate(rewrittenSQL, map[string]string{})
			if err != nil {
				results <- queryResult{uniqueKey: key, policyData: input, err: err}
				return
			}

			resultValue := service.extractResultValue(result)
			results <- queryResult{uniqueKey: key, policyData: input, resultValue: resultValue, err: nil}
		}(uniqueKey, policyData)
	}

	// Collect all query results
	allResults := make([]queryResult, 0, queryCount)
	for i := 0; i < queryCount; i++ {
		result := <-results
		if result.err != nil {
			logger.Debugf("Super privilege query failed for key=%s: %v", result.uniqueKey, result.err)
			continue
		}
		allResults = append(allResults, result)
	}

	// Process results serially to create policies (database operations must be serial)
	policiesCreated := 0
	for _, result := range allResults {
		if !service.isPolicyAllowed(result.resultValue, result.policyData.policydf.SqlGetAllow, result.policyData.policydf.SqlGetDeny) {
			continue
		}

		// Record allowed policy for actor-to-group assignment
		allowedResults.recordAllowed(result.policyData.actorId, result.policyData.policydf.ID)

		// Check duplicate (silent mode to reduce log noise)
		var existing models.DBPolicy
		err := tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).
			Where("cnt_id=? AND actor_id=? AND object_id=? AND dbmgt_id=? AND dbpolicydefault_id=?",
				cntMgtID, result.policyData.actorId, -1, -1, result.policyData.policydf.ID).First(&existing).Error
		if err == nil {
			continue
		}

		policy := models.DBPolicy{
			CntMgt:          cntMgtID,
			DBPolicyDefault: result.policyData.policydf.ID,
			DBMgt:           -1,
			DBActorMgt:      result.policyData.actorId,
			DBObjectMgt:     -1,
			Status:          "enabled",
			Description:     "Auto-inserted by Group Policy",
		}
		if err := tx.Create(&policy).Error; err != nil {
			logger.Errorf("Failed to create super policy: %v", err)
			continue
		}
		policiesCreated++

		// Mark actor as having super privileges to skip PASS-2 and PASS-3
		superPrivActors.markSuperPrivilege(result.policyData.actorId)

		logger.Debugf("Created super policy: policy_id=%d, actor_id=%d marked with super privileges, result=%s",
			policy.ID, result.policyData.actorId, result.resultValue)
	}

	logger.Infof("PASS-1 completed: %d actors marked with super privileges", len(superPrivActors.actors))
	return policiesCreated
}

// executeActionWideQueries executes Pass 2 queries concurrently and creates action-wide policies
// Creates policies with object_id=-1, dbmgt_id=-1 for specific action
// Skips actors with super privileges from PASS-1
// Records allowed policies in allowedResults for actor-to-group assignment
// Uses goroutines + semaphore for concurrent query execution
func executeActionWideQueries(
	tx *gorm.DB,
	session *PrivilegeSession,
	queries map[string]policyInput,
	service *dbPolicyService,
	cntMgtID uint,
	cmt *models.CntMgt,
	grantedActions *grantedActionsCache,
	allowedResults *allowedPolicyResults,
	superPrivActors *superPrivilegeActors,
	logFile *os.File,
) int {
	type queryResult struct {
		uniqueKey   string
		policyData  policyInput
		resultValue string
		err         error
	}

	// Filter out queries for actors with super privileges BEFORE concurrent execution
	filteredQueries := make(map[string]policyInput)
	skippedBySuperPriv := 0
	for uniqueKey, policyData := range queries {
		if superPrivActors.hasSuperPrivilege(policyData.actorId) {
			skippedBySuperPriv++
			continue
		}
		filteredQueries[uniqueKey] = policyData
	}

	if skippedBySuperPriv > 0 {
		logger.Infof("PASS-2: Skipped %d queries for actors with super privileges (from %d total queries)",
			skippedBySuperPriv, len(queries))
	}

	if len(filteredQueries) == 0 {
		logger.Debugf("All action-wide queries skipped due to super privileges")
		return 0
	}

	// Execute filtered queries concurrently
	maxConcurrent := config.GetPrivilegeQueryConcurrency()
	logger.Debugf("Using privilege query concurrency: %d for %d action-wide queries (filtered from %d)",
		maxConcurrent, len(filteredQueries), len(queries))
	semaphore := make(chan struct{}, maxConcurrent)
	results := make(chan queryResult, len(filteredQueries))

	queryCount := 0
	for uniqueKey, policyData := range filteredQueries {
		queryCount++
		go func(key string, input policyInput) {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Panic in executeActionWideQueries goroutine for key %s: %v", key, r)
					results <- queryResult{uniqueKey: key, policyData: input, err: fmt.Errorf("panic: %v", r)}
				}
			}()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			rewrittenSQL := rewriteQueryForPrivilegeSession(input.finalSQL)

			// Log query if enabled
			writeQueryToLogFile(logFile, "PASS-2-ACTION-WIDE", key, rewrittenSQL)

			result, err := session.ExecuteTemplate(rewrittenSQL, map[string]string{})
			if err != nil {
				results <- queryResult{uniqueKey: key, policyData: input, err: err}
				return
			}

			resultValue := service.extractResultValue(result)
			results <- queryResult{uniqueKey: key, policyData: input, resultValue: resultValue, err: nil}
		}(uniqueKey, policyData)
	}

	// Collect all query results
	allResults := make([]queryResult, 0, queryCount)
	for i := 0; i < queryCount; i++ {
		result := <-results
		if result.err != nil {
			logger.Debugf("Action-wide query failed for key=%s: %v", result.uniqueKey, result.err)
			continue
		}
		allResults = append(allResults, result)
	}

	// Process results serially to create policies (database operations must be serial)
	policiesCreated := 0
	for _, result := range allResults {
		if !service.isPolicyAllowed(result.resultValue, result.policyData.policydf.SqlGetAllow, result.policyData.policydf.SqlGetDeny) {
			continue
		}

		// Record allowed policy for actor-to-group assignment
		allowedResults.recordAllowed(result.policyData.actorId, result.policyData.policydf.ID)

		// Check duplicate (silent mode to reduce log noise)
		var existing models.DBPolicy
		err := tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).
			Where("cnt_id=? AND actor_id=? AND object_id=? AND dbmgt_id=? AND dbpolicydefault_id=?",
				cntMgtID, result.policyData.actorId, -1, -1, result.policyData.policydf.ID).First(&existing).Error
		if err == nil {
			grantedActions.markGranted(result.policyData.actorId, result.policyData.policydf.ActionId)
			continue
		}

		policy := models.DBPolicy{
			CntMgt:          cntMgtID,
			DBPolicyDefault: result.policyData.policydf.ID,
			DBMgt:           -1,
			DBActorMgt:      result.policyData.actorId,
			DBObjectMgt:     -1,
			Status:          "enabled",
			Description:     "Auto-inserted by Group Policy",
		}
		if err := tx.Create(&policy).Error; err != nil {
			logger.Errorf("Failed to create action-wide policy: %v", err)
			continue
		}
		policiesCreated++

		grantedActions.markGranted(result.policyData.actorId, result.policyData.policydf.ActionId)
		logger.Debugf("Created action-wide policy: policy_id=%d, actor=%d, action=%d, result=%s",
			policy.ID, result.policyData.actorId, result.policyData.policydf.ActionId, result.resultValue)
	}

	return policiesCreated
}

// executeObjectSpecificQueries executes Pass 3 queries concurrently with action-grant checking
// Skips queries if actor has super privileges from PASS-1 or action already granted in Pass 2
// Records allowed policies in allowedResults for actor-to-group assignment
// Uses goroutines + semaphore for concurrent query execution
func executeObjectSpecificQueries(
	tx *gorm.DB,
	session *PrivilegeSession,
	queries map[string]policyInput,
	service *dbPolicyService,
	cntMgtID uint,
	grantedActions *grantedActionsCache,
	allowedResults *allowedPolicyResults,
	superPrivActors *superPrivilegeActors,
	logFile *os.File,
) int {
	type queryResult struct {
		uniqueKey   string
		policyData  policyInput
		resultValue string
		err         error
	}

	// Filter out queries for actors with super privileges OR already-granted actions BEFORE concurrent execution
	filteredQueries := make(map[string]policyInput)
	skippedBySuperPriv := 0
	skippedByGrantedAction := 0
	for uniqueKey, policyData := range queries {
		if superPrivActors.hasSuperPrivilege(policyData.actorId) {
			skippedBySuperPriv++
			continue
		}
		if grantedActions.isGranted(policyData.actorId, policyData.policydf.ActionId) {
			skippedByGrantedAction++
			continue
		}
		filteredQueries[uniqueKey] = policyData
	}

	if skippedBySuperPriv > 0 {
		logger.Infof("PASS-3: Skipped %d queries for actors with super privileges", skippedBySuperPriv)
	}
	if skippedByGrantedAction > 0 {
		logger.Debugf("PASS-3: Skipped %d queries for already-granted actions", skippedByGrantedAction)
	}

	if len(filteredQueries) == 0 {
		logger.Debugf("All object-specific queries skipped (super privileges: %d, granted actions: %d)",
			skippedBySuperPriv, skippedByGrantedAction)
		return 0
	}

	// Execute filtered queries concurrently
	maxConcurrent := config.GetPrivilegeQueryConcurrency()
	logger.Debugf("Using privilege query concurrency: %d for %d object-specific queries (filtered from %d)",
		maxConcurrent, len(filteredQueries), len(queries))
	semaphore := make(chan struct{}, maxConcurrent)
	results := make(chan queryResult, len(filteredQueries))

	queryCount := 0
	for uniqueKey, policyData := range filteredQueries {
		queryCount++
		go func(key string, input policyInput) {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Panic in executeObjectSpecificQueries goroutine for key %s: %v", key, r)
					results <- queryResult{uniqueKey: key, policyData: input, err: fmt.Errorf("panic: %v", r)}
				}
			}()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			rewrittenSQL := rewriteQueryForPrivilegeSession(input.finalSQL)

			// Log query if enabled
			writeQueryToLogFile(logFile, "PASS-3-OBJECT-SPECIFIC", key, rewrittenSQL)

			result, err := session.ExecuteTemplate(rewrittenSQL, map[string]string{})
			if err != nil {
				results <- queryResult{uniqueKey: key, policyData: input, err: err}
				return
			}

			resultValue := service.extractResultValue(result)
			results <- queryResult{uniqueKey: key, policyData: input, resultValue: resultValue, err: nil}
		}(uniqueKey, policyData)
	}

	// Collect all query results
	allResults := make([]queryResult, 0, queryCount)
	for i := 0; i < queryCount; i++ {
		result := <-results
		if result.err != nil {
			logger.Debugf("Object-specific query failed for key=%s: %v", result.uniqueKey, result.err)
			continue
		}
		allResults = append(allResults, result)
	}

	// Process results serially to create policies (database operations must be serial)
	policiesCreated := 0
	for _, result := range allResults {
		if !service.isPolicyAllowed(result.resultValue, result.policyData.policydf.SqlGetAllow, result.policyData.policydf.SqlGetDeny) {
			continue
		}

		// Record allowed policy for actor-to-group assignment
		allowedResults.recordAllowed(result.policyData.actorId, result.policyData.policydf.ID)

		// Check duplicate (silent mode to reduce log noise)
		var existing models.DBPolicy
		err := tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).
			Where("dbmgt_id=? AND actor_id=? AND object_id=? AND dbpolicydefault_id=?",
				result.policyData.dbmgtId, result.policyData.actorId, result.policyData.objectId, result.policyData.policydf.ID).First(&existing).Error
		if err == nil {
			continue
		}

		policy := models.DBPolicy{
			CntMgt:          cntMgtID,
			DBPolicyDefault: result.policyData.policydf.ID,
			DBMgt:           result.policyData.dbmgtId,
			DBActorMgt:      result.policyData.actorId,
			DBObjectMgt:     result.policyData.objectId,
			Status:          "enabled",
			Description:     "Auto-inserted by Group Policy",
		}
		if err := tx.Create(&policy).Error; err != nil {
			logger.Errorf("Failed to create object-specific policy: %v", err)
			continue
		}
		policiesCreated++

		logger.Debugf("Created object-specific policy: policy_id=%d, actor=%d, object=%d, dbmgt=%d, result=%s",
			policy.ID, result.policyData.actorId, result.policyData.objectId, result.policyData.dbmgtId, result.resultValue)
	}

	return policiesCreated
}

// loadPrivilegeDataFromResults populates in-memory session with privilege table data concurrently.
// Concurrency improves performance when loading multiple large privilege tables (mysql.user, mysql.db, etc).
// Returns error if any critical table fails to load, allowing caller to retry entire operation.
func loadPrivilegeDataFromResults(session *PrivilegeSession, results []QueryResult) error {
	// information_schema tables must be mapped to mysql.infoschema_* because go-mysql-server
	// enforces read-only constraint on information_schema database for SQL standard compliance
	tableMap := map[string]string{
		"mysql.user":                           "mysql.user",
		"mysql.db":                             "mysql.db",
		"mysql.tables_priv":                    "mysql.tables_priv",
		"mysql.procs_priv":                     "mysql.procs_priv",
		"mysql.role_edges":                     "mysql.role_edges",
		"mysql.global_grants":                  "mysql.global_grants",
		"mysql.proxies_priv":                   "mysql.proxies_priv",
		"information_schema.USER_PRIVILEGES":   "mysql.infoschema_user_privileges",
		"information_schema.SCHEMA_PRIVILEGES": "mysql.infoschema_schema_privileges",
		"information_schema.TABLE_PRIVILEGES":  "mysql.infoschema_table_privileges",
	}

	type loadResult struct {
		tableName string
		err       error
		rowCount  int
	}

	// Limit concurrent loads to prevent memory exhaustion when processing large privilege datasets
	maxConcurrent := config.GetPrivilegeLoadConcurrency()
	logger.Debugf("Using privilege load concurrency: %d", maxConcurrent)
	semaphore := make(chan struct{}, maxConcurrent)
	loadResults := make(chan loadResult, len(results))

	validResults := 0
	for _, result := range results {
		// VeloArtifact appends array index [0], [1] to distinguish multiple query executions
		queryKey := result.QueryKey
		if idx := strings.Index(queryKey, "["); idx != -1 {
			queryKey = queryKey[:idx]
		}

		tableName, ok := tableMap[queryKey]
		if !ok {
			logger.Warnf("Unknown privilege table key: %s", result.QueryKey)
			continue
		}

		logger.Debugf("Processing privilege data: query_key=%s, table=%s, status=%s, rows=%d",
			result.QueryKey, tableName, result.Status, len(result.Result))

		if result.Status != "success" {
			logger.Warnf("Query failed for %s (key=%s): status=%s",
				tableName, result.QueryKey, result.Status)
			continue
		}

		validResults++
		go func(tblName string, rows [][]interface{}) {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Panic in loadPrivilegeData goroutine for table %s: %v", tblName, r)
					loadResults <- loadResult{tableName: tblName, err: fmt.Errorf("panic: %v", r), rowCount: 0}
				}
			}()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			err := insertResultsIntoSession(session, tblName, rows)
			loadResults <- loadResult{tableName: tblName, err: err, rowCount: len(rows)}
		}(tableName, result.Result)
	}

	for i := 0; i < validResults; i++ {
		result := <-loadResults
		if result.err != nil {
			logger.Errorf("Failed to insert into %s: %v", result.tableName, result.err)
		} else {
			logger.Debugf("Loaded %d rows into %s", result.rowCount, result.tableName)
		}
	}

	return nil
}

// insertResultsIntoSession converts VeloArtifact query results into SQL INSERT statements.
// Escapes values to prevent SQL injection when building dynamic INSERT statements.
// Returns error if schema validation fails or INSERT execution fails.
func insertResultsIntoSession(session *PrivilegeSession, tableName string, rows [][]interface{}) error {
	if len(rows) == 0 {
		logger.Debugf("No rows to insert for %s", tableName)
		return nil
	}

	if len(rows[0]) == 0 {
		logger.Warnf("Empty row data for %s", tableName)
		return nil
	}

	columnNames, err := getTableColumnNames(tableName)
	if err != nil {
		return fmt.Errorf("failed to get column names for %s: %w", tableName, err)
	}

	// Column mismatch indicates schema drift between VeloArtifact query and session schema
	if len(rows[0]) != len(columnNames) {
		logger.Warnf("Column count mismatch for %s: expected %d, got %d", tableName, len(columnNames), len(rows[0]))
	}

	insertedCount := 0
	for _, row := range rows {
		values := make([]string, len(row))
		for i, val := range row {
			if val == nil {
				values[i] = "NULL"
			} else {
				// SQL standard escaping - single quotes must be doubled to prevent injection
				strVal := fmt.Sprintf("%v", val)
				escapedVal := strings.ReplaceAll(strVal, "'", "''")
				values[i] = fmt.Sprintf("'%s'", escapedVal)
			}
		}

		insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			tableName,
			strings.Join(columnNames, ", "),
			strings.Join(values, ", "))

		if _, err := session.ExecuteTemplate(insertSQL, map[string]string{}); err != nil {
			logger.Warnf("Failed to insert row into %s: %v", tableName, err)
			continue
		}
		insertedCount++
	}

	logger.Debugf("Inserted %d/%d rows into %s", insertedCount, len(rows), tableName)
	return nil
}

// rewriteQueryForPrivilegeSession rewrites queries to use mysql.infoschema_* tables instead of information_schema.*.
// Required because go-mysql-server doesn't allow INSERT into information_schema database.
func rewriteQueryForPrivilegeSession(sql string) string {
	// Replace information_schema table references with mysql.infoschema_* equivalents
	replacements := map[string]string{
		"information_schema.USER_PRIVILEGES":   "mysql.infoschema_user_privileges",
		"information_schema.SCHEMA_PRIVILEGES": "mysql.infoschema_schema_privileges",
		"information_schema.TABLE_PRIVILEGES":  "mysql.infoschema_table_privileges",
		// Case variations for safety
		"INFORMATION_SCHEMA.USER_PRIVILEGES":   "mysql.infoschema_user_privileges",
		"INFORMATION_SCHEMA.SCHEMA_PRIVILEGES": "mysql.infoschema_schema_privileges",
		"INFORMATION_SCHEMA.TABLE_PRIVILEGES":  "mysql.infoschema_table_privileges",
	}

	rewrittenSQL := sql
	for old, new := range replacements {
		rewrittenSQL = strings.ReplaceAll(rewrittenSQL, old, new)
	}

	return rewrittenSQL
}

// processQueriesWithSession executes policy template queries against in-memory privilege session and creates matching policies.
// Uses two-pass processing: wildcard objects first (objectID=-1), then specific objects (objectID>0).
// Wildcard policies prevent redundant specific object policies for same actor+policy combination.
// Returns count of policies created based on SqlGetAllow/SqlGetDeny rule matching.
func processQueriesWithSession(tx *gorm.DB, session *PrivilegeSession, sqlFinalMap map[string]policyInput, service *dbPolicyService, dbmgt *models.DBMgt, cmt *models.CntMgt) int {
	type queryResult struct {
		uniqueKey   string
		policyInput policyInput
		resultValue string
		err         error
	}

	// Execute all queries concurrently to prevent session resource exhaustion
	maxConcurrent := config.GetPrivilegeQueryConcurrency()
	logger.Debugf("Using privilege query concurrency: %d for %d queries", maxConcurrent, len(sqlFinalMap))
	semaphore := make(chan struct{}, maxConcurrent)
	results := make(chan queryResult, len(sqlFinalMap))

	queryCount := 0
	for uniqueKey, policyData := range sqlFinalMap {
		queryCount++
		go func(key string, input policyInput) {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Panic in processQueries goroutine for key %s: %v", key, r)
					results <- queryResult{uniqueKey: key, policyInput: input, err: fmt.Errorf("panic: %v", r)}
				}
			}()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Rewrite query to use mysql.infoschema_* tables instead of information_schema.*
			rewrittenSQL := rewriteQueryForPrivilegeSession(input.finalSQL)

			result, err := session.ExecuteTemplate(rewrittenSQL, map[string]string{})
			if err != nil {
				results <- queryResult{uniqueKey: key, policyInput: input, err: err}
				return
			}

			resultValue := service.extractResultValue(result)
			results <- queryResult{uniqueKey: key, policyInput: input, resultValue: resultValue, err: nil}
		}(uniqueKey, policyData)
	}

	// Collect all query results
	allResults := make([]queryResult, 0, queryCount)
	for i := 0; i < queryCount; i++ {
		result := <-results
		if result.err != nil {
			logger.Debugf("Query execution failed for key=%s: %v", result.uniqueKey, result.err)
			continue
		}
		allResults = append(allResults, result)
	}

	// Track wildcard policies to skip redundant specific objects
	allowedActorPolicies := make(map[string]bool)
	policiesCreated := 0

	// First pass: Process wildcard objects (objectID == -1)
	for _, result := range allResults {
		if result.policyInput.objectId != -1 {
			continue
		}

		// Check if policy should be created based on allow/deny rules
		if service.isPolicyAllowed(result.resultValue, result.policyInput.policydf.SqlGetAllow, result.policyInput.policydf.SqlGetDeny) {
			// Check for existing policy to prevent duplicates
			var existingPolicy models.DBPolicy
			err := tx.Where("dbmgt_id = ? AND actor_id = ? AND object_id = ? AND dbpolicydefault_id = ?",
				dbmgt.ID, result.policyInput.actorId, result.policyInput.objectId, result.policyInput.policydf.ID).
				First(&existingPolicy).Error

			if err == nil {
				logger.Debugf("Wildcard policy already exists for key=%s, skipping", result.uniqueKey)
				// Mark as allowed even though not newly created to skip specific objects
				actorPolicyKey := fmt.Sprintf("Actor:%d-PolicyDf:%d", result.policyInput.actorId, result.policyInput.policydf.ID)
				allowedActorPolicies[actorPolicyKey] = true
				continue
			} else if err != gorm.ErrRecordNotFound {
				logger.Errorf("Failed to check existing wildcard policy for key=%s: %v", result.uniqueKey, err)
				continue
			}

			policy := models.DBPolicy{
				CntMgt:          dbmgt.CntID,
				DBPolicyDefault: result.policyInput.policydf.ID,
				DBMgt:           utils.MustUintToInt(dbmgt.ID),
				DBActorMgt:      result.policyInput.actorId,
				DBObjectMgt:     result.policyInput.objectId,
				Status:          "enabled",
				Description:     "Auto-inserted by Group Policy",
			}
			if err := tx.Create(&policy).Error; err != nil {
				logger.Errorf("Failed to create wildcard policy for key=%s: %v", result.uniqueKey, err)
				continue
			}
			policiesCreated++

			// Track wildcard to skip specific objects for same actor+policy
			actorPolicyKey := fmt.Sprintf("Actor:%d-PolicyDf:%d", result.policyInput.actorId, result.policyInput.policydf.ID)
			allowedActorPolicies[actorPolicyKey] = true

			logger.Debugf("Created wildcard policy: actor=%d, policy_default=%d, result=%s",
				result.policyInput.actorId, result.policyInput.policydf.ID, result.resultValue)
		}
	}

	// Second pass: Process specific objects (objectID > 0)
	for _, result := range allResults {
		if result.policyInput.objectId == -1 {
			continue
		}

		// Skip if wildcard already grants permission for this actor+policy
		actorPolicyKey := fmt.Sprintf("Actor:%d-PolicyDf:%d", result.policyInput.actorId, result.policyInput.policydf.ID)
		if allowedActorPolicies[actorPolicyKey] {
			logger.Debugf("Skipping specific object for key=%s - wildcard already allowed", result.uniqueKey)
			continue
		}

		// Check if policy should be created based on allow/deny rules
		if service.isPolicyAllowed(result.resultValue, result.policyInput.policydf.SqlGetAllow, result.policyInput.policydf.SqlGetDeny) {
			// Check for existing policy to prevent duplicates
			var existingPolicy models.DBPolicy
			err := tx.Where("dbmgt_id = ? AND actor_id = ? AND object_id = ? AND dbpolicydefault_id = ?",
				dbmgt.ID, result.policyInput.actorId, result.policyInput.objectId, result.policyInput.policydf.ID).
				First(&existingPolicy).Error

			if err == nil {
				logger.Debugf("Specific policy already exists for key=%s, skipping", result.uniqueKey)
				continue
			} else if err != gorm.ErrRecordNotFound {
				logger.Errorf("Failed to check existing specific policy for key=%s: %v", result.uniqueKey, err)
				continue
			}

			policy := models.DBPolicy{
				CntMgt:          dbmgt.CntID,
				DBPolicyDefault: result.policyInput.policydf.ID,
				DBMgt:           utils.MustUintToInt(dbmgt.ID),
				DBActorMgt:      result.policyInput.actorId,
				DBObjectMgt:     result.policyInput.objectId,
				Status:          "enabled",
				Description:     "Auto-inserted by Group Policy",
			}
			if err := tx.Create(&policy).Error; err != nil {
				logger.Errorf("Failed to create specific policy for key=%s: %v", result.uniqueKey, err)
				continue
			}
			policiesCreated++

			logger.Debugf("Created specific policy: actor=%d, policy_default=%d, object=%d, result=%s",
				result.policyInput.actorId, result.policyInput.policydf.ID, result.policyInput.objectId, result.resultValue)
		}
	}

	return policiesCreated
}

// queryBuildCache holds pre-loaded data to eliminate N+1 database queries during query building
// All data loaded once upfront, then accessed via maps for O(1) lookup performance
type queryBuildCache struct {
	// All actors by CntID - loaded once to avoid repeated getDBActorMgts calls
	allActorsByCntID map[uint][]*models.DBActorMgt

	// Objects grouped by (ObjectId, DbMgtID) - loaded once to avoid repeated GetByObjectIdAndDbMgtInt calls
	// Key format: "ObjectId:DbMgtID" -> list of objects
	objectsByKey map[string][]*models.DBObjectMgt
}

// buildQueryBuildCache pre-loads all actors and objects data in batches to eliminate N+1 queries
// Dramatically improves query building performance by loading data once instead of hundreds of times in loops
func buildQueryBuildCache(tx *gorm.DB, cntID uint, allDatabases []models.DBMgt, policyDefaults []models.DBPolicyDefault, service *dbPolicyService) (*queryBuildCache, error) {
	cache := &queryBuildCache{
		allActorsByCntID: make(map[uint][]*models.DBActorMgt),
		objectsByKey:     make(map[string][]*models.DBObjectMgt),
	}

	// Pre-load all actors for this cntID (used by ObjectId=12)
	actors, err := service.getDBActorMgts(tx, cntID)
	if err != nil {
		logger.Warnf("Failed to pre-load actors for cnt_id=%d: %v", cntID, err)
		cache.allActorsByCntID[cntID] = []*models.DBActorMgt{}
	} else {
		cache.allActorsByCntID[cntID] = actors
		logger.Debugf("Pre-loaded %d actors for cnt_id=%d", len(actors), cntID)
	}

	// Collect all unique (ObjectId, DbMgtID) combinations that will be queried
	objectKeysToLoad := make(map[string]bool)
	for _, policydf := range policyDefaults {
		if policydf.SqlGetSpecific == "" {
			continue
		}

		sqlBytes, err := hex.DecodeString(policydf.SqlGetSpecific)
		if err != nil {
			continue
		}
		rawSQL := string(sqlBytes)

		hasObjectVar := strings.Contains(rawSQL, "${dbobjectmgt.objectname}")
		if hasObjectVar {
			// This policy will need objects - mark all databases for this ObjectId
			for _, dbmgt := range allDatabases {
				key := fmt.Sprintf("%d:%d", policydf.ObjectId, dbmgt.ID)
				objectKeysToLoad[key] = true
			}
		}
	}

	// Batch load all objects for collected keys
	objectsLoaded := 0
	for key := range objectKeysToLoad {
		var objectID int
		var dbMgtID uint
		fmt.Sscanf(key, "%d:%d", &objectID, &dbMgtID)

		objects, err := service.dbObjectMgtRepo.GetByObjectIdAndDbMgtInt(tx, objectID, dbMgtID)
		if err != nil || len(objects) == 0 {
			cache.objectsByKey[key] = []*models.DBObjectMgt{}
			continue
		}

		// Convert slice of values to slice of pointers
		objectPtrs := make([]*models.DBObjectMgt, len(objects))
		for i := range objects {
			objectPtrs[i] = &objects[i]
		}
		cache.objectsByKey[key] = objectPtrs
		objectsLoaded += len(objectPtrs)
	}

	logger.Infof("Pre-loaded cache: %d actors, %d object groups (%d total objects)",
		len(cache.allActorsByCntID[cntID]), len(cache.objectsByKey), objectsLoaded)

	return cache, nil
}

// processGeneralSQLTemplatesForSession builds SQL queries from general templates with actor-centric approach for MySQL only.
// Loops through actors first, then applies database context when ${dbmgt.dbname} variable exists.
// MySQL-specific handling: Skip ObjectId 15 (schema objects) for queries without object name variable.
// Returns map of unique keys to policy inputs for query execution.
func processGeneralSQLTemplatesForSession(
	tx *gorm.DB,
	policyDefaults []models.DBPolicyDefault,
	allDatabases []models.DBMgt,
	actors []models.DBActorMgt,
	cmt *models.CntMgt,
	service *dbPolicyService,
) (map[string]policyInput, error) {
	sqlFinalMap := make(map[string]policyInput)

	// MySQL-only: caller must validate cmt.CntType == "mysql" before calling this function
	logger.Debugf("Processing general SQL templates for database type: %s", cmt.CntType)

	for _, policydf := range policyDefaults {
		if policydf.SqlGet == "" {
			continue
		}

		sqlBytes, err := hex.DecodeString(policydf.SqlGet)
		if err != nil {
			logger.Warnf("Failed to decode SqlGet for policy_default_id=%d: %v", policydf.ID, err)
			continue
		}
		rawSQL := string(sqlBytes)

		// MySQL-only: Skip ObjectId 15 (schema objects) when query doesn't reference specific object names
		if !strings.Contains(rawSQL, "${dbobjectmgt.objectname}") {
			if policydf.ObjectId == 15 {
				continue
			}
		}

		hasDatabaseVar := strings.Contains(rawSQL, "${dbmgt.dbname}")

		for _, actor := range actors {
			if hasDatabaseVar {
				for _, dbmgt := range allDatabases {
					finalSQL := rawSQL
					finalSQL = strings.ReplaceAll(finalSQL, "${dbmgt.dbname}", dbmgt.DbName)
					finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.dbuser}", actor.DBUser)
					finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.ip_address}", actor.IPAddress)
					finalSQL = strings.ReplaceAll(finalSQL, "${dbobjectmgt.objectname}", "*")

					uniqueKey := fmt.Sprintf("Actor:%d_PolicyDf:%d_DbMgt:%d_General", actor.ID, policydf.ID, dbmgt.ID)

					sqlFinalMap[uniqueKey] = policyInput{
						policydf: policydf,
						actorId:  actor.ID,
						objectId: -1,
						dbmgtId:  utils.MustUintToInt(dbmgt.ID),
						finalSQL: finalSQL,
					}
				}
			} else {
				finalSQL := rawSQL
				finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.dbuser}", actor.DBUser)
				finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.ip_address}", actor.IPAddress)
				finalSQL = strings.ReplaceAll(finalSQL, "${dbobjectmgt.objectname}", "*")

				uniqueKey := fmt.Sprintf("Actor:%d_PolicyDf:%d_General", actor.ID, policydf.ID)

				sqlFinalMap[uniqueKey] = policyInput{
					policydf: policydf,
					actorId:  actor.ID,
					objectId: -1,
					dbmgtId:  -1,
					finalSQL: finalSQL,
				}
			}
		}
	}

	return sqlFinalMap, nil
}

// processSpecificSQLTemplatesForSession builds SQL queries from specific templates with actor-centric approach for MySQL only.
// Handles object-specific queries with database context awareness.
// MySQL-specific handling: ObjectId 12 (user objects) uses all actors for substitution, ObjectId 15 (schema objects) is skipped.
// Uses cache to eliminate N+1 database queries during query building.
// Returns map of unique keys to policy inputs for query execution.
func processSpecificSQLTemplatesForSession(
	tx *gorm.DB,
	policyDefaults []models.DBPolicyDefault,
	allDatabases []models.DBMgt,
	actors []models.DBActorMgt,
	cmt *models.CntMgt,
	service *dbPolicyService,
	cache *queryBuildCache,
) (map[string]policyInput, error) {
	sqlFinalMap := make(map[string]policyInput)

	// MySQL-only: caller must validate cmt.CntType == "mysql" before calling this function
	logger.Debugf("Processing specific SQL templates for database type: %s", cmt.CntType)

	for _, policydf := range policyDefaults {
		if policydf.SqlGetSpecific == "" {
			continue
		}

		sqlBytes, err := hex.DecodeString(policydf.SqlGetSpecific)
		if err != nil {
			logger.Warnf("Failed to decode SqlGetSpecific for policy_default_id=%d: %v", policydf.ID, err)
			continue
		}
		rawSQL := string(sqlBytes)

		hasDatabaseVar := strings.Contains(rawSQL, "${dbmgt.dbname}")
		hasObjectVar := strings.Contains(rawSQL, "${dbobjectmgt.objectname}")

		// MySQL-only special handling for specific object types
		if !hasObjectVar {
			switch policydf.ObjectId {
			case 12:
				// ObjectId = 12: User objects - substitute with all actors from cache
				actorMgts := cache.allActorsByCntID[allDatabases[0].CntID]
				if len(actorMgts) == 0 {
					logger.Debugf("No cached actor records for ObjectId 12, cnt_id=%d", allDatabases[0].CntID)
					continue
				}

				for _, actor := range actors {
					if hasDatabaseVar {
						for _, dbmgt := range allDatabases {
							for _, targetActor := range actorMgts {
								finalSQL := rawSQL
								finalSQL = strings.ReplaceAll(finalSQL, "${dbmgt.dbname}", dbmgt.DbName)
								finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.dbuser}", targetActor.DBUser)
								finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.ip_address}", targetActor.IPAddress)

								uniqueKey := fmt.Sprintf("Actor:%d_PolicyDf:%d_DbMgt:%d_Object:%d_Specific", actor.ID, policydf.ID, dbmgt.ID, targetActor.ID)

								sqlFinalMap[uniqueKey] = policyInput{
									policydf: policydf,
									actorId:  actor.ID,
									objectId: utils.MustUintToInt(targetActor.ID),
									dbmgtId:  utils.MustUintToInt(dbmgt.ID),
									finalSQL: finalSQL,
								}
							}
						}
					} else {
						for _, targetActor := range actorMgts {
							finalSQL := rawSQL
							finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.dbuser}", targetActor.DBUser)
							finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.ip_address}", targetActor.IPAddress)

							uniqueKey := fmt.Sprintf("Actor:%d_PolicyDf:%d_Object:%d_Specific", actor.ID, policydf.ID, targetActor.ID)

							sqlFinalMap[uniqueKey] = policyInput{
								policydf: policydf,
								actorId:  actor.ID,
								objectId: utils.MustUintToInt(targetActor.ID),
								dbmgtId:  -1,
								finalSQL: finalSQL,
							}
						}
					}
				}
				continue

			case 15:
				// ObjectId = 15: Skip for MySQL
				continue
			}
		}

		// Standard processing for objects with ${dbobjectmgt.objectname}
		for _, actor := range actors {
			if hasDatabaseVar {
				for _, dbmgt := range allDatabases {
					finalSQL := strings.ReplaceAll(rawSQL, "${dbmgt.dbname}", dbmgt.DbName)
					finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.dbuser}", actor.DBUser)
					finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.ip_address}", actor.IPAddress)

					if hasObjectVar {
						// Lookup objects from cache instead of database
						key := fmt.Sprintf("%d:%d", policydf.ObjectId, dbmgt.ID)
						dbobjectmgts := cache.objectsByKey[key]
						if len(dbobjectmgts) == 0 {
							continue
						}

						for _, object := range dbobjectmgts {
							finalSqlObject := strings.ReplaceAll(finalSQL, "${dbobjectmgt.objectname}", object.ObjectName)

							uniqueKey := fmt.Sprintf("Actor:%d_PolicyDf:%d_DbMgt:%d_Object:%d", actor.ID, policydf.ID, dbmgt.ID, object.ID)

							sqlFinalMap[uniqueKey] = policyInput{
								policydf: policydf,
								actorId:  actor.ID,
								objectId: utils.MustUintToInt(object.ID),
								dbmgtId:  utils.MustUintToInt(dbmgt.ID),
								finalSQL: finalSqlObject,
							}
						}
					} else {
						uniqueKey := fmt.Sprintf("Actor:%d_PolicyDf:%d_DbMgt:%d_Specific", actor.ID, policydf.ID, dbmgt.ID)

						sqlFinalMap[uniqueKey] = policyInput{
							policydf: policydf,
							actorId:  actor.ID,
							objectId: -1,
							dbmgtId:  utils.MustUintToInt(dbmgt.ID),
							finalSQL: finalSQL,
						}
					}
				}
			} else {
				if hasObjectVar {
					for _, dbmgt := range allDatabases {
						// Lookup objects from cache instead of database
						key := fmt.Sprintf("%d:%d", policydf.ObjectId, dbmgt.ID)
						dbobjectmgts := cache.objectsByKey[key]
						if len(dbobjectmgts) == 0 {
							continue
						}

						for _, object := range dbobjectmgts {
							finalSQL := rawSQL
							finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.dbuser}", actor.DBUser)
							finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.ip_address}", actor.IPAddress)
							finalSQL = strings.ReplaceAll(finalSQL, "${dbobjectmgt.objectname}", object.ObjectName)

							uniqueKey := fmt.Sprintf("Actor:%d_PolicyDf:%d_Object:%d", actor.ID, policydf.ID, object.ID)

							sqlFinalMap[uniqueKey] = policyInput{
								policydf: policydf,
								actorId:  actor.ID,
								objectId: utils.MustUintToInt(object.ID),
								dbmgtId:  -1,
								finalSQL: finalSQL,
							}
						}
					}
				} else {
					finalSQL := rawSQL
					finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.dbuser}", actor.DBUser)
					finalSQL = strings.ReplaceAll(finalSQL, "${dbactormgt.ip_address}", actor.IPAddress)

					uniqueKey := fmt.Sprintf("Actor:%d_PolicyDf:%d_Specific", actor.ID, policydf.ID)

					sqlFinalMap[uniqueKey] = policyInput{
						policydf: policydf,
						actorId:  actor.ID,
						objectId: -1,
						dbmgtId:  -1,
						finalSQL: finalSQL,
					}
				}
			}
		}
	}

	return sqlFinalMap, nil
}

// getTableColumnNames returns column schema for privilege tables in insertion order.
// Column order must match VeloArtifact query SELECT order to prevent data corruption.
// Returns error if table name is not recognized, indicating schema drift.
func getTableColumnNames(tableName string) ([]string, error) {
	columnMap := map[string][]string{
		// "mysql.user": {
		// 	"Host", "User", "Select_priv", "Insert_priv", "Update_priv", "Delete_priv",
		// 	"Create_priv", "Drop_priv", "Reload_priv", "Shutdown_priv", "Process_priv",
		// 	"File_priv", "Grant_priv", "References_priv", "Index_priv", "Alter_priv",
		// 	"Show_db_priv", "Super_priv", "Create_tmp_table_priv", "Lock_tables_priv",
		// 	"Execute_priv", "Repl_slave_priv", "Repl_client_priv", "Create_view_priv",
		// 	"Show_view_priv", "Create_routine_priv", "Alter_routine_priv", "Create_user_priv",
		// 	"Event_priv", "Trigger_priv", "Create_tablespace_priv", "Create_role_priv",
		// 	"Resource_group_admin_priv",
		// },
		"mysql.user": {
			"Host", "User", "Select_priv", "Insert_priv", "Update_priv", "Delete_priv",
			"Create_priv", "Drop_priv", "Reload_priv", "Shutdown_priv", "Process_priv",
			"File_priv", "Grant_priv", "References_priv", "Index_priv", "Alter_priv",
			"Show_db_priv", "Super_priv", "Create_tmp_table_priv", "Lock_tables_priv",
			"Execute_priv", "Repl_slave_priv", "Repl_client_priv", "Create_view_priv",
			"Show_view_priv", "Create_routine_priv", "Alter_routine_priv", "Create_user_priv",
			"Event_priv", "Trigger_priv", "Create_tablespace_priv",
		},
		"mysql.db": {
			"Host", "Db", "User", "Select_priv", "Insert_priv", "Update_priv", "Delete_priv",
			"Create_priv", "Drop_priv", "Grant_priv", "References_priv", "Index_priv",
			"Alter_priv", "Create_tmp_table_priv", "Lock_tables_priv", "Create_view_priv",
			"Show_view_priv", "Create_routine_priv", "Alter_routine_priv", "Execute_priv",
			"Event_priv", "Trigger_priv",
		},
		"mysql.tables_priv": {
			"Host", "Db", "User", "Table_name", "Grantor", "Timestamp", "Table_priv", "Column_priv",
		},
		"mysql.procs_priv": {
			"Host", "Db", "User", "Routine_name", "Routine_type", "Grantor", "Timestamp", "Proc_priv",
		},
		"mysql.role_edges": {
			"FROM_HOST", "FROM_USER", "TO_HOST", "TO_USER", "WITH_ADMIN_OPTION",
		},
		"mysql.global_grants": {
			"USER", "HOST", "PRIV", "WITH_GRANT_OPTION",
		},
		"mysql.proxies_priv": {
			"Host", "User", "Proxied_host", "Proxied_user", "With_grant", "Grantor", "Timestamp",
		},
		"mysql.infoschema_user_privileges": {
			"GRANTEE", "TABLE_CATALOG", "PRIVILEGE_TYPE", "IS_GRANTABLE",
		},
		"mysql.infoschema_schema_privileges": {
			"GRANTEE", "TABLE_CATALOG", "TABLE_SCHEMA", "PRIVILEGE_TYPE", "IS_GRANTABLE",
		},
		"mysql.infoschema_table_privileges": {
			"GRANTEE", "TABLE_CATALOG", "TABLE_SCHEMA", "TABLE_NAME", "PRIVILEGE_TYPE", "IS_GRANTABLE",
		},
	}

	columns, ok := columnMap[tableName]
	if !ok {
		return nil, fmt.Errorf("unknown table: %s", tableName)
	}
	return columns, nil
}
