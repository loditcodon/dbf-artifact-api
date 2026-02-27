package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/repository"

	"gorm.io/gorm"
)

// policyClassification categorizes policy templates by execution order and scope.
// Shared by both MySQL and Oracle handlers for three-pass execution strategy.
type policyClassification struct {
	superPrivileges     []models.DBPolicyDefault // Execute first, broadest scope (all actions, all objects, all databases)
	actionWidePrivs     []models.DBPolicyDefault // Execute second, action across all objects/databases
	objectSpecificPrivs []models.DBPolicyDefault // Execute last, specific objects/databases
}

// grantedActionsCache tracks which actions have been globally granted to prevent redundant checks.
// Key: actorID, Value: set of actionIDs already granted.
// Thread-safe for concurrent access from multiple goroutines.
type grantedActionsCache struct {
	mu     sync.RWMutex
	grants map[uint]map[int]bool
}

// allowedPolicyResults stores direct query results that passed isPolicyAllowed check.
// Used by assignActorsToGroups instead of reading from dbpolicy table.
// Key: actorID, Value: set of policy_default_ids that were allowed by query evaluation.
// Thread-safe for concurrent access from multiple goroutines.
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

// groupListPolicy holds a parsed dbgroup_listpolicies entry with its required policy_default_ids.
// Shared by both MySQL and Oracle actor-to-group assignment.
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

	// Load active dbgroup_listpolicies for matching
	groupListPolicyRepo := repository.NewDBGroupListPoliciesRepository()
	allGroupListPolicies, err := groupListPolicyRepo.GetActiveByDatabaseType(tx, dbType.ID)
	if err != nil {
		logger.Warnf("Failed to load DBGroupListPolicies for database_type_id=%d: %v", dbType.ID, err)
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
