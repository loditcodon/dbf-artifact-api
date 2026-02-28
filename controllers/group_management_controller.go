package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"dbfartifactapi/models"
	"dbfartifactapi/pkg/logger"
	"dbfartifactapi/services/group"
	"dbfartifactapi/utils"

	"github.com/gin-gonic/gin"
)

var groupMgtSrv = group.NewGroupManagementService()

// SetGroupManagementService initializes the group management service instance.
func SetGroupManagementService(srv group.GroupManagementService) {
	groupMgtSrv = srv
}

// Group Management Endpoints

// CreateGroup creates a new database group
// @Summary Create database group
// @Description Creates a new database group with specified parameters. Group code must be unique.
// @Tags Group Management
// @Accept json
// @Produce json
// @Param group body GroupCreateRequest true "Database Group object"
// @Success 201 {object} GroupCreateResponse "Group created successfully"
// @Failure 400 {object} ValidationErrorResponse "Invalid request body or validation error"
// @Failure 409 {object} GroupCodeConflictResponse "Group code already exists"
// @Failure 500 {object} GroupCreationErrorResponse "Internal server error"
// @Router /api/queries/groups [post]
func createGroup(c *gin.Context) {
	var group models.DBGroupMgt
	if err := c.ShouldBindJSON(&group); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	if err := utils.ValidateStruct(&group); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	createdGroup, err := groupMgtSrv.CreateGroup(c.Request.Context(), &group)
	if err != nil {
		logger.Errorf("Failed to create group: %v", err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Infof("Successfully created group: id=%d, name=%s", createdGroup.ID, createdGroup.Name)
	utils.JSONResponse(c, http.StatusCreated, createdGroup)
}

// UpdateGroup updates an existing database group
// @Summary Update database group
// @Description Updates an existing database group with specified ID. Code must remain unique.
// @Tags Group Management
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param group body GroupCreateRequest true "Database Group object"
// @Success 200 {object} GroupCreateResponse "Group updated successfully"
// @Failure 400 {object} InvalidGroupIDResponse "Invalid group ID or request body"
// @Failure 404 {object} GroupNotFoundResponse "Group not found"
// @Failure 409 {object} GroupCodeConflictResponse "Group code already exists"
// @Failure 500 {object} GroupCreationErrorResponse "Internal server error"
// @Router /api/queries/groups/{id} [put]
func updateGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid group ID"))
		return
	}

	var group models.DBGroupMgt
	if err := c.ShouldBindJSON(&group); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	if err := utils.ValidateStruct(&group); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	updatedGroup, err := groupMgtSrv.UpdateGroup(c.Request.Context(), uint(id), &group)
	if err != nil {
		logger.Errorf("Failed to update group %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Infof("Successfully updated group: id=%d, name=%s", updatedGroup.ID, updatedGroup.Name)
	utils.JSONResponse(c, http.StatusOK, updatedGroup)
}

// DeleteGroup deletes a database group
// @Summary Delete database group
// @Description Deletes a database group and all its associations. Cannot delete groups with child groups.
// @Tags Group Management
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {object} SuccessResponse "Group deleted successfully"
// @Failure 400 {object} InvalidGroupIDResponse "Invalid group ID"
// @Failure 404 {object} GroupNotFoundResponse "Group not found"
// @Failure 409 {object} GroupHasChildrenResponse "Group has child groups"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/groups/{id} [delete]
func deleteGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid group ID"))
		return
	}

	if err := groupMgtSrv.DeleteGroup(c.Request.Context(), uint(id)); err != nil {
		logger.Errorf("Failed to delete group %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Infof("Successfully deleted group: id=%d", id)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": "Group deleted successfully",
	})
}

// GetGroupByID retrieves a database group by ID
// @Summary Get database group by ID
// @Description Retrieves a database group with specified ID
// @Tags Group Management
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {object} GroupCreateResponse "Group details"
// @Failure 400 {object} InvalidGroupIDResponse "Invalid group ID"
// @Failure 404 {object} GroupNotFoundResponse "Group not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/groups/{id} [get]
func getGroupByID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid group ID"))
		return
	}

	group, err := groupMgtSrv.GetGroupByID(c.Request.Context(), uint(id))
	if err != nil {
		logger.Errorf("Failed to get group %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}

	utils.JSONResponse(c, http.StatusOK, group)
}

// GetAllGroups retrieves all active database groups
// @Summary Get all database groups
// @Description Retrieves all active database groups. Optional filter by database type ID.
// @Tags Group Management
// @Accept json
// @Produce json
// @Param database_type_id query int false "Filter by database type ID"
// @Success 200 {array} GroupCreateResponse "List of groups"
// @Failure 400 {object} ValidationErrorResponse "Invalid database_type_id parameter"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/groups [get]
func getAllGroups(c *gin.Context) {
	databaseTypeIDStr := c.Query("database_type_id")

	var groups []models.DBGroupMgt
	var err error

	if databaseTypeIDStr != "" {
		databaseTypeID, parseErr := strconv.ParseUint(databaseTypeIDStr, 10, 32)
		if parseErr != nil {
			utils.ErrorResponse(c, fmt.Errorf("invalid database_type_id"))
			return
		}
		groups, err = groupMgtSrv.GetGroupsByDatabaseType(c.Request.Context(), uint(databaseTypeID))
	} else {
		groups, err = groupMgtSrv.GetAllGroups(c.Request.Context())
	}

	if err != nil {
		logger.Errorf("Failed to get groups: %v", err)
		utils.ErrorResponse(c, err)
		return
	}

	utils.JSONResponse(c, http.StatusOK, groups)
}

// Policy Group Management Endpoints

// AssignPoliciesToGroup assigns policies to a group
// @Summary Assign policies to group
// @Description Assigns multiple policies to a database group
// @Tags Group Management
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param policies body PolicyAssignmentRequest true "Policy IDs to assign"
// @Success 200 {object} GroupAssignmentResponse "Policies assigned successfully"
// @Failure 400 {object} InvalidGroupIDResponse "Invalid group ID"
// @Failure 400 {object} EmptyListValidationResponse "Empty policy list"
// @Failure 404 {object} GroupNotFoundResponse "Group not found"
// @Failure 500 {object} PolicyAssignmentErrorResponse "Policy assignment failed"
// @Router /api/queries/groups/{id}/policies [post]
func assignPoliciesToGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid group ID"))
		return
	}

	var request struct {
		PolicyIDs []uint `json:"policy_ids" validate:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	if err := utils.ValidateStruct(&request); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	if err := groupMgtSrv.AssignPoliciesToGroup(c.Request.Context(), uint(id), request.PolicyIDs); err != nil {
		logger.Errorf("Failed to assign policies to group %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Infof("Successfully assigned %d policies to group %d", len(request.PolicyIDs), id)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": fmt.Sprintf("Successfully assigned %d policies to group", len(request.PolicyIDs)),
	})
}

// RemovePoliciesFromGroup removes policies from a group
// @Summary Remove policies from group
// @Description Removes multiple policies from a database group
// @Tags Group Management
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param policies body PolicyAssignmentRequest true "Policy IDs to remove"
// @Success 200 {object} GroupAssignmentResponse "Policies removed successfully"
// @Failure 400 {object} InvalidGroupIDResponse "Invalid group ID"
// @Failure 400 {object} EmptyListValidationResponse "Empty policy list"
// @Failure 404 {object} GroupNotFoundResponse "Group not found"
// @Failure 500 {object} PolicyAssignmentErrorResponse "Policy removal failed"
// @Router /api/queries/groups/{id}/policies [delete]
func removePoliciesFromGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid group ID"))
		return
	}

	var request struct {
		PolicyIDs []uint `json:"policy_ids" validate:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	if err := utils.ValidateStruct(&request); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	if err := groupMgtSrv.RemovePoliciesFromGroup(c.Request.Context(), uint(id), request.PolicyIDs); err != nil {
		logger.Errorf("Failed to remove policies from group %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Infof("Successfully removed %d policies from group %d", len(request.PolicyIDs), id)
	utils.JSONResponse(c, http.StatusOK, gin.H{
		"message": fmt.Sprintf("Successfully removed %d policies from group", len(request.PolicyIDs)),
	})
}

// GetGroupPolicies retrieves active policies for a group
// @Summary Get group policies
// @Description Retrieves all active policies assigned to a database group
// @Tags Group Management
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {array} GroupListPolicyResponse "List of policies"
// @Failure 400 {object} InvalidGroupIDResponse "Invalid group ID"
// @Failure 404 {object} GroupNotFoundResponse "Group not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/groups/{id}/policies [get]
func getGroupPolicies(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid group ID"))
		return
	}

	policies, err := groupMgtSrv.GetActivePoliciesForGroup(c.Request.Context(), uint(id))
	if err != nil {
		logger.Errorf("Failed to get policies for group %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}

	utils.JSONResponse(c, http.StatusOK, policies)
}

// GetPolicyGroups retrieves groups assigned to a policy
// @Summary Get policy groups
// @Description Retrieves all groups assigned to a specific policy
// @Tags Group Management
// @Accept json
// @Produce json
// @Param policy_id path int true "Policy ID"
// @Success 200 {array} GroupCreateResponse "List of groups"
// @Failure 400 {object} InvalidPolicyIDResponse "Invalid policy ID"
// @Failure 404 {object} PolicyNotFoundResponse "Policy not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/policies/{policy_id}/groups [get]
func getPolicyGroups(c *gin.Context) {
	policyID, err := strconv.ParseUint(c.Param("policy_id"), 10, 32)
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid policy ID"))
		return
	}

	groups, err := groupMgtSrv.GetGroupsForPolicy(c.Request.Context(), uint(policyID))
	if err != nil {
		logger.Errorf("Failed to get groups for policy %d: %v", policyID, err)
		utils.ErrorResponse(c, err)
		return
	}

	utils.JSONResponse(c, http.StatusOK, groups)
}

// Actor Group Management Endpoints

// AssignActorsToGroup assigns actors to a group
// @Summary Assign actors to group
// @Description Assigns multiple actors to a database group. Sends VeloArtifact allow commands to clients before updating database. Supports partial success - if some VeloArtifact operations fail, the operation continues with successful ones. Returns detailed execution status including success/failure information for each VeloArtifact job.
// @Tags Group Management
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param actors body ActorAssignmentRequest true "Actor IDs to assign"
// @Success 200 {object} ActorAssignmentResult "Actors assigned with detailed execution status"
// @Failure 400 {object} InvalidGroupIDResponse "Invalid group ID"
// @Failure 400 {object} EmptyListValidationResponse "Empty actor list"
// @Failure 404 {object} GroupNotFoundResponse "Group not found"
// @Failure 500 {object} ActorAssignmentErrorResponse "All VeloArtifact operations failed or actor assignment failed"
// @Router /api/queries/groups/{id}/actors [post]
func assignActorsToGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid group ID"))
		return
	}

	var request struct {
		ActorIDs []uint `json:"actor_ids" validate:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	if err := utils.ValidateStruct(&request); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	result, err := groupMgtSrv.AssignActorsToGroup(c.Request.Context(), uint(id), request.ActorIDs)
	if err != nil {
		logger.Errorf("Failed to assign actors to group %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Infof("Assigned %d actors to group %d, %d already assigned, %d VeloArtifact executions",
		len(result.ActorsAssigned), id, len(result.AlreadyAssigned), result.TotalExecutions)
	utils.JSONResponse(c, http.StatusOK, result)
}

// RemoveActorsFromGroup removes actors from a group
// @Summary Remove actors from group
// @Description Removes multiple actors from a database group. Sends VeloArtifact deny commands to clients before updating database. Supports partial success - if some VeloArtifact operations fail, the operation continues with successful ones. Returns detailed execution status including success/failure information for each VeloArtifact job.
// @Tags Group Management
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param actors body ActorAssignmentRequest true "Actor IDs to remove"
// @Success 200 {object} ActorRemovalResult "Actors removed with detailed execution status"
// @Failure 400 {object} InvalidGroupIDResponse "Invalid group ID"
// @Failure 400 {object} EmptyListValidationResponse "Empty actor list"
// @Failure 404 {object} GroupNotFoundResponse "Group not found"
// @Failure 500 {object} ActorAssignmentErrorResponse "All VeloArtifact operations failed or actor removal failed"
// @Router /api/queries/groups/{id}/actors [delete]
func removeActorsFromGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid group ID"))
		return
	}

	var request struct {
		ActorIDs []uint `json:"actor_ids" validate:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	if err := utils.ValidateStruct(&request); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	result, err := groupMgtSrv.RemoveActorsFromGroup(c.Request.Context(), uint(id), request.ActorIDs)
	if err != nil {
		logger.Errorf("Failed to remove actors from group %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Infof("Removed %d actors from group %d, %d not assigned, %d VeloArtifact executions",
		len(result.ActorsRemoved), id, len(result.NotAssigned), result.TotalExecutions)
	utils.JSONResponse(c, http.StatusOK, result)
}

// GetGroupActors retrieves active actors for a group
// @Summary Get group actors
// @Description Retrieves all active actors assigned to a database group
// @Tags Group Management
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {array} ActorResponse "List of actors"
// @Failure 400 {object} InvalidGroupIDResponse "Invalid group ID"
// @Failure 404 {object} GroupNotFoundResponse "Group not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/groups/{id}/actors [get]
func getGroupActors(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid group ID"))
		return
	}

	actors, err := groupMgtSrv.GetActiveActorsForGroup(c.Request.Context(), uint(id))
	if err != nil {
		logger.Errorf("Failed to get actors for group %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}

	utils.JSONResponse(c, http.StatusOK, actors)
}

// GetActorGroups retrieves groups assigned to an actor
// @Summary Get actor groups
// @Description Retrieves all groups assigned to a specific actor
// @Tags Group Management
// @Accept json
// @Produce json
// @Param actor_id path int true "Actor ID"
// @Success 200 {array} GroupCreateResponse "List of groups"
// @Failure 400 {object} InvalidActorIDResponse "Invalid actor ID"
// @Failure 404 {object} ActorNotFoundResponse "Actor not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/actors/{actor_id}/groups [get]
func getActorGroups(c *gin.Context) {
	actorID, err := strconv.ParseUint(c.Param("actor_id"), 10, 32)
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid actor ID"))
		return
	}

	groups, err := groupMgtSrv.GetGroupsForActor(c.Request.Context(), uint(actorID))
	if err != nil {
		logger.Errorf("Failed to get groups for actor %d: %v", actorID, err)
		utils.ErrorResponse(c, err)
		return
	}

	utils.JSONResponse(c, http.StatusOK, groups)
}

// GetGroupDetails retrieves comprehensive group information
// @Summary Get comprehensive group details
// @Description Retrieves group details along with assigned policies and actors
// @Tags Group Management
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {object} GroupDetailsResponse "Comprehensive group information"
// @Failure 400 {object} InvalidGroupIDResponse "Invalid group ID"
// @Failure 404 {object} GroupNotFoundResponse "Group not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/groups/{id}/details [get]
func getGroupDetails(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid group ID"))
		return
	}

	groupInfo, err := groupMgtSrv.GetGroupWithPoliciesAndActors(c.Request.Context(), uint(id))
	if err != nil {
		logger.Errorf("Failed to get comprehensive group details for %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}

	utils.JSONResponse(c, http.StatusOK, groupInfo)
}

// UpdateGroupAssignments performs optimized bulk update of group assignments
// @Summary Update group assignments (optimized)
// @Description Updates both policies and actors for a group with minimal VeloArtifact executions. This is the optimized alternative to individual assignment operations.
// @Tags Group Management
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param request body GroupAssignmentsUpdateRequest true "New policy and actor assignments"
// @Success 200 {object} GroupAssignmentsUpdateResult "Group assignments updated successfully with optimization details"
// @Failure 400 {object} InvalidGroupIDResponse "Invalid group ID or request"
// @Failure 404 {object} GroupNotFoundResponse "Group not found"
// @Failure 500 {object} InternalServerErrorResponse "Internal server error"
// @Router /api/queries/groups/{id}/assignments [put]
func updateGroupAssignments(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid group ID"))
		return
	}

	var request group.GroupAssignmentsUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid request body: %v", err))
		return
	}

	result, err := groupMgtSrv.UpdateGroupAssignments(c.Request.Context(), uint(id), &request)
	if err != nil {
		logger.Errorf("Failed to update group assignments for %d: %v", id, err)
		utils.ErrorResponse(c, err)
		return
	}

	logger.Infof("Updated group %d assignments: %d policies added, %d policies removed, %d actors added, %d actors removed, %d VeloArtifact executions",
		id, len(result.PoliciesAdded), len(result.PoliciesRemoved), len(result.ActorsAdded), len(result.ActorsRemoved), result.TotalExecutions)

	utils.JSONResponse(c, http.StatusOK, result)
}

// Bulk Operations

// BulkAssignPolicyToGroups assigns a policy to multiple groups
// @Summary Bulk assign policy to groups
// @Description Assigns a single policy to multiple database groups. Returns partial success if some groups fail.
// @Tags Group Management
// @Accept json
// @Produce json
// @Param policy_id path int true "Policy ID"
// @Param groups body GroupAssignmentRequest true "Group IDs to assign policy to"
// @Success 200 {object} GroupBulkAssignmentResponse "Policy assigned to all groups successfully"
// @Success 206 {object} GroupBulkAssignmentResponse "Policy assigned to some groups (partial success)"
// @Failure 400 {object} InvalidPolicyIDResponse "Invalid policy ID"
// @Failure 400 {object} EmptyListValidationResponse "Empty group list"
// @Failure 404 {object} PolicyNotFoundResponse "Policy not found"
// @Failure 500 {object} PolicyAssignmentErrorResponse "Policy assignment failed"
// @Router /api/queries/policies/{policy_id}/assign-groups [post]
func bulkAssignPolicyToGroups(c *gin.Context) {
	policyID, err := strconv.ParseUint(c.Param("policy_id"), 10, 32)
	if err != nil {
		utils.ErrorResponse(c, fmt.Errorf("invalid policy ID"))
		return
	}

	var request struct {
		GroupIDs []uint `json:"group_ids" validate:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	if err := utils.ValidateStruct(&request); err != nil {
		utils.ErrorResponse(c, err)
		return
	}

	successCount := 0
	var errors []string

	for _, groupID := range request.GroupIDs {
		if err := groupMgtSrv.AssignPoliciesToGroup(c.Request.Context(), groupID, []uint{uint(policyID)}); err != nil {
			errors = append(errors, fmt.Sprintf("Group %d: %v", groupID, err))
		} else {
			successCount++
		}
	}

	if len(errors) > 0 {
		logger.Errorf("Bulk assign policy %d partially failed: %s", policyID, strings.Join(errors, "; "))
		utils.JSONResponse(c, http.StatusPartialContent, gin.H{
			"message":       fmt.Sprintf("Policy assigned to %d/%d groups", successCount, len(request.GroupIDs)),
			"success_count": successCount,
			"total_count":   len(request.GroupIDs),
			"errors":        errors,
		})
	} else {
		logger.Infof("Successfully assigned policy %d to %d groups", policyID, successCount)
		utils.JSONResponse(c, http.StatusOK, gin.H{
			"message":       fmt.Sprintf("Policy assigned to all %d groups successfully", successCount),
			"success_count": successCount,
		})
	}
}

// RegisterGroupManagementRoutes registers HTTP endpoints for group management operations.
func RegisterGroupManagementRoutes(rg *gin.RouterGroup) {
	// Group CRUD operations
	groups := rg.Group("/groups")
	{
		groups.POST("", createGroup)
		groups.GET("", getAllGroups)
		groups.GET("/:id", getGroupByID)
		groups.PUT("/:id", updateGroup)
		groups.DELETE("/:id", deleteGroup)
		groups.GET("/:id/details", getGroupDetails)
		groups.PUT("/:id/assignments", updateGroupAssignments)

		// Policy assignments for groups
		groups.POST("/:id/policies", assignPoliciesToGroup)
		groups.DELETE("/:id/policies", removePoliciesFromGroup)
		groups.GET("/:id/policies", getGroupPolicies)

		// Actor assignments for groups
		groups.POST("/:id/actors", assignActorsToGroup)
		groups.DELETE("/:id/actors", removeActorsFromGroup)
		groups.GET("/:id/actors", getGroupActors)
	}

	// Policy-centric routes
	policies := rg.Group("/policies")
	{
		policies.GET("/:policy_id/groups", getPolicyGroups)
		policies.POST("/:policy_id/assign-groups", bulkAssignPolicyToGroups)
	}

	// Actor-centric routes
	actors := rg.Group("/actors")
	{
		actors.GET("/:actor_id/groups", getActorGroups)
	}
}
